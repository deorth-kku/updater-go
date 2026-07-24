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
var (
	sevenZipMagic = []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}
	zipMagic      = []byte{0x50, 0x4b, 0x03, 0x04}
)

func init() {
	archives.RegisterFormat(sevenZipSFX{})
}

type sevenZipSFX struct{}

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

	offset, ty := findSfxOffsetInData(data)
	if ty == notSfx || offset == 0 {
		return mr, nil
	}

	mr.ByStream = true
	mr.ByName = true
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

	offset, ty := findSfxOffsetReaderAt(file)
	if ty == notSfx || offset == 0 {
		return archives.NoMatch
	}

	payload := io.NewSectionReader(file, offset, size-offset)
	switch ty {
	case sevenZipSfx:
		return archives.SevenZip{}.Extract(ctx, payload, handleFile)
	case zipSfx:
		return archives.Zip{}.Extract(ctx, payload, handleFile)
	default:
		return fmt.Errorf("not valid sfxType %d", ty)
	}
}

// findSfxOffset scans the beginning of f for the 7z signature. Self-extracting
// (SFX) archives embed a 7z payload after an executable stub, so the signature
// is not at offset 0. Returns the offset of the signature and true if found.
func findSfxOffset(f *os.File) (int64, sfxType) {
	return findSfxOffsetReaderAt(f)
}

type sfxType int

const (
	notSfx sfxType = iota
	sevenZipSfx
	zipSfx
)

func findSfxOffsetReaderAt(r io.ReaderAt) (int64, sfxType) {
	data := make([]byte, searchLimit)
	n, err := r.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return 0, notSfx
	}
	return findSfxOffsetInData(data[:n])
}

func findSfxOffsetInData(data []byte) (int64, sfxType) {
	offset, ok := findSfxOffsetInDataWithMagic(data, sevenZipMagic)
	if ok {
		return offset, sevenZipSfx
	}
	offset, ok = findSfxOffsetInDataWithZipVersion(data)
	if ok {
		return offset, zipSfx
	}
	return 0, notSfx
}

func findSfxOffsetInDataWithMagic(data []byte, magic []byte) (int64, bool) {
	for i := range len(data) - len(magic) + 1 {
		if bytes.Equal(magic, data[i:i+len(magic)]) {
			return int64(i), true
		}
	}
	return 0, false
}

func findSfxOffsetInDataWithZipVersion(data []byte) (int64, bool) {
	magic := zipMagic
	for i := range len(data) - len(magic) + 2 {
		if !bytes.Equal(magic, data[i:i+len(magic)]) {
			continue
		}
		if data[i+4] == 0 { // zip version, cannot be zero
			continue
		}
		return int64(i), true
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
