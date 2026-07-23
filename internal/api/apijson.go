package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// ApiJsonAPI implements API for path-based JSON navigation (e.g. Skyline APK builds).
type ApiJsonAPI struct {
	apiURL     string
	dlCfg      config.DownloadConfig
	verCfg     config.VersionConfig
	downloader Downloader
	jsonData   any // Can be map or array
	logger     *slog.Logger
}

// NewApiJsonAPI creates a new ApiJson API adapter.
func NewApiJsonAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, dl Downloader, logger *slog.Logger) *ApiJsonAPI {
	return &ApiJsonAPI{
		apiURL:     cfg.APIURL,
		dlCfg:      dlCfg,
		verCfg:     verCfg,
		downloader: dl,
		logger:     logger,
	}
}

func (a *ApiJsonAPI) fetchJSON(ctx context.Context) error {
	if a.jsonData != nil {
		return nil
	}

	resp, err := a.downloader.Get(ctx, a.apiURL, nil)
	if err != nil {
		return fmt.Errorf("apijson fetch: %w", err)
	}

	if err := json.Unmarshal(resp.Body, &a.jsonData); err != nil {
		return fmt.Errorf("parse apijson: %w", err)
	}
	return nil
}

// dictPathGet traverses a nested structure using a path of keys/indices.
func dictPathGet(input any, path []config.PathSegment) (any, error) {
	current := input
	for _, segment := range path {
		if segment.IsString() {
			m, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("apijson: expected map at key %q, got %T", segment.Str, current)
			}
			current = m[segment.Str]
		} else {
			arr, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("apijson: expected array at index %d, got %T", segment.Int, current)
			}
			if segment.Int < 0 || segment.Int >= len(arr) {
				return nil, fmt.Errorf("apijson: index %d out of range [0, %d)", segment.Int, len(arr))
			}
			current = arr[segment.Int]
		}
		if current == nil {
			return nil, fmt.Errorf("apijson: nil value at path segment %v", segment)
		}
	}
	return current, nil
}

// Latest fetches the JSON and extracts version + download URL via configured paths.
func (a *ApiJsonAPI) Latest(ctx context.Context) (*Release, error) {
	if err := a.fetchJSON(ctx); err != nil {
		return nil, err
	}

	version := "unknown"
	// Use Version.Path to extract version from nested JSON
	if len(a.verCfg.Path) > 0 {
		val, err := dictPathGet(a.jsonData, a.verCfg.Path)
		if err == nil && val != nil {
			version = fmt.Sprintf("%v", val)
		}
	}
	a.logger.Info("latest version detected",
		"url", a.apiURL,
		"version", version,
		"reason", "version read from configured json path",
		"result", version,
	)

	dlURL, err := a.buildDownloadURL()
	if err != nil {
		return nil, err
	}
	a.logger.Debug("apijson url extracted",
		"url", dlURL,
		"reason", "download url built from configured path segments",
		"result", dlURL,
	)

	fileName := a.dlCfg.FilenameOverride
	if fileName == "" {
		fileName = extractFilename(dlURL)
	}

	return &Release{
		URL:     dlURL,
		Version: version,
		Assets: []Asset{
			{URL: dlURL, Name: fileName},
		},
	}, nil
}

// LatestByVersion is not supported for ApiJson (no historical version list).
func (a *ApiJsonAPI) LatestByVersion(_ context.Context, _ string) (*Release, error) {
	return nil, fmt.Errorf("rollback not supported for apijson backend")
}

func (a *ApiJsonAPI) buildDownloadURL() (string, error) {
	if len(a.dlCfg.Path) == 0 {
		return "", fmt.Errorf("apijson: no path configured")
	}

	parts := make([]string, 0, len(a.dlCfg.Path))
	for i, segment := range a.dlCfg.Path {
		if segment.Path == nil {
			parts = append(parts, segment.Str)
		} else {
			val, err := dictPathGet(a.jsonData, segment.Path)
			if err != nil {
				return "", fmt.Errorf("apijson path[%d]: %w", i, err)
			}
			parts = append(parts, fmt.Sprintf("%v", val))
		}
	}

	return strings.Join(parts, "/"), nil
}
