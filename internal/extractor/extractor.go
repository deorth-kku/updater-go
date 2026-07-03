// Package extractor handles file decompression and extraction.
package extractor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/ulikunitz/xz"
)

// Extractor handles decompression based on file type.
type Extractor struct {
	cfg config.DecompressConfig
}

// New creates a new Extractor with the given decompress config.
func New(cfg config.DecompressConfig) *Extractor {
	return &Extractor{cfg: cfg}
}

// Extract decompresses the given file to the destination directory.
func (e *Extractor) Extract(srcPath, destDir string) error {
	if e.cfg.Skip.Bool() {
		return nil
	}

	// clean_install: remove existing files in dest before extraction
	if e.cfg.CleanInstall {
		if err := cleanInstall(destDir); err != nil {
			return fmt.Errorf("clean_install: %w", err)
		}
	}

	ext := detectExt(srcPath)

	// single_dir: extract to temp dir, then move contents up if single subdirectory
	if e.cfg.SingleDir.Bool() {
		return e.extractWithSingleDir(srcPath, destDir)
	}

	switch ext {
	case ".zip":
		return e.extractZip(srcPath, destDir)
	case ".tar.gz", ".tgz":
		return e.extractTarGz(srcPath, destDir)
	case ".tar.xz", ".txz":
		return e.extractTarXz(srcPath, destDir)
	case ".7z":
		return e.extractSevenZ(srcPath, destDir)
	default:
		// For .exe, .apk, .dmg, etc. — just copy the file
		return copyFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)))
	}
}

// extractWithSingleDir extracts to a temp dir, then if there's exactly one
// subdirectory at the top level, moves its contents into destDir.
func (e *Extractor) extractWithSingleDir(srcPath, destDir string) error {
	tmpDir, err := os.MkdirTemp("", "updater-extract-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract into temp dir
	ext := detectExt(srcPath)
	switch ext {
	case ".zip":
		if err := e.extractZip(srcPath, tmpDir); err != nil {
			return err
		}
	case ".tar.gz", ".tgz":
		if err := e.extractTarGz(srcPath, tmpDir); err != nil {
			return err
		}
	case ".tar.xz", ".txz":
		if err := e.extractTarXz(srcPath, tmpDir); err != nil {
			return err
		}
	case ".7z":
		if err := e.extractSevenZ(srcPath, tmpDir); err != nil {
			return err
		}
	default:
		return copyFile(srcPath, filepath.Join(destDir, filepath.Base(srcPath)))
	}

	// Detect single subdirectory
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
		// Single subdirectory: move its contents into destDir
		src := filepath.Join(tmpDir, subDirs[0])
		return moveDirContents(src, destDir)
	}

	// Multiple or no subdirectories: move everything into destDir
	return moveDirContents(tmpDir, destDir)
}

// moveDirContents moves all contents from srcDir into destDir (without removing srcDir itself).
func moveDirContents(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			// If rename fails (cross-device), fall back to copy+remove
			if copyErr := copyDir(src, dst); copyErr != nil {
				return fmt.Errorf("move %s: %w (copy fallback: %v)", src, err, copyErr)
			}
			os.RemoveAll(src)
		}
	}
	return nil
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}

// cleanInstall removes all files and subdirectories inside destDir.
func cleanInstall(destDir string) error {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(destDir, 0o755)
		}
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(destDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

// detectExt detects the archive extension, handling compound extensions like .tar.gz.
func detectExt(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(lower, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(lower, ".tar.xz"):
		return ".tar.xz"
	case strings.HasSuffix(lower, ".txz"):
		return ".txz"
	default:
		return filepath.Ext(lower)
	}
}

func (e *Extractor) shouldSkipFile(name string) bool {
	for _, ext := range e.cfg.ExcludeFileType {
		if strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
			return true
		}
	}
	return false
}

func (e *Extractor) extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", src, err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Skip files matching exclude_file_type
		if e.shouldSkipFile(f.Name) {
			continue
		}

		target := filepath.Join(dest, f.Name)

		// Security: prevent zip slip
		if !safePath(target, dest) {
			return fmt.Errorf("invalid zip entry: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	return e.extractTar(tar.NewReader(gzr), dest)
}

func (e *Extractor) extractTarXz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("xz decompress %s: %w", src, err)
	}

	return e.extractTar(tar.NewReader(xzr), dest)
}

// extractTar is the common tar extraction logic shared by tar.gz and tar.xz.
func (e *Extractor) extractTar(tr *tar.Reader, dest string) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Security: prevent tar slip
		// filepath.Join cleans the path, so we check the raw name for ..
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}
		if !safePath(target, dest) {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

// safePath checks if target is safely within dest (prevents path traversal).
func safePath(target, dest string) bool {
	// Ensure dest ends with separator for proper prefix check
	dest = strings.TrimRight(dest, string(os.PathSeparator)) + string(os.PathSeparator)
	return strings.HasPrefix(target, dest)
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
