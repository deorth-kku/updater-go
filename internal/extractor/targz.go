package extractor

import (
	"archive/tar"
	"compress/gzip"
	"os"
)

// tarGzExtractor extracts .tar.gz / .tgz archives.
type tarGzExtractor struct {
	src string
}

// Ensure tarGzExtractor implements Extractor.
var _ Extractor = (*tarGzExtractor)(nil)

func newTarGzExtractor(src string) *tarGzExtractor {
	return &tarGzExtractor{src: src}
}

func (t *tarGzExtractor) Extract(skip skipper, dest string) error {
	f, err := os.Open(t.src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	return extractTar(tar.NewReader(gzr), dest, skip)
}
