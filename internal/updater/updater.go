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
	"time"

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
				"version", rel.Version,
				"reason", reason,
				"result", "skip",
			)
			return result
		}
		u.log().Info("update needed",
			"old_version", result.OldVersion,
			"new_version", rel.Version,
			"force", u.force,
			"reason", reason,
			"result", "proceed",
		)
	} else { // generic comparison
		if rel.Version == result.OldVersion {
			u.log().Info("no update needed",
				"version", rel.Version,
				"reason", "detected version equals installed version and force is off",
				"result", "skip",
			)
			return result
		}
		u.log().Info("update needed",
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
		"url", dlURL,
		"filename", filename,
		"save_dir", saveDir,
		"reason", "download URL and filename resolved",
		"result", "begin",
	)
	localPath, _, err := u.dl.Download(ctx, dlURL, filename, saveDir, u.projectCfg.Basic.Headers)
	if err != nil {
		result.Error = fmt.Errorf("download: %w", err)
		return result
	}
	result.Downloaded = localPath
	u.log().Info("download finished",
		"path", localPath,
		"reason", "downloader reported completion",
		"result", localPath,
	)

	// Step 4.5: When restart is NOT allowed (gap #4), mirror updater-rpc's
	// `elif popup` branch: if the target process is still running, log a
	// warning and wait for it to exit before extracting, so we don't extract
	// over a running/locked file.
	if !u.projectCfg.Process.AllowRestart && u.projectCfg.Process.Popup {
		imageName := u.projectCfg.Process.ImageName
		if imageName == "" {
			imageName = result.ProjectName
		}
		ctrl := process.New(imageName, u.log().With("comp", "process"))
		if ctrl.IsRunning() {
			msg := fmt.Sprintf("waiting for process %s to stop so we can update %s",
				imageName, result.ProjectName)
			u.log().Warn(msg)
			if err := ctrl.PopupMsg("Updater", msg); err != nil {
				u.log().Warn("popup message failed",
					"error", err,
				)
			}
			if err := ctrl.WaitForStop(ctx, 5*time.Minute); err != nil {
				u.log().Warn("timed out waiting for process to stop",
					"image", imageName,
					"error", err,
				)
			}
		}
	}

	// Step 5: Extract
	if !u.projectCfg.Decompress.Skip.Bool() {
		u.log().Info("extracting archive",
			"path", localPath,
			"reason", "decompress not skipped",
			"result", "begin",
		)
		ex, err := extractor.New(ctx, localPath, u.projectCfg.Decompress, u.isInstallMode(), u.projectCfg.Process.ImageName, u.log().With("comp", "extractor"))
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
			"save_path", u.entry.SavePath,
			"reason", "archive extracted to save path",
			"result", "ok",
		)

		// Delete archive unless keep_download_file is true
		if !u.projectCfg.Decompress.KeepDownloadFile {
			if err := os.Remove(localPath); err != nil {
				u.log().Warn("failed to remove download file",
					"path", localPath,
					"error", err,
					"reason", "keep_download_file is false",
					"result", "skip remove",
				)
			} else {
				u.log().Debug("removed download file",
					"path", localPath,
					"reason", "keep_download_file is false",
					"result", "removed",
				)
			}
		}
	} else {
		u.log().Info("extraction skipped",
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
			u.entry.SavePath,
			u.projectCfg.Process.StopCmd,
			u.projectCfg.Process.StartCmd,
			u.projectCfg.Process.Service,
			u.projectCfg.Process.RestartWait,
			u.log(),
		)

		// Stop process
		stopped, err := ctrl.Stop(ctx)
		if err != nil {
			u.log().Warn("stop failed", "error", err)
			return result
		}
		if !stopped {
			u.log().Info("no process running, skip start",
				"image", imageName,
				"reason", "stop found nothing to stop",
				"result", "skip start",
			)
			return result
		}

		// Start process
		if err := ctrl.Start(ctx); err != nil {
			u.log().Warn("start failed", "error", err)
		}
	}

	// Step 7: Post-cmds execution (gap #1). Mirror updater-rpc's main.py loop:
	// each command has %PATH/%NAME/%DL_FILENAME/%VER replaced, then executed
	// via the system shell (os.system), so quoting and shell features behave
	// identically. %DL_FILENAME maps to the downloaded file path (Python's
	// obj.fullfilename); %PATH is wrapped in double quotes as in the Python
	// implementation.
	postCmds := u.projectCfg.PostCmds
	for _, line := range postCmds {
		replaced := replaceVars(line, u.entry.SavePath, result.ProjectName, localPath, rel.Version)
		u.log().Info("running post-cmd",
			"cmd", replaced,
			"reason", "post-update command configured",
			"result", "begin",
		)
		if replaced == "" {
			continue
		}
		var cmdObj *exec.Cmd
		if runtime.GOOS == "windows" {
			cmdObj = exec.Command("cmd", "/c", replaced)
		} else {
			cmdObj = exec.Command("sh", "-c", replaced)
		}
		cmdObj.Stdout = nil
		cmdObj.Stderr = nil
		if err := cmdObj.Run(); err != nil {
			u.log().Warn("post-cmd failed", "error", err)
		}
	}

	// Step 8: Write .VERSION file (gap #3). Mirrors updater-rpc's
	// updateVersionFile(): when not using use_exe_version, write the detected
	// version string to <save_path>/<name>.VERSION.
	if !u.projectCfg.Version.UseExeVersion {
		if err := u.updateVersionFile(rel.Version); err != nil {
			u.log().Warn("write .VERSION file failed",
				"error", err,
			)
		}
	}

	u.log().Info("update completed",
		"version", rel.Version,
		"downloaded", localPath,
		"extracted", result.Extracted,
		"reason", "all update steps finished",
		"result", "ok",
	)

	return result
}

