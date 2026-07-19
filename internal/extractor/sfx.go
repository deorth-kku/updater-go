package extractor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mholt/archives"
)

const (
	searchLimit = 2 * 1024 * 1024
)

// sevenZipMagic is the 7z format signature that marks the start of a 7z stream.
var sevenZipMagic = []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}

func init() {
	archives.RegisterFormat(sevenZipSFX{})
}

type sevenZipSFX archives.SevenZip

type sfxSeekReaderAt interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

func (sevenZipSFX) Extension() string {
	return ".exe"
}

func (sevenZipSFX) MediaType() string { return "application/x-msdownload" }

func (sevenZipSFX) Match(_ context.Context, filename string, stream io.Reader) (archives.MatchResult, error) {
	var mr archives.MatchResult

	if !strings.HasSuffix(strings.ToLower(filename), ".exe") {
		return mr, nil
	}

	data, err := readAtMostLocal(stream, searchLimit)
	if err != nil {
		return mr, err
	}

	offset, ok := findSfxOffsetInData(data)
	if !ok || offset == 0 {
		return mr, nil
	}

	mr.ByStream = true
	return mr, nil
}

func (sfx sevenZipSFX) Extract(ctx context.Context, archive io.Reader, handleFile archives.FileHandler) error {
	file, ok := archive.(sfxSeekReaderAt)
	if !ok {
		return fmt.Errorf("input type must support io.ReaderAt and io.Seeker for 7z SFX extraction")
	}

	size, err := streamSize(file)
	if err != nil {
		return fmt.Errorf("determine source archive size: %w", err)
	}

	offset, ok := findSfxOffsetReaderAt(file)
	if !ok || offset == 0 {
		return archives.NoMatch
	}

	payload := io.NewSectionReader(file, offset, size-offset)
	return archives.SevenZip(sfx).Extract(ctx, payload, handleFile)
}

// findSfxOffset scans the beginning of f for the 7z signature. Self-extracting
// (SFX) archives embed a 7z payload after an executable stub, so the signature
// is not at offset 0. Returns the offset of the signature and true if found.
func findSfxOffset(f *os.File) (int64, bool) {
	return findSfxOffsetReaderAt(f)
}

func findSfxOffsetReaderAt(r io.ReaderAt) (int64, bool) {
	data := make([]byte, searchLimit)
	n, err := r.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return 0, false
	}
	return findSfxOffsetInData(data[:n])
}

func findSfxOffsetInData(data []byte) (int64, bool) {
	for i := range len(data) - len(sevenZipMagic) + 1 {
		if bytes.Equal(sevenZipMagic, data[i:i+len(sevenZipMagic)]) {
			return int64(i), true
		}
	}
	return 0, false
}

func streamSize(stream io.Seeker) (int64, error) {
	current, err := stream.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	size, err := stream.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	if _, err := stream.Seek(current, io.SeekStart); err != nil {
		return 0, err
	}

	return size, nil
}

func readAtMostLocal(stream io.Reader, limit int) ([]byte, error) {
	if stream == nil || limit <= 0 {
		return []byte{}, nil
	}

	buf := make([]byte, limit)
	n, err := io.ReadFull(stream, buf)
	if err == nil || err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}

	return nil, err
}
