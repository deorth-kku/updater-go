// Package api provides version detection for different source backends.
package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deorth-kku/updater-go/internal/config"
)

// Release represents a downloadable release from any API.
type Release struct {
	URL       string
	Version   string
	Assets    []Asset
	Artifacts []AppveyorArtifact
	JobID     string
	BaseURL   string
}

// Asset represents a single downloadable file within a release.
type Asset struct {
	URL  string
	Name string
}

// API is the interface all version-source backends implement.
type API interface {
	Latest(ctx context.Context) (*Release, error)
}

// NewAPI creates the appropriate API adapter based on the project config.
func NewAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, buildCfg config.BuildConfig, dl Downloader, logger *slog.Logger) (API, error) {
	logger = logger.With("api_type", cfg.APIType)
	switch cfg.APIType {
	case "github":
		api := NewGitHubAPI(cfg, dl, logger)
		api.SetNoPull(buildCfg.NoPull)
		logger.Info("api backend selected",
			"project", cfg.ProjectName,
			"api_type", "github",
			"reason", "config api_type is github",
			"result", "github",
		)
		return api, nil
	case "appveyor":
		api := NewAppveyorAPI(cfg, dl, logger)
		api.SetBranch(buildCfg.Branch)
		logger.Info("api backend selected",
			"project", cfg.ProjectName,
			"api_type", "appveyor",
			"reason", "config api_type is appveyor",
			"result", "appveyor",
		)
		return api, nil
	case "sourceforge":
		logger.Info("api backend selected",
			"project", cfg.ProjectName,
			"api_type", "sourceforge",
			"reason", "config api_type is sourceforge",
			"result", "sourceforge",
		)
		return NewSourceforgeAPI(cfg, dlCfg, dl, logger), nil
	case "simplespider":
		logger.Info("api backend selected",
			"project", cfg.ProjectName,
			"api_type", "simplespider",
			"reason", "config api_type is simplespider",
			"result", "simplespider",
		)
		return NewSimpleSpiderAPI(cfg, dlCfg, verCfg, dl, logger), nil
	case "apijson":
		logger.Info("api backend selected",
			"project", cfg.ProjectName,
			"api_type", "apijson",
			"reason", "config api_type is apijson",
			"result", "apijson",
		)
		return NewApiJsonAPI(cfg, dlCfg, verCfg, dl, logger), nil
	default:
		logger.Error("unknown api_type",
			"project", cfg.ProjectName,
			"api_type", cfg.APIType,
			"reason", "config api_type did not match any known backend",
			"result", "error",
		)
		return nil, fmt.Errorf("unknown api_type: %q", cfg.APIType)
	}
}