// updateVersionFile writes the detected version to <save_path>/<name>.VERSION,
// mirroring updater-rpc's updateVersionFile() for non-use_exe_version projects.
func (u *Updater) updateVersionFile(version string) error {
	versionFilePath := filepath.Join(u.entry.SavePath, u.projectCfg.Basic.ProjectName+".VERSION")
	if err := os.MkdirAll(u.entry.SavePath, 0o755); err != nil {
		return fmt.Errorf("mkdir save path: %w", err)
	}
	if err := os.WriteFile(versionFilePath, []byte(version), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", versionFilePath, err)
	}
	u.log().Info("version file written",
		"path", versionFilePath,
		"version", version,
		"reason", "use_exe_version is false",
		"result", "ok",
	)
	return nil
}

// selectDownloadURL picks the best download URL from a release.
func (u *Updater) selectDownloadURL(rel *api.Release) string {
	// If a direct URL is configured, use it
	if u.projectCfg.Download.URL != "" {
		// %VER global replacement (gap #25): the configured URL may embed
		// %VER which must be expanded to the detected version.
		url := strings.ReplaceAll(u.projectCfg.Download.URL, "%VER", rel.Version)
		u.log().Info("download URL selected",
			"reason", "direct download.url configured, overrides asset matching",
			"result", url,
		)
		return url
	}

	// For GitHub releases, filter assets by keywords and index
	if len(rel.Assets) > 0 {
		fs := extractor.NewFileSelector(u.projectCfg.Download, u.projectCfg.Decompress, u.isInstallMode(), u.log().With("comp", "selector"))
		matched := fs.SelectFiles(assetNames(rel.Assets))
		u.log().Debug("assets matched",
			"total", len(rel.Assets),
			"matched", len(matched),
			"reason", "file selector filtered release assets",
			"result", strings.Join(matched, ","),
		)
		// Apply index/indexes filtering
		matched = u.applyIndex(matched)
		u.log().Debug("index applied",
			"index", u.projectCfg.Download.Index,
			"indexes", fmt.Sprintf("%v", u.projectCfg.Download.Indexes),
			"reason", "matched assets filtered by download.index/indexes",
			"result", strings.Join(matched, ","),
		)
		for _, name := range matched {
			for _, a := range rel.Assets {
				if a.Name == name {
					u.log().Info("download URL selected",
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
		fs := extractor.NewFileSelector(u.projectCfg.Download, u.projectCfg.Decompress, u.isInstallMode(), u.log().With("comp", "selector"))
		matched := fs.SelectFiles(artifactNames(rel.Artifacts))
		u.log().Debug("artifacts matched",
			"total", len(rel.Artifacts),
			"matched", len(matched),
			"reason", "file selector filtered appveyor artifacts",
			"result", strings.Join(matched, ","),
		)
		matched = u.applyIndex(matched)
		u.log().Debug("index applied",
			"index", u.projectCfg.Download.Index,
			"indexes", fmt.Sprintf("%v", u.projectCfg.Download.Indexes),
			"reason", "matched artifacts filtered by download.index/indexes",
			"result", strings.Join(matched, ","),
		)
		for _, name := range matched {
			for _, art := range rel.Artifacts {
				if art.FileName == name {
					url := rel.BaseURL + "/buildjobs/" + rel.JobID + "/artifacts/" + art.FileName
					u.log().Info("download URL selected",
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
			"reason", "no asset/artifact matched, using release URL as last resort",
			"result", rel.URL,
		)
		return rel.URL
	}

	u.log().Warn("no download URL selected",
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

// isInstallMode mirrors updater-rpc's install flag used to decide whether the
// update_keyword branch is active (gap #9). For use_exe_version projects,
// install mode is true when the installed exe is missing; otherwise it is true
// when there is no recorded currentVersion.
func (u *Updater) isInstallMode() bool {
	if u.projectCfg.Version.UseExeVersion {
		_, err := os.Stat(u.exePath())
		return err != nil
	}
	return u.entry.Version == ""
}

// applyIndex narrows a list of matched filenames to the element(s) selected by
// the download.index / download.indexes config, mirroring updater-rpc's
// getDlUrl behaviour for github/appveyor backends.
//
// When download.indexes is set, each entry selects one filename (0-based).
// Otherwise, when download.index is set, only the single filename at that
// 0-based position is returned (Python returns match_urls[index] — a single
// element, not a slice from that position onward).
func (u *Updater) applyIndex(matched []string) []string {
	if len(u.projectCfg.Download.Indexes) > 0 {
		var indexed []string
		for _, idx := range u.projectCfg.Download.Indexes {
			if idx >= 0 && idx < len(matched) {
				indexed = append(indexed, matched[idx])
			}
		}
		return indexed
	}
	if u.projectCfg.Download.Index > 0 && u.projectCfg.Download.Index < len(matched) {
		// Python is 0-based: match_urls[index]. Return exactly that element.
		return matched[u.projectCfg.Download.Index : u.projectCfg.Download.Index+1]
	}
	return matched
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
	// %VER global replacement (gap #25): mirror updater-rpc's var_replace
	// applied to the whole config after version detection.
	version = strings.ReplaceAll(version, "%VER", version)
	if u.projectCfg.Download.FilenameOverride != "" {
		name := u.projectCfg.Download.FilenameOverride
		usedVersionToken := strings.Contains(name, "{version}")
		if u.projectCfg.Download.AddVersionToFilename {
			name = strings.ReplaceAll(name, "{version}", version)
			name = strings.ReplaceAll(name, "%arch", runtime.GOARCH)
			name = strings.ReplaceAll(name, "%OS", runtime.GOOS)
		}
		// %VER may also appear verbatim in the override (gap #25).
		name = strings.ReplaceAll(name, "%VER", version)
		// gap #7: add_version_to_filename also applies to the override name,
		// but only when the {version} token wasn't already used for placement.
		if u.projectCfg.Download.AddVersionToFilename && !usedVersionToken {
			name = addVersionToName(name, version, u.projectCfg.Download.Filetype)
		}
		u.log().Debug("download filename resolved",
			"reason", "filename_override configured (version/arch/os substituted)",
			"result", name,
		)
		return name
	}
	// Extract filename from URL
	parts := strings.Split(dlURL, "/")
	name := parts[len(parts)-1]
	// gap #7: mirror updater-rpc's download() — insert the sanitized version
	// before the filetype extension even for URL-derived filenames.
	if u.projectCfg.Download.AddVersionToFilename {
		name = addVersionToName(name, version, u.projectCfg.Download.Filetype)
	}
	u.log().Debug("download filename resolved",
		"reason", "no override, derived from last URL path segment",
		"result", name,
	)
	return name
}

// sanitizeVersion replaces characters disallowed in filenames (mirrors
// updater-rpc's download() loop over < > / \ | : * ?).
func sanitizeVersion(v string) string {
	return strings.NewReplacer(
		"<", " ", ">", " ", "/", " ", "\\", " ",
		"|", " ", ":", " ", "*", " ", "?", " ",
	).Replace(v)
}

// addVersionToName inserts a sanitized version into the filename, placed right
// before the matching filetype extension. Mirrors updater-rpc's download():
// strip the trailing filetype, rstrip a dot, append "_<version>.<filetype>".
// If no configured filetype matches the name, the name is returned unchanged.
func addVersionToName(name, version string, filetypes []string) string {
	version = sanitizeVersion(version)
	for _, ft := range filetypes {
		if strings.HasSuffix(name, ft) {
			base := strings.TrimSuffix(name, ft)
			base = strings.TrimSuffix(base, ".")
			return base + "_" + version + "." + ft
		}
	}
	return name
}

// Ensure platform is used
var _ = platform.ArchName
