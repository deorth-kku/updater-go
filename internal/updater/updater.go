// Package updater orchestrates the full update flow: version check → download → extract → copy → post-cmds.
package updater

import (
	"context"
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
	"github.com/deorth-kku/updater-go/internal/peversion"
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
	entry      config.ProjectEntry
	force      bool
	dl         downloader.Downloader
	httpDL     api.Downloader
	logger     *slog.Logger
}

// New creates a new Updater.
func New(cfg config.ProjectConfig, entry config.ProjectEntry, force bool, dl downloader.Downloader, httpDL api.Downloader, logger *slog.Logger) *Updater {
	return &Updater{
		projectCfg: cfg,
		entry:      entry,
		force:      force,
		dl:         dl,
		httpDL:     httpDL,
		logger:     logger,
	}
}

// log returns the updater's logger, falling back to slog.Default when nil
// (e.g. in unit tests that construct a bare Updater).
func (u *Updater) log() *slog.Logger {
	if u.logger != nil {
		return u.logger
	}
	return slog.Default()
}

// replaceVars replaces %PATH, %NAME, %DL_FILENAME, %VER in a string.
func replaceVars(s, path, name, dlFilename, version string) string {
	s = strings.ReplaceAll(s, "%PATH", path)
	s = strings.ReplaceAll(s, "%NAME", name)
	s = strings.ReplaceAll(s, "%DL_FILENAME", dlFilename)
	s = strings.ReplaceAll(s, "%VER", version)
	return s
}

// exePath resolves the installed executable path used for use_exe_version.
func (u *Updater) exePath() string {
	image := u.projectCfg.Process.ImageName
	if image == "" {
		image = u.projectCfg.Basic.ProjectName
	}
	p := filepath.Join(u.entry.SavePath, image)
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(p), ".exe") {
		p += ".exe"
	}
	return p
}

// needUpdateByExe implements updater-rpc's use_exe_version comparison. It
// returns (true, reason) when an update should proceed. When the exe is
// missing it is treated as a fresh install (always update). When the exe has
// no version resource we also update (mirrors the Python install-mode branch).
func (u *Updater) needUpdateByExe(remote string) (bool, string) {
	exepath := u.exePath()
	if _, err := os.Stat(exepath); err != nil {
		return true, "installed exe missing, treat as install"
	}
	fileVer, prodVer, err := peversion.FileVersion(exepath)
	if err != nil {
		u.log().Warn("read exe version failed",
			"project", u.projectCfg.Basic.ProjectName,
			"path", exepath,
			"error", err,
			"reason", "fall back to install mode",
			"result", "warn",
		)
		return true, "failed to read exe version, treat as install"
	}
	// Mirrors Python: no VS_FIXEDFILEINFO -> install mode (always update).
	if fileVer == (peversion.Version{}) && prodVer == (peversion.Version{}) {
		return true, "installed exe has no version resource, treat as install"
	}
	if !peversion.NeedsUpdate(remote, fileVer, prodVer) {
		return false, "remote version not newer than installed exe FileVersion/ProductVersion"
	}
	return true, "remote version newer than installed exe FileVersion/ProductVersion"
}

