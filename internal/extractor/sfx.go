package extractor

import (
	"bytes"
	"fmt"
	"os"
	"runtime"

	"github.com/deorth-kku/go-common/cleanup"
	cerrors "github.com/deorth-kku/go-common/errors"
)

const (
	errNotASfx  = cerrors.String("not a sfx file")
	searchLimit = 2 * 1024 * 1024
)

type sfxExtracter struct {
	f      *os.File
	offset int64
}

var magic_number = []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}

func newSfxExtracter(file string) (*sfxExtracter, error) {
	sfx := new(sfxExtracter)
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", file, err)
	}
	sfx.f = f
	runtime.AddCleanup(sfx, cleanup.Closer, f)
	data := make([]byte, searchLimit)
	n, err := f.Read(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", file, err)
	}
	data = data[:n]
	sfx.offset = -1
	for i := range len(data) - 8 {
		if bytes.Equal(magic_number, data[i:i+8]) {
			sfx.offset = int64(i)
			return sfx, nil
		}
	}
	return nil, errNotASfx
}

func (sfx *sfxExtracter) ReadAt(data []byte, at int64) (int, error) {
	return sfx.f.ReadAt(data, at+sfx.offset)
}

func (sfx *sfxExtracter) Extract(skip skipper, dest string) error {
	stat, err := sfx.f.Stat()
	if err != nil {
		return fmt.Errorf("stat file %s: %w", sfx.f.Name(), err)
	}
	return extract7zraw(sfx, stat.Size()-sfx.offset, skip, dest)
}
