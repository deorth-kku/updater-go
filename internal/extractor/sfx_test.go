package extractor

import (
	"os"
	"testing"
)

func TestFindSfxOffset(t *testing.T) {
	// Create a fake SFX file with magic number followed by data
	magic := sevenZipMagic
	payload := []byte("7z payload data here")
	fakeSfx := append(magic, payload...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ok := findSfxOffset(f)
	if !ok {
		t.Fatal("findSfxOffset() expected to find the 7z signature")
	}
	if offset != 0 {
		t.Errorf("findSfxOffset() = %d, want 0", offset)
	}
}

func TestFindSfxOffset_WithStub(t *testing.T) {
	// 7z signature embedded after an executable stub
	stub := []byte("MZ\x90\x00this is an executable stub of some length")
	magic := sevenZipMagic
	payload := []byte("0123456789ABCDEF")
	fakeSfx := append(append(stub, magic...), payload...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ok := findSfxOffset(f)
	if !ok {
		t.Fatal("findSfxOffset() expected to find the embedded 7z signature")
	}
	if offset != int64(len(stub)) {
		t.Errorf("findSfxOffset() = %d, want %d", offset, len(stub))
	}
}

func TestFindSfxOffset_NotASfx(t *testing.T) {
	// File without magic number
	tmpDir := t.TempDir()
	badPath := tmpDir + "/not_sfx.exe"
	if err := os.WriteFile(badPath, []byte("just some random data"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(badPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, ok := findSfxOffset(f); ok {
		t.Error("findSfxOffset() expected false for non-SFX file")
	}
}

func TestFindSfxOffset_FileNotFound(t *testing.T) {
	f, err := os.Open("/nonexistent/file.exe")
	if err == nil {
		f.Close()
		t.Fatal("expected error opening nonexistent file")
	}
}
