// Package extractor handles file decompression and extraction.
package extractor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/deorth-kku/updater-go/internal/config"
)

type skipper interface {
	shouldSkipFile(string) bool
}

// Extractor defines the interface for archive extraction implementations.
type Extractor interface {
	Extract(filter skipper, destDir string) error
}

// Decompressor handles decompression by dispatching to the appropriate
// Extractor based on the file extension.
type Decompressor struct {
	cfg config.DecompressConfig
}

// New creates a new Decompressor with the given decompress config.
func New(cfg config.DecompressConfig) *Decompressor {
	return &Decompressor{cfg: cfg}
}

// Extract decompresses the given file to the destination directory.
func (d *Decompressor) Extract(srcPath, destDir string) error {
	if d.cfg.Skip.Bool() {
		return nil
	}

	// clean_install: remove existing files in dest before extraction
	if d.cfg.CleanInstall {
		if err := cleanInstall(destDir); err != nil {
			return fmt.Errorf("clean_install: %w", err)
		}
	}

	ext := detectExt(srcPath)
	excludeFileType := d.cfg.ExcludeFileType

	ex := newExtractor(ext, srcPath)
	if ex == nil {
		// For non-archive files (.exe, .apk, .dmg, etc.) — just copy the file.
		return copyFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)))
	}
	// single_dir: extract to temp dir, then move contents up if single subdirectory
	if d.cfg.SingleDir.Bool() {
		return extractWithSingleDir(ex, d.cfg.SingleDir, excludeFileType, destDir)
	}

	// Dispatch to the appropriate Extractor via registry lookup.
	return ex.Extract(excludeSkipper(excludeFileType), destDir)
}

// extractWithSingleDir extracts to a temp dir, then if there's exactly one
// subdirectory at the top level, moves its contents into destDir.
func extractWithSingleDir(ex Extractor, prefix config.BoolOrString, excludeFileType []string, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "updater-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	skip := skipper(excludeSkipper(excludeFileType))
	if prefix.String() != "" {
		skip = mergeSkipper{skip, prefixSkipper(prefix.StringVal)}
	}

	if err := ex.Extract(skip, tmpDir); err != nil {
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

// newExtractor returns the appropriate Extractor for the given extension and source path.
// Returns nil if the extension is not supported.
func newExtractor(ext, srcPath string) Extractor {
	switch ext {
	case ".zip":
		return newZipExtractor(srcPath)
	case ".tar.gz", ".tgz":
		return newTarGzExtractor(srcPath)
	case ".tar.xz", ".txz":
		return newTarXzExtractor(srcPath)
	case ".7z":
		return newSevenZExtractor(srcPath)
	default:
		return nil
	}
}
