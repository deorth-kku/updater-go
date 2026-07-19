package extractor

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// safePath checks if target is safely within dest (prevents path traversal).
func safePath(target, dest string) bool {
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

	return copyF(in, dst)
}

func copyF(f *os.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, f)
	return err
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

type excludeSkipper []string

// shouldSkipFile checks if a filename should be excluded based on file type extensions.
func (excludeFileType excludeSkipper) shouldSkipFile(name string) bool {
	for _, ext := range excludeFileType {
		if strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
			slog.Default().Debug("file skipped during extract",
				"step", "extractor.skip",
				"name", name,
				"exclude_ext", ext,
				"reason", "matched exclude file type",
				"result", "skip",
			)
			return true
		}
	}
	return false
}

type prefixSkipper string

func (p prefixSkipper) shouldSkipFile(name string) bool {
	return !strings.HasPrefix(name, string(p))
}

type mergeSkipper []skipper

func (p mergeSkipper) shouldSkipFile(name string) bool {
	for _, v := range p {
		if v.shouldSkipFile(name) {
			return true
		}
	}
	return false
}
