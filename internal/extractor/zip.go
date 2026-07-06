package extractor

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// zipExtractor extracts .zip archives.
type zipExtractor struct {
	src string
}

// Ensure zipExtractor implements Extractor.
var _ Extractor = (*zipExtractor)(nil)

func newZipExtractor(src string) *zipExtractor {
	return &zipExtractor{src: src}
}

func (z *zipExtractor) Extract(excludeFileType []string, dest string) error {
	r, err := zip.OpenReader(z.src)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", z.src, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if shouldSkipFile(f.Name, excludeFileType) {
			continue
		}

		target := filepath.Join(dest, f.Name)

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
