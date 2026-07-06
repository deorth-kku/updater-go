package extractor

import (
	"archive/tar"
	"fmt"
	"os"

	"github.com/ulikunitz/xz"
)

// tarXzExtractor extracts .tar.xz / .txz archives.
type tarXzExtractor struct {
	src string
}

// Ensure tarXzExtractor implements Extractor.
var _ Extractor = (*tarXzExtractor)(nil)

func newTarXzExtractor(src string) *tarXzExtractor {
	return &tarXzExtractor{src: src}
}

func (t *tarXzExtractor) Extract(skip skipper, dest string) error {
	f, err := os.Open(t.src)
	if err != nil {
		return err
	}
	defer f.Close()

	xzr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("xz decompress %s: %w", t.src, err)
	}

	return extractTar(tar.NewReader(xzr), dest, skip)
}
