// Package api provides version detection for different source backends.
package api

import (
	"context"
	"fmt"

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
func NewAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, buildCfg config.BuildConfig, dl Downloader) (API, error) {
	switch cfg.APIType {
	case "github":
		api := NewGitHubAPI(cfg, dl)
		api.SetNoPull(buildCfg.NoPull)
		return api, nil
	case "appveyor":
		api := NewAppveyorAPI(cfg, dl)
		api.SetBranch(buildCfg.Branch)
		return api, nil
	case "sourceforge":
		return NewSourceforgeAPI(cfg, dl), nil
	case "simplespider":
		return NewSimpleSpiderAPI(cfg, dlCfg, verCfg, dl), nil
	case "apijson":
		return NewApiJsonAPI(cfg, dlCfg, verCfg, dl), nil
	default:
		return nil, fmt.Errorf("unknown api_type: %q", cfg.APIType)
	}
}
