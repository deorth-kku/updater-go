package extractor

import (
	"bytes"
	"io"
	"os"
)

const (
	searchLimit = 2 * 1024 * 1024
)

// sevenZipMagic is the 7z format signature that marks the start of a 7z stream.
var sevenZipMagic = []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}

// findSfxOffset scans the beginning of f for the 7z signature. Self-extracting
// (SFX) archives embed a 7z payload after an executable stub, so the signature
// is not at offset 0. Returns the offset of the signature and true if found.
func findSfxOffset(f *os.File) (int64, bool) {
	data := make([]byte, searchLimit)
	n, err := f.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return 0, false
	}
	data = data[:n]

	for i := range len(data) - len(sevenZipMagic) {
		if bytes.Equal(sevenZipMagic, data[i:i+len(sevenZipMagic)]) {
			return int64(i), true
		}
	}
	return 0, false
}
