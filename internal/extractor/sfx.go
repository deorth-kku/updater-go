package extractor

import (
	"bytes"
	"context"
	"debug/pe"
	"errors"
	"io"
	"maps"
	"os"
	"strings"
	_ "unsafe"

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

//go:linkname formats github.com/mholt/archives.formats
var formats map[string]archives.Format

func init() {
	archives.RegisterFormat(SFX(maps.Clone(formats)))
}

type sfxSeekReaderAt interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

type SFX map[string]archives.Format

func (SFX) Extension() string {
	return ".exe"
}

func (SFX) MediaType() string { return "application/x-msdownload" }

func (s SFX) Match(ctx context.Context, filename string, stream io.Reader) (archives.MatchResult, error) {
	var mr archives.MatchResult

	if filename != "" && !strings.HasSuffix(strings.ToLower(filename), ".exe") {
		return mr, nil
	}

	rdat := NewCachedReaderAt(stream)
	off := findSfxOffsetPE(rdat)
	var name string
	if off == 0 {
		rdat.expandTo(searchLimit)
		off, name = findSfxOffsetInData(rdat.buf)
	} else {
		rdat.expandTo(off + 4096)
		if len(rdat.buf) > int(off) {
			name = s.findname(ctx, func() io.Reader {
				return bytes.NewReader(rdat.buf[off:])
			})
		}
	}

	if name == "" || off == 0 {
		return mr, nil
	}

	mr.ByName = filename != ""
	mr.ByStream = true
	return mr, nil
}

func resetAndSection(stream sfxSeekReaderAt, offset int64) io.Reader {
	size, err := stream.Seek(0, io.SeekEnd)
	if err != nil {
		return nil
	}
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	if offset >= size {
		return nil
	}
	return io.NewSectionReader(stream, offset, size-offset)
}

func (s SFX) findname(ctx context.Context, streamf func() io.Reader) string {
	for name, ty := range s {
		stream := streamf()
		if stream == nil {
			break
		}
		mr, err := ty.Match(ctx, "", stream)
		if err != nil {
			continue
		}
		if mr.ByStream {
			return name
		}
	}
	return ""
}

func (s SFX) findSfxOffsetReaderAt(ctx context.Context, r sfxSeekReaderAt) (int64, string) {
	off := findSfxOffsetPE(r)
	if off == 0 {
		// bytes search fallback
		return findSfxOffsetReaderAt(r)
	}
	return off, s.findname(ctx, func() io.Reader {
		return resetAndSection(r, off)
	})
}

var errReaderAt = errors.New("input type must support io.ReadAt and io.Seeker for SFX extraction")

func (sfx SFX) Extract(ctx context.Context, archive io.Reader, handleFile archives.FileHandler) error {
	file, ok := archive.(sfxSeekReaderAt)
	if !ok {
		return errReaderAt
	}

	off, name := sfx.findSfxOffsetReaderAt(ctx, file)
	if off == 0 {
		return archives.NoMatch
	}
	arc, ok := sfx[name].(archives.Extractor)
	if !ok {
		return archives.NoMatch
	}
	sec := resetAndSection(file, off)
	if sec == nil {
		return errReaderAt
	}
	return arc.Extract(ctx, sec, handleFile)
}

// findSfxOffset locates the embedded archive payload in an SFX file. It
// prefers debug/pe parsing of the PE section table for an accurate offset,
// falling back to brute-force signature scanning when the file is not a valid
// PE or the section table doesn't point to a known archive signature.
func findSfxOffset(f *os.File) (int64, string) {
	return findSfxOffsetReaderAt(f)
}

const (
	notSfx      = ""
	sevenZipSfx = "7z"
	zipSfx      = "zip"
)

// findSfxOffsetReaderAt locates the embedded archive payload in an SFX file.
// It first tries debug/pe to parse the PE header for an accurate offset, then
// falls back to brute-force signature scanning for non-PE or malformed files.
func findSfxOffsetReaderAt(r io.ReaderAt) (int64, string) {
	data := make([]byte, searchLimit)
	n, err := r.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return 0, notSfx
	}
	return findSfxOffsetInData(data[:n])
}

// findSfxOffsetPE uses debug/pe to parse the PE header and locate the embedded
// archive payload. SFX archives append their compressed data after the last PE
// section's raw data, so the payload offset equals the end of the last section.
// Returns 0 if the file is not a valid PE
func findSfxOffsetPE(r io.ReaderAt) int64 {
	peFile, err := pe.NewFile(r)
	if err != nil {
		return 0
	}
	defer peFile.Close()

	// Find the end of the last PE section's raw data.
	var maxEnd int64
	for _, sec := range peFile.Sections {
		end := int64(sec.Offset) + int64(sec.Size)
		if end > maxEnd {
			maxEnd = end
		}
	}

	return maxEnd
}

func findSfxOffsetInData(data []byte) (int64, string) {
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
	for i := range len(data) - len(magic) {
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
