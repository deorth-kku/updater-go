// Package extractor handles file decompression and extraction.
package extractor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/mholt/archives"
)

type skipper interface {
	shouldSkipFile(string) bool
}

// Decompressor handles decompression by dispatching to the appropriate
// archive format, which is auto-detected via github.com/mholt/archives.
type Decompressor struct {
	cfg config.DecompressConfig
}

// New creates a new Decompressor with the given decompress config.
func New(cfg config.DecompressConfig) *Decompressor {
	return &Decompressor{cfg: cfg}
}

// Extract decompresses the given file to the destination directory.
func (d *Decompressor) Extract(ctx context.Context, srcPath, destDir string) error {
	if d.cfg.Skip.Bool() {
		return nil
	}

	// clean_install: remove existing files in dest before extraction
	if d.cfg.CleanInstall {
		if err := cleanInstall(destDir); err != nil {
			return fmt.Errorf("clean_install: %w", err)
		}
	}

	excludeFileType := d.cfg.ExcludeFileType
	skip := excludeSkipper(excludeFileType)

	// single_dir: extract to temp dir, then move contents up if single subdirectory
	if d.cfg.SingleDir.Bool() {
		return extractWithSingleDir(ctx, srcPath, d.cfg.SingleDir, skip, destDir)
	}

	// Dispatch to the appropriate format via auto-detection.
	return extractFile(ctx, srcPath, destDir, skip)
}

// extractWithSingleDir extracts to a temp dir, then if there's exactly one
// subdirectory at the top level, moves its contents into destDir.
func extractWithSingleDir(ctx context.Context, srcPath string, prefix config.BoolOrString, skip skipper, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "updater-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	s := skipper(skip)
	if prefix.String() != "" {
		s = mergeSkipper{s, prefixSkipper(prefix.StringVal)}
	}

	if err := extractFile(ctx, srcPath, tmpDir, s); err != nil {
		return err
	}

	// Detect single subdirectory.
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
		return moveDirContents(filepath.Join(tmpDir, subDirs[0]), destDir)
	}

	return moveDirContents(tmpDir, destDir)
}

// extractFile extracts (or copies) srcPath into destDir, auto-detecting the
// archive format. Non-archive files are copied verbatim into destDir.
func extractFile(ctx context.Context, srcPath, destDir string, skip skipper) error {

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer f.Close()

	// Reset to start for format identification.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek %s: %w", srcPath, err)
	}

	format, _, err := archives.Identify(ctx, filepath.Base(srcPath), f)
	if err != nil {
		if err == archives.NoMatch {
			// Non-archive file (.exe, .apk, .dmg, ...) — copy it as-is.
			return copyFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)))
		}
		return fmt.Errorf("identify %s: %w", srcPath, err)
	}

	ex, ok := format.(archives.Extractor)
	if !ok {
		return copyFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)))
	}

	return ex.Extract(ctx, f, makeHandler(destDir, skip))
}

// makeHandler returns an archives.FileHandler that writes each archive entry
// into destDir, honoring the skip filter and guarding against path traversal.
func makeHandler(destDir string, skip skipper) archives.FileHandler {
	return func(ctx context.Context, fi archives.FileInfo) error {
		name := fi.NameInArchive
		if skip != nil && skip.shouldSkipFile(name) {
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
		return nil
	}
}
