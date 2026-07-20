// Package extractor handles file decompression and extraction.
package extractor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/mholt/archives"
)

type skipper interface {
	shouldSkipFile(string) bool
}

// Decompressor handles decompression by dispatching to the appropriate
// archive format, which is auto-detected via github.com/mholt/archives.
type Decompressor struct {
	cfg       config.DecompressConfig
	f         *os.File
	extract   archives.Extractor
	install   bool   // install mode (vs re-update) — controls exclude_when_update
	imageName string // image_name used for single-file rename fallback
	logger    *slog.Logger
}

// New creates a new Decompressor with the given decompress config. install
// mirrors updater-rpc's install flag (controls exclude_file_type_when_update
// application, gap #11). imageName is the process image_name used for the
// single-file rename fallback (gap #13).
func New(ctx context.Context, srcPath string, cfg config.DecompressConfig, install bool, imageName string, logger *slog.Logger) (*Decompressor, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", srcPath, err)
	}

	// Reset to start for format identification.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, fmt.Errorf("seek %s: %w", srcPath, err)
	}

	format, _, err := archives.Identify(ctx, filepath.Base(srcPath), f)
	switch err {
	case nil:
		arc, _ := format.(archives.Extractor)
		logger.Info("archive format detected",
			"path", srcPath,
			"format", fmt.Sprintf("%T", format),
			"reason", "archives.Identify matched a known archive format",
			"result", fmt.Sprintf("%T", format),
		)
		return &Decompressor{cfg: cfg, f: f, extract: arc, install: install, imageName: imageName, logger: logger}, nil
	case archives.NoMatch:
		logger.Info("archive format not detected",
			"path", srcPath,
			"reason", "archives.Identify returned NoMatch, treat as plain file copy",
			"result", "no extractor",
		)
		return &Decompressor{cfg: cfg, f: f, install: install, imageName: imageName, logger: logger}, nil
	default:
		f.Close()
		return nil, fmt.Errorf("identify %s: %w", srcPath, err)
	}
}

func (d *Decompressor) Close() error {
	return d.f.Close()
}

// log returns the decompressor's logger, falling back to slog.Default when nil
// (e.g. in unit tests that construct a bare Decompressor struct literal).
func (d *Decompressor) log() *slog.Logger {
	if d.logger != nil {
		return d.logger
	}
	return slog.Default()
}

// Extract decompresses the given file to the destination directory.
func (d *Decompressor) Extract(ctx context.Context, destDir string) error {
	if d.cfg.Skip.Bool() {
		d.log().Info("extraction skipped",
			"dest", destDir,
			"reason", "decompress.skip enabled",
			"result", "skip",
		)
		return nil
	}

	// clean_install: remove existing files in dest before extraction
	if d.cfg.CleanInstall {
		d.log().Info("clean install",
			"dest", destDir,
			"reason", "clean_install enabled, remove existing files first",
			"result", "begin",
		)
		if err := cleanInstall(destDir); err != nil {
			return fmt.Errorf("clean_install: %w", err)
		}
	}

	// use_system_package_manager: install .deb/.rpm with dpkg/rpm on linux,
	// skipping normal extraction (gap #2).
	if d.cfg.UseSystemPackageManager {
		return d.installWithSystemPackageManager(destDir)
	}

	// Resolve the single_dir handling (gap #12). When single_dir is a string it
	// is a fixed prefix directory inside the archive; when it is a bool (true)
	// the single top-level subdirectory is auto-detected and collapsed into the
	// destination; when false there is none (extract directly to destDir).
	var prefix string
	if d.cfg.SingleDir.IsString {
		prefix = d.cfg.SingleDir.StringVal
	}

	// exclude_file_type_when_update is appended to the exclude list only on
	// re-update (NOT install mode) — mirrors updater-rpc (gap #11).
	exclude := append([]string{}, d.cfg.ExcludeFileType...)
	if !d.install {
		exclude = append(exclude, d.cfg.ExcludeFileTypeWhenUpdate...)
	}
	include := d.cfg.IncludeFileType

	// The inner-file selector predicate (prefix + include + exclude) applied
	// during extraction (gaps #10, #11, #12).
	sel := &innerSelector{prefix: prefix, include: include, exclude: exclude, logger: d.log()}

	switch {
	case d.cfg.SingleDir.IsString:
		return d.extractWithSingleDir(ctx, prefix, sel, destDir)
	case d.cfg.SingleDir.Bool():
		return d.extractWithSingleDirAuto(ctx, sel, destDir)
	}

	d.log().Info("extraction mode",
		"dest", destDir,
		"reason", "no single_dir, extract directly to dest",
		"result", "direct",
	)
	if err := d.extractFile(ctx, destDir, sel); err != nil {
		return err
	}

	// Single-file rename fallback (gpu-z, gap #13): when exactly one file was
	// extracted and image_name is set, rename it to image_name.
	if d.imageName != "" {
		selected := sel.extracted
		if len(selected) == 1 {
			extracted := filepath.Join(destDir, selected[0])
			target := filepath.Join(destDir, d.imageName)
			if extracted != target {
				if err := os.Rename(extracted, target); err != nil {
					return fmt.Errorf("rename single extracted file: %w", err)
				}
				d.log().Info("single file renamed",
					"from", extracted,
					"to", target,
					"reason", "exactly one file selected, renamed to image_name",
					"result", "renamed",
				)
			}
		}
	}
	return nil
}

