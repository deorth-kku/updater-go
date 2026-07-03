package extractor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bodgit/sevenzip"
)

// extractSevenZ extracts a 7z archive to destDir.
func (e *Extractor) extractSevenZ(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open 7z %s: %w", src, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat 7z %s: %w", src, err)
	}

	r, err := sevenzip.NewReader(f, info.Size())
	if err != nil {
		return fmt.Errorf("open 7z %s: %w", src, err)
	}

	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)

		// Security: prevent zip slip
		if !safePath(target, dest) {
			return fmt.Errorf("invalid 7z entry: %s", f.Name)
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
