package extractor

import (
	"bytes"
	"context"
	"debug/pe"
	"io"
	"maps"
	"strings"
	_ "unsafe"

	"github.com/mholt/archives"
)

const (
	searchLimit = 2 * 1024 * 1024
	peakHeader  = 4096
)

// sevenZipMagic is the 7z format signature that marks the start of a 7z stream.
var (
	sevenZipMagic = []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}
	zipMagic      = []byte{0x50, 0x4b, 0x03, 0x04}
)

//go:linkname formats github.com/mholt/archives.formats
var (
	formats    map[string]archives.Format
	sfxMatcher = SFX(maps.Clone(formats))
)

func init() {
	archives.RegisterFormat(sfxMatcher)
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

func (s SFX) match(ctx context.Context, stream io.Reader) (int64, string) {
	_, off, name := s.matchcontinue(ctx, stream)
	return off, name
}

func (s SFX) matchcontinue(ctx context.Context, stream io.Reader) (io.Reader, int64, string) {
	rdat := asReaderAt(stream)
	off := findSfxOffsetPE(rdat)
	var name string
	if off == 0 {
		data, _ := readAt(rdat, 0, searchLimit)
		off, name = findSfxOffsetInData(data)
	} else {
		data, _ := readAt(rdat, off, peakHeader)
		install_off := installOffset(data)
		off += install_off
		if int64(len(data)) > install_off {
			name = s.findname(ctx, func() io.Reader {
				return bytes.NewReader(data[install_off:])
			})
		}
	}
	return continueAt(rdat, off), off, name
}

const install = ";!@InstallEnd@!"

func installOffset(data []byte) int64 {
	install_offset, found := findSfxOffsetInDataWithMagic(data, []byte(install))
	if !found {
		return 0
	}
	return findNextNonSpaceASCII(data, install_offset+int64(len(install)))
}

func isASCIIWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func findNextNonSpaceASCII(data []byte, offset int64) int64 {
	if offset < 0 {
		return offset
	}

	for i := offset; i < int64(len(data)); i++ {
		if !isASCIIWhitespace(data[i]) {
			return i
		}
	}

	return offset
}

func (s SFX) Match(ctx context.Context, filename string, stream io.Reader) (archives.MatchResult, error) {
	var mr archives.MatchResult

	if filename != "" && !strings.HasSuffix(strings.ToLower(filename), ".exe") {
		return mr, nil
	}

	off, name := s.match(ctx, stream)
	if name == "" || off == 0 {
		return mr, nil
	}

	mr.ByName = filename != ""
	mr.ByStream = true
	return mr, nil
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

func (sfx SFX) Extract(ctx context.Context, archive io.Reader, handleFile archives.FileHandler) error {
	cont, off, name := sfx.matchcontinue(ctx, archive)
	if off == 0 {
		return archives.NoMatch
	}
	arc, ok := sfx[name].(archives.Extractor)
	if !ok {
		return archives.NoMatch
	}
	return arc.Extract(ctx, cont, handleFile)
}

const (
	notSfx      = ""
	sevenZipSfx = "7z"
	zipSfx      = "zip"
)

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
