// Command updater is the main CLI entry point for the updater-rpc Go rewrite.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"

	"github.com/deorth-kku/updater-go/internal/api"
	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/downloader"
	"github.com/deorth-kku/updater-go/internal/metadata"
	"github.com/deorth-kku/updater-go/internal/updater"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	flagConfig   string
	flagForce    bool
	flagAdd2Conf bool
	flagWait     bool
	flagJobs     int
	flagVerbose  bool
	flagPath     string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "updater [projects...]",
		Short: "Update applications via aria2 RPC",
		Long:  "Detect latest versions via multiple API backends, download via aria2, extract, and manage process restarts.",
		RunE:  run,
	}

	rootCmd.Flags().StringVarP(&flagConfig, "config", "c", "", "config file path (default: ./config.json or $HOME/.config/updater-rpc/config.json)")
	rootCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "force update regardless of version")
	rootCmd.Flags().BoolVarP(&flagAdd2Conf, "add2conf", "a", false, "persist added project to config")
	rootCmd.Flags().BoolVarP(&flagWait, "wait", "w", false, "pause before exit (Windows convenience)")
	rootCmd.Flags().IntVarP(&flagJobs, "jobs", "j", 0, "max parallel update workers (default: GOMAXPROCS)")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "enable debug logging")
	rootCmd.Flags().StringVarP(&flagPath, "path", "p", "", "install path for added project (e.g. --path /opt/tools)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Setup logging
	logLevel := slog.LevelInfo
	if flagVerbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Resolve config path
	configPath := resolveConfigPath()
	var cfg *config.Config
	_, err := os.Stat(configPath)
	if err != nil {
		cfg = config.GetDefault()
	} else {
		// Load main config
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	}

	// Handle --path flag: add project and update
	if flagPath != "" {
		if len(args) != 1 {
			return fmt.Errorf("--path requires exactly one project name, got %d", len(args))
		}
		projectName := args[0]
		if err := persistProject(cfg, configPath, projectName, flagPath); err != nil {
			return fmt.Errorf("persist project: %w", err)
		}
		// Reload config after adding project
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("reload config: %w", err)
		}
		args = nil // Clear args to run all projects
	}

	// Persist added project if --add2conf
	if flagAdd2Conf && len(args) > 0 {
		if err := persistProject(cfg, configPath, args[0], cfg.LocalDir); err != nil {
			logger.Error("persist project failed", "error", err)
		}
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create aria2 downloader with fallback to local aria2c subprocess
	addr := cfg.Aria2.RPCAddr()

	var aria2DL downloader.Downloader
	var localAria2 *downloader.LocalAria2
	aria2DL, localAria2, err = downloader.NewAria2DownloaderOrLocal(ctx, addr, cfg.Aria2.RPCSecret, cfg.Aria2.RemoteDir, cfg.Aria2.LocalDir, cfg.Binaries.Aria2c, logger, cfg.Requests.GetTimeout())
	if err != nil {
		logger.Warn("aria2 connection failed", "error", err)
		return err
	}
	if aria2DL != nil {
		defer aria2DL.Close()
	}

	if localAria2 != nil {
		defer localAria2.Stop()
	}

	// Create HTTP downloader for metadata
	httpDL := api.NewHTTPClientWithProxy(cfg.Requests.GetTimeout(), cfg.Requests.Proxy)

	// Load metadata from repositories
	var metaStore *metadata.Store
	if len(cfg.Repositories) > 0 {
		metaStore = metadata.NewStore(cfg.Repositories, httpDL)
		// Set local config directory for storing project configs
		localConfigDir := filepath.Join(filepath.Dir(configPath), "config")
		metaStore.SetLocalConfigDir(localConfigDir)

		if err := metaStore.Load(ctx); err != nil {
			logger.Error("metadata load failed", "error", err)
			return err
		}

		// Ensure local configs are up-to-date for all projects
		for _, proj := range cfg.Projects {
			if !proj.Enabled {
				continue
			}
			if err := metaStore.EnsureLocalConfig(ctx, proj.Name); err != nil {
				logger.Warn("failed to ensure local config", "name", proj.Name, "error", err)
			}
		}
	}

	// Determine worker count
	jobs := flagJobs
	if jobs <= 0 {
		jobs = runtime.GOMAXPROCS(0)
	}

	// Run updates with bounded parallelism
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(jobs)

	var results []*updater.UpdateResult
	var mu sync.Mutex

	for _, proj := range cfg.Projects {
		if !proj.Enabled {
			continue
		}

		// Filter by positional args if provided
		if len(args) > 0 {
			found := slices.Contains(args, proj.Name)
			if !found {
				continue
			}
		}

		projCfg, err := loadProjectConfig(filepath.Dir(configPath), proj.Name, cfg.Defaults)
		if err != nil {
			logger.Error("load project config", "name", proj.Name, "error", err)
			continue
		}
		if projCfg == nil {
			logger.Warn("no project config found", "name", proj.Name)
			continue
		}

		g.Go(func() error {
			u := updater.New(*projCfg, proj.SavePath, flagForce, aria2DL, httpDL, logger)
			result := u.Update(gctx)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
			return result.Error
		})
	}

	if err := g.Wait(); err != nil {
		logger.Error("update failed", "error", err)
	}

	// Log results
	for _, r := range results {
		if r.Error != nil {
			logger.Error("update failed", "project", r.ProjectName, "error", r.Error)
		} else {
			logger.Info("update ok", "project", r.ProjectName, "version", r.NewVersion)
		}
	}

	if flagWait {
		fmt.Println("Press Enter to exit...")
		fmt.Scanln()
	}

	return nil
}

// projectExists checks if a project name already exists in the config.
func projectExists(projects []config.ProjectEntry, name string) bool {
	for _, p := range projects {
		if p.Name == name {
			return true
		}
	}
	return false
}

// persistProject adds a new project to the config file.
func persistProject(cfg *config.Config, configPath, projectName, savePath string) error {
	// Check if project already exists
	if projectExists(cfg.Projects, projectName) {
		slog.Info("project already exists in config", "name", projectName)
		return nil
	}

	// Add new project
	cfg.Projects = append(cfg.Projects, config.ProjectEntry{
		Name:     projectName,
		SavePath: savePath,
		Enabled:  true,
	})

	// Write back
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	err = os.MkdirAll(filepath.Dir(configPath), 0o755)
	if err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("project persisted to config", "name", projectName)
	return nil
}

func resolveConfigPath() string {
	if flagConfig != "" {
		return flagConfig
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}

	// Windows: %APPDATA%\updater-rpc\config.json
	// Linux/macOS: $HOME/.config/updater-rpc/config.json
	var defaultPath string
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			return "config.json"
		}
		defaultPath = filepath.Join(appdata, "updater-rpc", "config.json")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "config.json"
		}
		defaultPath = filepath.Join(home, ".config", "updater-rpc", "config.json")
	}
	return defaultPath
}

func loadProjectConfig(configRoot, name string, defaults json.RawMessage) (*config.ProjectConfig, error) {
	localPath := config.ProjectConfigPath(configRoot, name)
	data, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read project config %s: %w", localPath, err)
	}
	var pc config.ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("unmarshal project config %s: %w", localPath, err)
	}
	if err := config.ApplyDefaults(&pc, data, defaults); err != nil {
		return nil, fmt.Errorf("apply defaults for %s: %w", name, err)
	}
	return &pc, nil
}