// extractWithSingleDir extracts to a temp dir, applies the inner selector, then
// copies the contents of the prefix subdir into destDir (gap #12). When prefix
// is a fixed string from single_dir config, only files under that prefix are
// extracted and moved; when prefix is auto-detected (single_dir: true) the
// common top-level directory is flattened into destDir.
func (d *Decompressor) extractWithSingleDir(ctx context.Context, prefix string, sel *innerSelector, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "updater-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	d.log().Info("extraction mode",
		"dest", destDir,
		"single_dir", d.cfg.SingleDir.String(),
		"prefix", prefix,
		"reason", "single_dir enabled, extract to temp then move prefix subtree",
		"result", "single_dir",
	)

	if err := d.extractFile(ctx, tmpDir, sel); err != nil {
		return err
	}

	srcPrefix := tmpDir
	if prefix != "" {
		srcPrefix = filepath.Join(tmpDir, prefix)
	}

	// If the prefix directory doesn't exist (e.g. no files matched the inner
	// selector) there is nothing to move.
	if _, err := os.Stat(srcPrefix); err != nil {
		d.log().Warn("single_dir prefix missing after extraction",
			"prefix", prefix,
			"reason", "no extracted files under prefix, nothing to move",
			"result", "skip move",
		)
		return nil
	}

	return moveDirContents(srcPrefix, destDir)
}

// extractWithSingleDirAuto implements single_dir: true. It extracts the
// archive to a temp dir, then if there is exactly one subdirectory (or a common
// top-level directory) at the archive root, collapses its contents into destDir
// (mirroring updater-rpc's getPrefixDir() behaviour, gap #12).
func (d *Decompressor) extractWithSingleDirAuto(ctx context.Context, sel *innerSelector, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "updater-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	d.log().Info("extraction mode",
		"dest", destDir,
		"single_dir", "true",
		"reason", "single_dir enabled, extract to temp then flatten single top dir",
		"result", "single_dir",
	)

	if err := d.extractFile(ctx, tmpDir, sel); err != nil {
		return err
	}

	// Detect a single top-level subdirectory. If multiple entries or none, we
	// cannot safely collapse, so move everything verbatim (best effort).
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read temp dir: %w", err)
	}
	var subDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			subDirs = append(subDirs, entry.Name())
		}
	}

	if len(subDirs) == 1 {
		srcPrefix := filepath.Join(tmpDir, subDirs[0])
		return moveDirContents(srcPrefix, destDir)
	}

	d.log().Warn("single_dir auto detection found no single top dir",
		"dir_count", len(subDirs),
		"reason", "fallback to moving all extracted content verbatim",
		"result", "move all",
	)
	return moveDirContents(tmpDir, destDir)
}

