// Package updater orchestrates the full update flow: version check → download → extract → copy → post-cmds.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deorth-kku/updater-go/internal/api"
	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/downloader"
	"github.com/deorth-kku/updater-go/internal/extractor"
	"github.com/deorth-kku/updater-go/internal/platform"
	"github.com/deorth-kku/updater-go/internal/process"
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

// replaceVars replaces %PATH, %NAME, %DL_FILENAME, %VER in a string.
func replaceVars(s, path, name, dlFilename, version string) string {
	s = strings.ReplaceAll(s, "%PATH", path)
	s = strings.ReplaceAll(s, "%NAME", name)
	s = strings.ReplaceAll(s, "%DL_FILENAME", dlFilename)
	s = strings.ReplaceAll(s, "%VER", version)
	return s
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

	// Step 6: Process management (stop/start if allow_restart)
	if u.projectCfg.Process.AllowRestart {
		imageName := u.projectCfg.Process.ImageName
		if imageName == "" {
			imageName = result.ProjectName
		}

		ctrl := process.NewWithConfig(
			imageName,
			u.projectCfg.Process.StopCmd,
			u.projectCfg.Process.StartCmd,
			u.projectCfg.Process.Service,
			u.projectCfg.Process.RestartWait,
		)

		// Stop process
		if u.projectCfg.Process.StopCmd != "" {
			u.logger.Info("running custom stop command", "project", result.ProjectName)
			if err := ctrl.Stop(ctx); err != nil {
				u.logger.Warn("stop command failed", "project", result.ProjectName, "error", err)
			}
		} else {
			u.logger.Info("stopping process", "project", result.ProjectName, "image", imageName)
			if err := ctrl.Stop(ctx); err != nil {
				u.logger.Warn("stop failed", "project", result.ProjectName, "error", err)
			}
		}

		// Start process
		if u.projectCfg.Process.StartCmd != "" {
			u.logger.Info("running custom start command", "project", result.ProjectName)
			if err := ctrl.Start(ctx, ""); err != nil {
				u.logger.Warn("start command failed", "project", result.ProjectName, "error", err)
			}
		} else {
			// Find the executable in the save path
			exePath := filepath.Join(u.savePath, result.ProjectName, imageName)
			if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(exePath), ".exe") {
				exePath += ".exe"
			}
			u.logger.Info("starting process", "project", result.ProjectName, "path", exePath)
			if err := ctrl.Start(ctx, exePath); err != nil {
				u.logger.Warn("start failed", "project", result.ProjectName, "error", err)
			}
		}
	}

	// Step 7: Post-cmds execution
	postCmds := u.getPostCmds()
	for _, cmd := range postCmds {
		replaced := replaceVars(cmd, u.savePath, result.ProjectName, filename, rel.Version)
		u.logger.Info("running post-cmd", "project", result.ProjectName, "cmd", replaced)
		parts := strings.Fields(replaced)
		if len(parts) == 0 {
			continue
		}
		cmdObj := exec.Command(parts[0], parts[1:]...)
		cmdObj.Stdout = nil
		cmdObj.Stderr = nil
		if err := cmdObj.Run(); err != nil {
			u.logger.Warn("post-cmd failed", "project", result.ProjectName, "error", err)
		}
	}

	// Step 8: Config writeback — save current version
	if err := u.writeConfigBack(result.ProjectName, rel.Version); err != nil {
		u.logger.Warn("config writeback failed", "project", result.ProjectName, "error", err)
	}

	u.logger.Info("update completed",
		"project", result.ProjectName,
		"version", rel.Version,
		"downloaded", localPath,
	)

	return result
}

// getPostCmds returns post-update commands from the project config.
// This is a placeholder — the Python version uses a "post_cmd" field.
func (u *Updater) getPostCmds() []string {
	// The Python config has post_cmd as a list of strings in the project config.
	// For now, return empty — this can be extended when the field is added.
	return nil
}

// writeConfigBack writes the current version back to the project config file.
func (u *Updater) writeConfigBack(projectName, version string) error {
	configPath := filepath.Join(filepath.Dir(u.savePath), "config", projectName+".json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", configPath, err)
	}

	var pc config.ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return fmt.Errorf("parse config %s: %w", configPath, err)
	}

	pc.CurrentVersion = version

	out, err := json.MarshalIndent(pc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", configPath, err)
	}

	return nil
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
