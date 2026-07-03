package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// ApiJsonAPI implements API for path-based JSON navigation (e.g. Skyline APK builds).
type ApiJsonAPI struct {
	apiURL     string
	dlCfg      config.DownloadConfig
	verCfg     config.VersionConfig
	downloader Downloader
	jsonData   interface{} // Can be map or array
}

// NewApiJsonAPI creates a new ApiJson API adapter.
func NewApiJsonAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, dl Downloader) *ApiJsonAPI {
	return &ApiJsonAPI{
		apiURL:     cfg.APIURL,
		dlCfg:      dlCfg,
		verCfg:     verCfg,
		downloader: dl,
	}
}

func (a *ApiJsonAPI) fetchJSON(ctx context.Context) error {
	if a.jsonData != nil {
		return nil
	}

	resp, err := a.downloader.Get(ctx, a.apiURL)
	if err != nil {
		return fmt.Errorf("apijson fetch: %w", err)
	}

	if err := unmarshalJSON(resp.Body, &a.jsonData); err != nil {
		return fmt.Errorf("parse apijson: %w", err)
	}
	return nil
}

// dictPathGet traverses a nested structure using a path of keys/indices.
func dictPathGet(input interface{}, path []interface{}) (interface{}, error) {
	current := input
	for _, segment := range path {
		switch s := segment.(type) {
		case string:
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("apijson: expected map at key %q, got %T", s, current)
			}
			current = m[s]
		case float64:
			arr, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("apijson: expected array at index %v, got %T", s, current)
			}
			idx := int(s)
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("apijson: index %d out of range [0, %d)", idx, len(arr))
			}
			current = arr[idx]
		default:
			return nil, fmt.Errorf("apijson: unsupported path segment type %T", segment)
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

	dlURL, err := a.buildDownloadURL()
	if err != nil {
		return nil, err
	}

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

func (a *ApiJsonAPI) buildDownloadURL() (string, error) {
	if len(a.dlCfg.Path) == 0 {
		return "", fmt.Errorf("apijson: no path configured")
	}

	parts := make([]string, 0, len(a.dlCfg.Path))
	for i, segment := range a.dlCfg.Path {
		switch s := segment.(type) {
		case string:
			parts = append(parts, s)
		case []interface{}:
			val, err := dictPathGet(a.jsonData, s)
			if err != nil {
				return "", fmt.Errorf("apijson path[%d]: %w", i, err)
			}
			parts = append(parts, fmt.Sprintf("%v", val))
		default:
			return "", fmt.Errorf("apijson: unsupported path segment type %T at index %d", segment, i)
		}
	}

	return strings.Join(parts, "/"), nil
}