// innerSelector filters archive entries by prefix, include, and exclude file
// types, mirroring updater-rpc's file_sel (gaps #10, #11, #12). It also tracks
// the relative paths of files that were actually extracted (for the single
// file rename fallback, gap #13).
type innerSelector struct {
	prefix    string
	include   []string
	exclude   []string
	logger    *slog.Logger
	extracted []string
}

func (s *innerSelector) shouldSkipFile(name string) bool {
	// Prefix constraint: file must live under the single_dir prefix.
	if s.prefix != "" {
		pfx := strings.TrimSuffix(s.prefix, "/")
		if !strings.HasPrefix(name, pfx+"/") && name != pfx {
			return true
		}
	}
	lower := strings.ToLower(name)
	// Exclude by file type.
	for _, ext := range s.exclude {
		if strings.HasSuffix(lower, strings.ToLower(ext)) {
			return true
		}
	}
	// Include by file type (only when include list is non-empty).
	if len(s.include) > 0 {
		matched := false
		for _, inc := range s.include {
			if strings.HasSuffix(lower, strings.ToLower(inc)) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}
	return false
}

// installWithSystemPackageManager installs a .deb/.rpm via dpkg/rpm on linux,
// mirroring updater-rpc's use_system_package_manager branch (gap #2).
func (d *Decompressor) installWithSystemPackageManager(destDir string) error {
	if runtime.GOOS != "linux" {
		d.log().Warn("use_system_package_manager skipped",
			"reason", "option only works on linux",
			"result", "skip",
		)
		return nil
	}
	pkgPath := d.f.Name()
	if _, err := os.Stat(pkgPath); err != nil {
		return fmt.Errorf("package file missing: %w", err)
	}

	bin := which("dpkg")
	var cmd *exec.Cmd
	if bin != "" {
		cmd = exec.Command(bin, "-i", "--force-confdef", pkgPath)
	} else {
		bin = which("rpm")
		if bin == "" {
			d.log().Warn("use_system_package_manager skipped",
				"reason", "no dpkg or rpm found",
				"result", "skip",
			)
			return nil
		}
		cmd = exec.Command(bin, "-ivh", pkgPath)
	}

	cmd.Stdout = nil
	cmd.Stderr = nil
	d.log().Info("installing system package",
		"bin", bin,
		"package", pkgPath,
		"reason", "use_system_package_manager enabled on linux",
		"result", "begin",
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("system package install: %w", err)
	}
	return nil
}

// which returns the absolute path of an executable, or "" if not found.
func which(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// extractFile extracts (or copies) srcPath into destDir, auto-detecting the
// archive format. Non-archive files are copied verbatim into destDir.
func (d *Decompressor) extractFile(ctx context.Context, destDir string, skip skipper) error {
	if d.extract == nil {
		return copyFile(d.f.Name(), filepath.Join(destDir, filepath.Base(d.f.Name())))
	}
	return d.extract.Extract(ctx, d.f, makeHandler(destDir, skip, d.log()))
}

// makeHandler returns an archives.FileHandler that writes each archive entry
// into destDir, honoring the skip filter and guarding against path traversal.
// When the skip filter is an *innerSelector, successfully extracted file
// relative paths are recorded for the single-file rename fallback.
func makeHandler(destDir string, skip skipper, logger *slog.Logger) archives.FileHandler {
	return func(ctx context.Context, fi archives.FileInfo) error {
		name := fi.NameInArchive
		if skip != nil && skip.shouldSkipFile(name) {
			logger.Debug("file skipped during extract",
				"name", name,
				"reason", "matched exclude/include/prefix filter",
				"result", "skip",
			)
			return nil
		}

		target := filepath.Join(destDir, name)

		// Security: prevent archive slip / path traversal.
		if !safePath(target, destDir) {
			return fmt.Errorf("invalid archive entry: %s", name)
		}

		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := fi.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, rc); err != nil {
			return err
		}

		if sel, ok := skip.(*innerSelector); ok && !fi.IsDir() {
			sel.extracted = append(sel.extracted, name)
		}
		return nil
	}
}
