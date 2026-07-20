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
	logger.Info("logging initialized",
		"level", logLevel.String(),
		"verbose", flagVerbose,
		"reason", "log level selected from --verbose flag",
		"result", logLevel.String(),
	)

	// Resolve config path
	configPath := resolveConfigPath()
	logger.Info("config path resolved",
		"path", configPath,
		"reason", "resolved from --config flag, cwd, or home dir",
		"result", configPath,
	)
	var cfg *config.Config
	_, err := os.Stat(configPath)
	if err != nil {
		logger.Warn("config not found, using defaults",
			"path", configPath,
			"reason", "no config file at resolved path",
			"result", "default config",
		)
		cfg = config.GetDefault()
	} else {
		// Load main config
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		logger.Info("config loaded",
			"path", configPath,
			"projects", len(cfg.Projects),
			"reason", "config file exists and parsed",
			"result", "ok",
		)
	}

	// Handle --path flag: add project and update
	if flagPath != "" {
		if len(args) != 1 {
			return fmt.Errorf("--path requires exactly one project name, got %d", len(args))
		}
		addProject(cfg, args[0], flagPath)
		// Persist added project if --add2conf
		if flagAdd2Conf {
			if err := writeJSON(configPath, cfg); err != nil {
				logger.Error("persist project failed", "error", err)
			}
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
	dlLogger := logger.With("component", "downloader")

	var aria2DL downloader.Downloader
	var localAria2 *downloader.LocalAria2
	aria2DL, localAria2, err = downloader.NewAria2DownloaderOrLocal(ctx, addr, cfg.Aria2.RPCSecret, cfg.Aria2.RemoteDir, cfg.Aria2.LocalDir, cfg.Binaries.Aria2c, cfg.Requests.Proxy, cfg.Requests.Retry, dlLogger, cfg.Requests.GetTimeout())
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
	httpDL := api.NewHTTPClientWithProxy(cfg.Requests.GetTimeout(), cfg.Requests.Proxy, cfg.Requests.Retry)

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
			if len(args) != 0 && !slices.Contains(args, proj.Name) {
				continue
			}
			if !proj.Enabled() {
				continue
			}
			if err := metaStore.EnsureLocalConfig(ctx, proj.Name); err != nil {
				logger.Warn("failed to ensure local config", "name", proj.Name, "error", err)
				return err
			}
		}
	}

	// Determine worker count
	jobs := flagJobs
	if jobs <= 0 {
		jobs = runtime.GOMAXPROCS(0)
		logger.Info("worker count resolved",
			"jobs", jobs,
			"reason", "--jobs not set, defaulting to GOMAXPROCS",
			"result", jobs,
		)
	} else {
		logger.Info("worker count resolved",
			"jobs", jobs,
			"reason", "--jobs flag set",
			"result", jobs,
		)
	}

	// Run updates with bounded parallelism
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(jobs)

	var results []*updater.UpdateResult
	var mu sync.Mutex

	for _, proj := range cfg.Projects {
		if !proj.Enabled() {
			logger.Debug("project skipped",
				"name", proj.Name,
				"reason", "project disabled in config",
				"result", "skip",
			)
			continue
		}

		// Filter by positional args if provided
		if len(args) > 0 {
			found := slices.Contains(args, proj.Name)
			if !found {
				logger.Debug("project skipped",
					"name", proj.Name,
					"reason", "not in positional args filter",
					"result", "skip",
				)
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
			upLogger := logger.With("component", "updater", "project", proj.Name)
			u := updater.New(*projCfg, proj, flagForce, aria2DL, httpDL, upLogger)
			result := u.Update(gctx)
			mu.Lock()
			defer mu.Unlock()
			results = append(results, result)
			if result.Error != nil {
				return result.Error
			}
			updateVersion(cfg, proj.Name, result.NewVersion)
			return writeJSON(configPath, cfg)
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

// writeJSON writes data as indented JSON to a file, creating parent dirs as needed.
func writeJSON(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func updateVersion(cfg *config.Config, name, version string) {
	for i, v := range cfg.Projects {
		if v.Name == name {
			cfg.Projects[i].Version = version
		}
	}
}

// projectExists checks if a project name already exists in the config.
func projectExists(projects []config.ProjectEntry, name string) int {
	for i, p := range projects {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// addProject adds a new project to the config file.
func addProject(cfg *config.Config, projectName, savePath string) {
	// Check if project already exists
	i := projectExists(cfg.Projects, projectName)
	if i >= 0 {
		cfg.Projects[i].SavePath = savePath
		return
	}

	// Add new project
	cfg.Projects = append(cfg.Projects, config.ProjectEntry{
		Name:     projectName,
		SavePath: savePath,
	})
	slog.Info("project added to config", "name", projectName, "path", savePath)
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
