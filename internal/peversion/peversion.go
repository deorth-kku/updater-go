// Package peversion reads the binary version embedded in a Windows PE
// executable's VS_FIXEDFILEINFO resource. This is the exact data Python's
// pefile exposes as VS_FIXEDFILEINFO.FileVersionMS/LS, which the old
// updater-rpc uses for version.use_exe_version.
//
// We do NOT use any third-party PE parser: stdlib debug/pe gives us the
// resource directory RVA and raw section bytes, and the VS_FIXEDFILEINFO
// layout is small and fixed, so we parse it directly (zero CGO, fully
// cross-compilable).
package peversion

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
)

// Version is a 4-component binary version (major.minor.build.revision),
// matching the layout of the PE VS_FIXEDFILEINFO.FileVersionMS/LS fields.
type Version [4]uint16

// String renders the version as "major.minor.build.revision".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

// FileVersion returns the 4-component FileVersion and ProductVersion from the
// PE version resource. It returns (zero, zero, nil) when the file has no
// version resource (e.g. a freshly compiled exe without a .rsrc version
// block), which mirrors Python pefile behaviour of treating the binary as
// "needs install".
func FileVersion(path string) (fileVer, prodVer Version, err error) {
	f, e := pe.Open(path)
	if e != nil {
		return Version{}, Version{}, fmt.Errorf("open pe %s: %w", path, e)
	}
	defer f.Close()

	fv, pv, e := fileVersionFromPE(f)
	if e != nil {
		return Version{}, Version{}, e
	}
	return fv, pv, nil
}

// FileVersionFromReader is like FileVersion but accepts an io.ReaderAt
// (e.g. an in-memory buffer), useful for tests and for callers that
// already have the file mapped.
func FileVersionFromReader(r io.ReaderAt) (fileVer, prodVer Version, err error) {
	f, e := pe.NewFile(r)
	if e != nil {
		return Version{}, Version{}, fmt.Errorf("parse pe: %w", e)
	}
	return fileVersionFromPE(f)
}

// fileVersionFromPE walks the resource directory (.rsrc) looking for
// RT_VERSION (type 16), then parses the VS_FIXEDFILEINFO structure that
// immediately follows the "VS_VERSION_INFO" key string.
func fileVersionFromPE(f *pe.File) (fileVer, prodVer Version, err error) {
	// Locate the resource data directory via the optional header.
	dd, ok := resourceDataDirectory(f)
	if !ok {
		return Version{}, Version{}, nil // no resource directory at all
	}
	if dd.VirtualAddress == 0 || dd.Size == 0 {
		return Version{}, Version{}, nil
	}

	section := findSectionByRVA(f, dd.VirtualAddress)
	if section == nil {
		return Version{}, Version{}, nil
	}
	data, err := section.Data()
	if err != nil {
		return Version{}, Version{}, fmt.Errorf("read .rsrc: %w", err)
	}
	// Offset of the resource directory within the raw section bytes.
	base := int(dd.VirtualAddress) - int(section.VirtualAddress)

	fv, pv, found, e := walkResourceDir(data, base, base, int(section.VirtualAddress), int(dd.Size), 0)
	if e != nil {
		return Version{}, Version{}, e
	}
	if !found {
		return Version{}, Version{}, nil
	}
	return fv, pv, nil
}

// resourceDataDirectory returns the IMAGE_DIRECTORY_ENTRY_RESOURCE entry.
func resourceDataDirectory(f *pe.File) (pe.DataDirectory, bool) {
	const imageDirectoryEntryResource = 2
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if int(imageDirectoryEntryResource) >= len(oh.DataDirectory) {
			return pe.DataDirectory{}, false
		}
		return oh.DataDirectory[imageDirectoryEntryResource], true
	case *pe.OptionalHeader64:
		if int(imageDirectoryEntryResource) >= len(oh.DataDirectory) {
			return pe.DataDirectory{}, false
		}
		return oh.DataDirectory[imageDirectoryEntryResource], true
	default:
		return pe.DataDirectory{}, false
	}
}

// findSectionByRVA returns the section whose raw data contains rva.
func findSectionByRVA(f *pe.File, rva uint32) *pe.Section {
	for _, s := range f.Sections {
		if s.VirtualAddress == 0 || s.Size == 0 {
			continue
		}
		if rva >= s.VirtualAddress && rva < s.VirtualAddress+s.VirtualSize {
			return s
		}
	}
	return nil
}