// Update runs the full update flow for the project.
func (u *Updater) Update(ctx context.Context) *UpdateResult {
	result := &UpdateResult{ProjectName: u.projectCfg.Basic.ProjectName, OldVersion: u.entry.Version}
	// Step 1: Detect latest version via API
	apiAdapter, err := api.NewAPI(u.projectCfg.Basic, u.projectCfg.Download, u.projectCfg.Version, u.projectCfg.Build, u.httpDL, u.log())
	if err != nil {
		result.Error = fmt.Errorf("create api: %w", err)
		return result
	}
	u.log().Info("api backend selected",
		"project", result.ProjectName,
		"api_type", u.projectCfg.Basic.APIType,
		"reason", "backend chosen from config api_type",
		"result", "ok",
	)

	rel, err := apiAdapter.Latest(ctx)
	if err != nil {
		result.Error = fmt.Errorf("fetch latest: %w", err)
		return result
	}
	result.NewVersion = rel.Version
	u.log().Debug("latest version detected",
		"project", result.ProjectName,
		"version", rel.Version,
		"assets", len(rel.Assets),
		"reason", "queried backend Latest",
		"result", rel.Version,
	)

	// Step 2: Check if update is needed.
	//
	// Mirrors updater-rpc's `run`: the decision is `checkIfUpdateIsNeed(...)
	// or force`. So force takes precedence over everything: when set we
	// always proceed regardless of version. Only when force is off do we
	// fall into the version-specific checks.
	if u.force {
		u.log().Info("update needed",
			"project", result.ProjectName,
			"old_version", result.OldVersion,
			"new_version", rel.Version,
			"force", u.force,
			"reason", "force enabled",
			"result", "proceed",
		)
	} else if u.projectCfg.Version.UseExeVersion {
		// use_exe_version: instead of comparing against the recorded
		// currentVersion, read the binary FileVersion / ProductVersion
		// straight from the installed exe (Windows PE only). This mirrors
		// updater-rpc's checkIfUpdateIsNeed: if the exe is missing we treat
		// it as a fresh install; otherwise an update is needed only when the
		// remote version is strictly greater than BOTH the installed
		// FileVersion and ProductVersion.
		need, reason := u.needUpdateByExe(rel.Version)
		if !need {
			u.log().Info("no update needed",
				"project", result.ProjectName,
				"version", rel.Version,
				"reason", reason,
				"result", "skip",
			)
			return result
		}
		u.log().Info("update needed",
			"project", result.ProjectName,
			"old_version", result.OldVersion,
			"new_version", rel.Version,
			"force", u.force,
			"reason", reason,
			"result", "proceed",
		)
	} else { // generic comparison
		if rel.Version == result.OldVersion {
			u.log().Info("no update needed",
				"project", result.ProjectName,
				"version", rel.Version,
				"reason", "detected version equals installed version and force is off",
				"result", "skip",
			)
			return result
		}
		u.log().Info("update needed",
			"project", result.ProjectName,
			"old_version", result.OldVersion,
			"new_version", rel.Version,
			"force", u.force,
			"reason", "detected version differs from installed",
			"result", "proceed",
		)
	}

	// Step 3: Select download URL
	dlURL := u.selectDownloadURL(rel)
	if dlURL == "" {
		result.Error = fmt.Errorf("no matching download URL found")
		return result
	}

	// Step 4: Download
	filename := u.downloadFilename(rel.Version, dlURL)
	saveDir := filepath.Join(u.entry.SavePath, u.entry.Name)
	u.log().Info("starting download",
		"project", result.ProjectName,
		"url", dlURL,
		"filename", filename,
		"save_dir", saveDir,
		"reason", "download URL and filename resolved",
		"result", "begin",
	)
	localPath, _, err := u.dl.Download(ctx, dlURL, filename, saveDir)
	if err != nil {
		result.Error = fmt.Errorf("download: %w", err)
		return result
	}
	result.Downloaded = localPath
	u.log().Info("download finished",
		"project", result.ProjectName,
		"path", localPath,
		"reason", "downloader reported completion",
		"result", localPath,
	)

	// Step 5: Extract
	if !u.projectCfg.Decompress.Skip.Bool() {
		u.log().Info("extracting archive",
			"project", result.ProjectName,
			"path", localPath,
			"reason", "decompress not skipped",
			"result", "begin",
		)
		ex, err := extractor.New(ctx, localPath, u.projectCfg.Decompress, u.log().With("comp", "extractor"))
		if err != nil {
			result.Error = fmt.Errorf("detect format %w", err)
			return result
		}
		if err := ex.Extract(ctx, u.entry.SavePath); err != nil {
			ex.Close()
			result.Error = fmt.Errorf("extract: %w", err)
			return result
		}
		ex.Close()
		result.Extracted = true
		u.log().Info("extraction finished",
			"project", result.ProjectName,
			"save_path", u.entry.SavePath,
			"reason", "archive extracted to save path",
			"result", "ok",
		)

		// Delete archive unless keep_download_file is true
		if !u.projectCfg.Decompress.KeepDownloadFile {
			if err := os.Remove(localPath); err != nil {
				u.log().Warn("failed to remove download file",
					"project", result.ProjectName,
					"path", localPath,
					"error", err,
					"reason", "keep_download_file is false",
					"result", "skip remove",
				)
			} else {
				u.log().Debug("removed download file",
					"project", result.ProjectName,
					"path", localPath,
					"reason", "keep_download_file is false",
					"result", "removed",
				)
			}
		}
	} else {
		u.log().Info("extraction skipped",
			"project", result.ProjectName,
			"reason", "decompress.skip enabled",
			"result", "skip",
		)
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
			u.log(),
		)

		// Stop process
		if u.projectCfg.Process.StopCmd != "" {
			u.log().Info("stopping process",
				"project", result.ProjectName,
				"image", imageName,
				"reason", "custom stop_cmd configured, takes priority over service/image",
				"result", "run stop_cmd",
			)
			if err := ctrl.Stop(ctx); err != nil {
				u.log().Warn("stop command failed", "project", result.ProjectName, "error", err)
			}
		} else if u.projectCfg.Process.Service {
			u.log().Info("stopping process",
				"project", result.ProjectName,
				"image", imageName,
				"reason", "service mode enabled, no custom stop_cmd",
				"result", "stop service",
			)
			if err := ctrl.Stop(ctx); err != nil {
				u.log().Warn("stop failed", "project", result.ProjectName, "error", err)
			}
		} else {
			u.log().Info("stopping process",
				"project", result.ProjectName,
				"image", imageName,
				"reason", "no stop_cmd and no service, terminate by image name",
				"result", "kill image",
			)
			if err := ctrl.Stop(ctx); err != nil {
				u.log().Warn("stop failed", "project", result.ProjectName, "error", err)
			}
		}

		// Start process
		if u.projectCfg.Process.StartCmd != "" {
			u.log().Info("starting process",
				"project", result.ProjectName,
				"image", imageName,
				"reason", "custom start_cmd configured, takes priority over service/image",
				"result", "run start_cmd",
			)
			if err := ctrl.Start(ctx, ""); err != nil {
				u.log().Warn("start command failed", "project", result.ProjectName, "error", err)
			}
		} else if u.projectCfg.Process.Service {
			u.log().Info("starting process",
				"project", result.ProjectName,
				"image", imageName,
				"reason", "service mode enabled, no custom start_cmd",
				"result", "start service",
			)
			if err := ctrl.Start(ctx, ""); err != nil {
				u.log().Warn("start failed", "project", result.ProjectName, "error", err)
			}
		} else {
			// Find the executable in the save path
			exePath := filepath.Join(u.entry.SavePath, imageName)
			if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(exePath), ".exe") {
				exePath += ".exe"
			}
			u.log().Info("starting process",
				"project", result.ProjectName,
				"image", imageName,
				"path", exePath,
				"reason", "no start_cmd and no service, launch executable by path",
				"result", "start binary",
			)
			if err := ctrl.Start(ctx, exePath); err != nil {
				u.log().Warn("start failed", "project", result.ProjectName, "error", err)
			}
		}
	}

	// Step 7: Post-cmds execution
	postCmds := u.getPostCmds()
	for _, cmd := range postCmds {
		replaced := replaceVars(cmd, u.entry.SavePath, result.ProjectName, filename, rel.Version)
		u.log().Info("running post-cmd",
			"project", result.ProjectName,
			"cmd", replaced,
			"reason", "post-update command configured",
			"result", "begin",
		)
		parts := strings.Fields(replaced)
		if len(parts) == 0 {
			continue
		}
		cmdObj := exec.Command(parts[0], parts[1:]...)
		cmdObj.Stdout = nil
		cmdObj.Stderr = nil
		if err := cmdObj.Run(); err != nil {
			u.log().Warn("post-cmd failed", "project", result.ProjectName, "error", err)
		}
	}

	u.log().Info("update completed",
		"project", result.ProjectName,
		"version", rel.Version,
		"downloaded", localPath,
		"extracted", result.Extracted,
		"reason", "all update steps finished",
		"result", "ok",
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

// selectDownloadURL picks the best download URL from a release.
func (u *Updater) selectDownloadURL(rel *api.Release) string {
	// If a direct URL is configured, use it
	if u.projectCfg.Download.URL != "" {
		u.log().Info("download URL selected",
			"project", u.projectCfg.Basic.ProjectName,
			"reason", "direct download.url configured, overrides asset matching",
			"result", u.projectCfg.Download.URL,
		)
		return u.projectCfg.Download.URL
	}

	// For GitHub releases, filter assets by keywords and index
	if len(rel.Assets) > 0 {
		fs := extractor.NewFileSelector(u.projectCfg.Download, u.projectCfg.Decompress, u.log().With("comp", "selector"))
		matched := fs.SelectFiles(assetNames(rel.Assets))
		u.log().Debug("assets matched",
			"project", u.projectCfg.Basic.ProjectName,
			"total", len(rel.Assets),
			"matched", len(matched),
			"reason", "file selector filtered release assets",
			"result", strings.Join(matched, ","),
		)
		// Apply index/indexes filtering
		if len(u.projectCfg.Download.Indexes) > 0 {
			var indexed []string
			for _, idx := range u.projectCfg.Download.Indexes {
				if idx >= 0 && idx < len(matched) {
					indexed = append(indexed, matched[idx])
				}
			}
			matched = indexed
			u.log().Debug("indexes applied",
				"project", u.projectCfg.Basic.ProjectName,
				"indexes", fmt.Sprintf("%v", u.projectCfg.Download.Indexes),
				"reason", "download.indexes configured",
				"result", strings.Join(matched, ","),
			)
		} else if u.projectCfg.Download.Index > 0 && u.projectCfg.Download.Index <= len(matched) {
			matched = matched[u.projectCfg.Download.Index-1:]
			u.log().Debug("index applied",
				"project", u.projectCfg.Basic.ProjectName,
				"index", u.projectCfg.Download.Index,
				"reason", "single download.index configured",
				"result", strings.Join(matched, ","),
			)
		}
		for _, name := range matched {
			for _, a := range rel.Assets {
				if a.Name == name {
					u.log().Info("download URL selected",
						"project", u.projectCfg.Basic.ProjectName,
						"asset", name,
						"reason", "matched asset chosen for download",
						"result", a.URL,
					)
					return a.URL
				}
			}
		}
	}

	// For AppVeyor artifacts
	if len(rel.Artifacts) > 0 {
		fs := extractor.NewFileSelector(u.projectCfg.Download, u.projectCfg.Decompress, u.log().With("comp", "selector"))
		matched := fs.SelectFiles(artifactNames(rel.Artifacts))
		u.log().Debug("artifacts matched",
			"project", u.projectCfg.Basic.ProjectName,
			"total", len(rel.Artifacts),
			"matched", len(matched),
			"reason", "file selector filtered appveyor artifacts",
			"result", strings.Join(matched, ","),
		)
		if len(u.projectCfg.Download.Indexes) > 0 {
			var indexed []string
			for _, idx := range u.projectCfg.Download.Indexes {
				if idx >= 0 && idx < len(matched) {
					indexed = append(indexed, matched[idx])
				}
			}
			matched = indexed
			u.log().Debug("indexes applied",
				"project", u.projectCfg.Basic.ProjectName,
				"indexes", fmt.Sprintf("%v", u.projectCfg.Download.Indexes),
				"reason", "download.indexes configured",
				"result", strings.Join(matched, ","),
			)
		} else if u.projectCfg.Download.Index > 0 && u.projectCfg.Download.Index <= len(matched) {
			matched = matched[u.projectCfg.Download.Index-1:]
			u.log().Debug("index applied",
				"project", u.projectCfg.Basic.ProjectName,
				"index", u.projectCfg.Download.Index,
				"reason", "single download.index configured",
				"result", strings.Join(matched, ","),
			)
		}
		for _, name := range matched {
			for _, art := range rel.Artifacts {
				if art.FileName == name {
					url := rel.BaseURL + "/buildjobs/" + rel.JobID + "/artifacts/" + art.FileName
					u.log().Info("download URL selected",
						"project", u.projectCfg.Basic.ProjectName,
						"artifact", name,
						"reason", "matched appveyor artifact chosen for download",
						"result", url,
					)
					return url
				}
			}
		}
	}

	// Fallback to the release URL
	if rel.URL != "" {
		u.log().Warn("download URL fallback",
			"project", u.projectCfg.Basic.ProjectName,
			"reason", "no asset/artifact matched, using release URL as last resort",
			"result", rel.URL,
		)
		return rel.URL
	}

	u.log().Warn("no download URL selected",
		"project", u.projectCfg.Basic.ProjectName,
		"reason", "no direct url, no matched asset/artifact, and no release url",
		"result", "",
	)
	return ""
}

// assetNames returns the names of all assets.
func assetNames(assets []api.Asset) []string {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	return names
}

// artifactNames returns the file names of all artifacts.
func artifactNames(artifacts []api.AppveyorArtifact) []string {
	names := make([]string, len(artifacts))
	for i, a := range artifacts {
		names[i] = a.FileName
	}
	return names
}

// downloadFilename determines the filename for the download.
func (u *Updater) downloadFilename(version, dlURL string) string {
	if u.projectCfg.Download.FilenameOverride != "" {
		name := u.projectCfg.Download.FilenameOverride
		if u.projectCfg.Download.AddVersionToFilename {
			name = strings.ReplaceAll(name, "{version}", version)
			name = strings.ReplaceAll(name, "%arch", runtime.GOARCH)
			name = strings.ReplaceAll(name, "%OS", runtime.GOOS)
		}
		u.log().Debug("download filename resolved",
			"project", u.projectCfg.Basic.ProjectName,
			"reason", "filename_override configured (version/arch/os substituted)",
			"result", name,
		)
		return name
	}
	// Extract filename from URL
	parts := strings.Split(dlURL, "/")
	name := parts[len(parts)-1]
	u.log().Debug("download filename resolved",
		"project", u.projectCfg.Basic.ProjectName,
		"reason", "no override, derived from last URL path segment",
		"result", name,
	)
	return name
}

// Ensure platform is used
var _ = platform.ArchName
