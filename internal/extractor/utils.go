package extractor

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

// extractTar is the common tar extraction logic shared by tar.gz and tar.xz.
func extractTar(tr *tar.Reader, dest string, skip skipper) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Skip files matching exclude_file_type
		if skip != nil && skip.shouldSkipFile(header.Name) {
			continue
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

type excludeSkipper []string

// shouldSkipFile checks if a filename should be excluded based on file type extensions.
func (excludeFileType excludeSkipper) shouldSkipFile(name string) bool {
	for _, ext := range excludeFileType {
		if strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
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
