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
	"sync"
	"syscall"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/downloader"
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

	// Load main config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received signal, shutting down")
		cancel()
	}()

	// Create aria2 downloader
	addr := cfg.Aria2.RPCAddr()
	var aria2DL downloader.Downloader
	aria2DL, err = downloader.NewAria2Downloader(ctx, addr, cfg.Aria2.RPCSecret, cfg.Aria2.RemoteDir, cfg.Aria2.LocalDir)
	if err != nil {
		logger.Warn("aria2 connection failed, continuing without download", "error", err)
	}
	if aria2DL != nil {
		defer aria2DL.Close()
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
			found := false
			for _, arg := range args {
				if arg == proj.Name {
					found = true
					break
				}
			}
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
			u := updater.New(*projCfg, proj.SavePath, flagForce, aria2DL, nil, logger)
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

func resolveConfigPath() string {
	if flagConfig != "" {
		return flagConfig
	}
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	defaultPath := filepath.Join(home, ".config", "updater-rpc", "config.json")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	return "config.json"
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
