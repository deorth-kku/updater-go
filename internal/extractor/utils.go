package extractor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dstPath)
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

// CachedReaderAt wraps an io.Reader into an io.ReaderAt.
type CachedReaderAt struct {
	r   io.Reader
	mu  sync.Mutex
	buf []byte
	err error // stores the final error returned by the underlying io.Reader (e.g. io.EOF)
}

// NewCachedReaderAt creates a new cached reader.
func NewCachedReaderAt(r io.Reader) *CachedReaderAt {
	return &CachedReaderAt{
		r:   r,
		buf: make([]byte, 0),
	}
}
func (c *CachedReaderAt) expandTo(end int64) {
	// If the requested offset extends beyond the current cached data and the underlying Reader has not finished,
	// continue reading to fill the cache.
	for int64(len(c.buf)) < end && c.err == nil {
		need := end - int64(len(c.buf))
		chunk := make([]byte, need)

		nr, rerr := c.r.Read(chunk)
		if nr > 0 {
			c.buf = append(c.buf, chunk[:nr]...)
		}
		if rerr != nil || nr == 0 {
			c.err = rerr
			break
		}
	}
}

// ReadAt implements the io.ReaderAt interface.
func (c *CachedReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("CachedReaderAt.ReadAt: negative offset")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	end := off + int64(len(p))
	c.expandTo(end)

	// At this point, the underlying data can no longer provide more bytes.
	if off >= int64(len(c.buf)) {
		if c.err != nil {
			return 0, c.err
		}
		return 0, io.EOF
	}

	// Copy data from the cache slice into the target slice p.
	n = copy(p, c.buf[off:])
	if n < len(p) {
		if c.err != nil {
			err = c.err
		} else {
			err = io.EOF
		}
	}

	return n, err
}