// walkResourceDir recursively descends the resource directory tree. When it
// reaches a leaf (data entry) whose raw bytes contain a VS_VERSION_INFO
// signature, it parses and returns the VS_FIXEDFILEINFO.
//
// dirBase   = raw offset of the current IMAGE_RESOURCE_DIRECTORY header.
// rootBase  = raw offset of the resource root; all offsets stored in entries
//
//	(both directory and leaf) are relative to this root.
//
// sectionVA = VirtualAddress of the .rsrc section; used to convert the leaf
//
//	IMAGE_RESOURCE_DATA_ENTRY.OffsetToData (an image-relative RVA) back
//	into a raw byte offset within the section slice.
//
// The bool reports whether a version resource was found.
func walkResourceDir(data []byte, dirBase, rootBase, sectionVA, size, depth int) (Version, Version, bool, error) {
	if depth > 8 || dirBase < 0 || dirBase+16 > len(data) {
		return Version{}, Version{}, false, nil
	}
	// IMAGE_RESOURCE_DIRECTORY header:
	//  Characteristics(4) TimeDateStamp(4) MajorVersion(2) MinorVersion(2)
	//  NumberOfNamedEntries(2) NumberOfIdEntries(2)
	named := binary.LittleEndian.Uint16(data[dirBase+12:])
	id := binary.LittleEndian.Uint16(data[dirBase+14:])
	entryCount := int(named) + int(id)
	entryBase := dirBase + 16

	for i := 0; i < entryCount; i++ {
		off := entryBase + i*8
		if off+8 > len(data) {
			break
		}
		offsetToData := binary.LittleEndian.Uint32(data[off+4:])

		dataIsDir := offsetToData&0x80000000 != 0
		if dataIsDir {
			// Descend into the subdirectory; its header lives at
			// rootBase + subdirOffset.
			fv, pv, found, err := walkResourceDir(data, rootBase+int(offsetToData&0x7FFFFFFF), rootBase, sectionVA, size, depth+1)
			if err != nil {
				return Version{}, Version{}, false, err
			}
			if found {
				return fv, pv, true, nil
			}
			continue
		}

		// Leaf: offsetToData (root-relative) points to an
		// IMAGE_RESOURCE_DATA_ENTRY structure whose first field,
		// OffsetToData, is an image-relative RVA — not a raw section offset.
		// Subtract sectionVA to get the actual byte offset within data[].
		dataEntryOff := rootBase + int(offsetToData&0x7FFFFFFF)
		if dataEntryOff+4 > len(data) {
			continue
		}
		dataRVA := int(binary.LittleEndian.Uint32(data[dataEntryOff:]))
		rawOff := dataRVA - sectionVA
		fv, pv, err := parseFixedFileInfo(data, rawOff)
		if err != nil {
			return Version{}, Version{}, false, err
		}
		if fv != (Version{}) {
			return fv, pv, true, nil
		}
	}
	return Version{}, Version{}, false, nil
}

// parseFixedFileInfo reads VS_VERSIONINFO -> VS_FIXEDFILEINFO.
//
// Layout:
//
//	VS_VERSIONINFO (root):
//	  wLength(2) wValueLength(2) wType(2)
//	  szKey = "VS_VERSION_INFO" (UTF-16, 15 chars + NUL = 32 bytes)
//	  (padding to 4-byte boundary)
//	  VS_FIXEDFILEINFO:
//	    dwSignature(4) = 0xFEEF04BD
//	    dwStrucVersion(4)
//	    dwFileVersionMS(4)  -> [0]=Major, [1]=Minor
//	    dwFileVersionLS(4)  -> [2]=Build, [3]=Revision
//	    dwProductVersionMS(4) -> [0]=Major, [1]=Minor
//	    dwProductVersionLS(4) -> [2]=Build, [3]=Revision
func parseFixedFileInfo(data []byte, dataEntryOffset int) (Version, Version, error) {
	if dataEntryOffset < 0 || dataEntryOffset+16 > len(data) {
		return Version{}, Version{}, nil
	}
	// Find the VS_VERSION_INFO key within a small window after the data entry
	// start. The data entry points at the beginning of the VS_VERSIONINFO
	// structure.
	root := dataEntryOffset
	// wValueLength (offset 2) tells how many bytes of fixed info follow the
	// key. We locate the key string "VS_VERSION_INFO".
	key := []byte("V\000S\000_\000V\000E\000R\000S\000I\000O\000N\000_\000I\000N\000F\000O\000\000\000")
	keyOff := indexOf(data, key, root, root+min(len(data), 256))
	if keyOff < 0 {
		return Version{}, Version{}, nil
	}
	// Fixed info starts right after the key (32 bytes), then round up to the
	// next 4-byte boundary.
	ffiStart := keyOff + len(key)
	ffiStart = (ffiStart + 3) &^ 3
	if ffiStart+52 > len(data) {
		return Version{}, Version{}, nil
	}
	// dwSignature must be 0xFEEF04BD.
	sig := binary.LittleEndian.Uint32(data[ffiStart:])
	if sig != 0xFEEF04BD {
		return Version{}, Version{}, nil
	}
	ms := binary.LittleEndian.Uint32(data[ffiStart+8:])
	ls := binary.LittleEndian.Uint32(data[ffiStart+12:])
	pms := binary.LittleEndian.Uint32(data[ffiStart+16:])
	pls := binary.LittleEndian.Uint32(data[ffiStart+20:])
	var fv Version
	fv[0] = uint16(ms >> 16)
	fv[1] = uint16(ms & 0xFFFF)
	fv[2] = uint16(ls >> 16)
	fv[3] = uint16(ls & 0xFFFF)
	var pv Version
	pv[0] = uint16(pms >> 16)
	pv[1] = uint16(pms & 0xFFFF)
	pv[2] = uint16(pls >> 16)
	pv[3] = uint16(pls & 0xFFFF)
	return fv, pv, nil
}

func indexOf(haystack, needle []byte, from, to int) int {
	if from < 0 {
		from = 0
	}
	if to > len(haystack) {
		to = len(haystack)
	}
	if to-from < len(needle) {
		return -1
	}
	for i := from; i <= to-len(needle); i++ {
		if bytesEqual(haystack[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
