package extractor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bodgit/sevenzip"
)

// sevenZExtractor extracts .7z archives.
type sevenZExtractor struct {
	src string
}

// Ensure sevenZExtractor implements Extractor.
var _ Extractor = (*sevenZExtractor)(nil)

func newSevenZExtractor(src string) *sevenZExtractor {
	return &sevenZExtractor{src: src}
}

func (s *sevenZExtractor) Extract(skip skipper, dest string) error {
	f, err := os.Open(s.src)
	if err != nil {
		return fmt.Errorf("open 7z %s: %w", s.src, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat 7z %s: %w", s.src, err)
	}

	r, err := sevenzip.NewReader(f, info.Size())
	if err != nil {
		return fmt.Errorf("open 7z %s: %w", s.src, err)
	}

	for _, f := range r.File {
		if skip != nil && skip.shouldSkipFile(f.Name) {
			continue
		}

		target := filepath.Join(dest, f.Name)

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
