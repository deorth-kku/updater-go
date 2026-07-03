// Package updater orchestrates the full update flow: version check → download → extract → copy → post-cmds.
package updater

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/deorth-kku/updater-go/internal/api"
	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/downloader"
	"github.com/deorth-kku/updater-go/internal/extractor"
	"github.com/deorth-kku/updater-go/internal/platform"
)

// UpdateResult holds the result of updating a single project.
type UpdateResult struct {
	ProjectName string
	OldVersion  string
	NewVersion  string
	Downloaded  string
	Extracted   bool
	Error       error
}

// Updater orchestrates the update process for a single project.
type Updater struct {
	projectCfg config.ProjectConfig
	savePath   string
	force      bool
	dl         downloader.Downloader
	httpDL     api.Downloader
	logger     *slog.Logger
}

// New creates a new Updater.
func New(cfg config.ProjectConfig, savePath string, force bool, dl downloader.Downloader, httpDL api.Downloader, logger *slog.Logger) *Updater {
	return &Updater{
		projectCfg: cfg,
		savePath:   savePath,
		force:      force,
		dl:         dl,
		httpDL:     httpDL,
		logger:     logger,
	}
}

// Update runs the full update flow for the project.
func (u *Updater) Update(ctx context.Context) *UpdateResult {
	result := &UpdateResult{ProjectName: u.projectCfg.Basic.ProjectName}
	result.OldVersion = u.projectCfg.CurrentVersion

	// Step 1: Detect latest version via API
	apiAdapter, err := api.NewAPI(u.projectCfg.Basic, u.projectCfg.Download, u.projectCfg.Version, u.httpDL)
	if err != nil {
		result.Error = fmt.Errorf("create api: %w", err)
		return result
	}

	rel, err := apiAdapter.Latest(ctx)
	if err != nil {
		result.Error = fmt.Errorf("fetch latest: %w", err)
		return result
	}
	result.NewVersion = rel.Version

	// Step 2: Check if update is needed
	if !u.force && rel.Version == result.OldVersion {
		u.logger.Info("no update needed", "project", result.ProjectName, "version", rel.Version)
		return result
	}

	// Step 3: Select download URL
	dlURL := u.selectDownloadURL(rel)
	if dlURL == "" {
		result.Error = fmt.Errorf("no matching download URL found")
		return result
	}

	// Step 4: Download
	filename := u.downloadFilename(rel.Version, dlURL)
	saveDir := filepath.Join(u.savePath, result.ProjectName)
	localPath, _, err := u.dl.Download(ctx, dlURL, filename, saveDir)
	if err != nil {
		result.Error = fmt.Errorf("download: %w", err)
		return result
	}
	result.Downloaded = localPath

	// Step 5: Extract
	if !u.projectCfg.Decompress.Skip.Bool() {
		ext := strings.ToLower(filepath.Ext(localPath))
		if isArchive(ext) {
			extDest := strings.TrimSuffix(localPath, ext)
			if err := extractor.New(u.projectCfg.Decompress).Extract(localPath, extDest); err != nil {
				result.Error = fmt.Errorf("extract: %w", err)
				return result
			}
			result.Extracted = true
		}
	}

	u.logger.Info("update completed",
		"project", result.ProjectName,
		"version", rel.Version,
		"downloaded", localPath,
	)

	return result
}

// selectDownloadURL picks the best download URL from a release.
func (u *Updater) selectDownloadURL(rel *api.Release) string {
	// If a direct URL is configured, use it
	if u.projectCfg.Download.URL != "" {
		return u.projectCfg.Download.URL
	}

	// For GitHub releases, filter assets by keywords
	if len(rel.Assets) > 0 {
		fs := extractor.NewFileSelector(u.projectCfg.Download)
		for _, a := range rel.Assets {
			if fs.Match(a.Name) {
				return a.URL
			}
		}
	}

	// For AppVeyor artifacts
	if len(rel.Artifacts) > 0 {
		fs := extractor.NewFileSelector(u.projectCfg.Download)
		for _, art := range rel.Artifacts {
			if fs.Match(art.FileName) {
				return rel.BaseURL + "/buildjobs/" + rel.JobID + "/artifacts/" + art.FileName
			}
		}
	}

	// Fallback to the release URL
	if rel.URL != "" {
		return rel.URL
	}

	return ""
}

// downloadFilename determines the filename for the download.
func (u *Updater) downloadFilename(version, dlURL string) string {
	if u.projectCfg.Download.FilenameOverride != "" {
		name := u.projectCfg.Download.FilenameOverride
		if u.projectCfg.Download.AddVersionToFilename {
			name = strings.ReplaceAll(name, "{version}", version)
		}
		return name
	}
	// Extract filename from URL
	parts := strings.Split(dlURL, "/")
	return parts[len(parts)-1]
}

func isArchive(ext string) bool {
	switch ext {
	case ".zip", ".tar.gz", ".tgz", ".tar.xz", ".txz":
		return true
	}
	return false
}

// Ensure platform is used
var _ = platform.ArchName
