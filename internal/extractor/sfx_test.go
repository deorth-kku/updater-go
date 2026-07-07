package extractor

import (
	"bytes"
	"os"
	"testing"
)

func TestSfxExtracter_ReadAt(t *testing.T) {
	// Create a fake SFX file with magic number followed by data
	magic := []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}
	payload := []byte("7z payload data here")
	fakeSfx := append(magic, payload...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	ex, err := newSfxExtracter(sfxPath)
	if err != nil {
		t.Fatalf("newSfxExtracter() error = %v", err)
	}

	// ReadAt reads from the magic offset (position 0), so it reads from the start of the file
	buf := make([]byte, 8)
	n, err := ex.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt() error = %v", err)
	}
	if n != 8 {
		t.Errorf("ReadAt() n = %d, want 8", n)
	}
	if !bytes.Equal(buf, magic) {
		t.Errorf("ReadAt() = %q, want magic bytes", buf)
	}
}

func TestSfxExtracter_ReadAt_Offset(t *testing.T) {
	magic := []byte{0x37, 0x7a, 0xbc, 0xaf, 0x27, 0x1c, 0x00, 0x04}
	payload := []byte("0123456789ABCDEF")
	fakeSfx := append(magic, payload...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	ex, err := newSfxExtracter(sfxPath)
	if err != nil {
		t.Fatalf("newSfxExtracter() error = %v", err)
	}

	// Read at offset 8 should read from position 8 (after magic)
	buf := make([]byte, 4)
	n, err := ex.ReadAt(buf, 8)
	if err != nil {
		t.Fatalf("ReadAt() error = %v", err)
	}
	if n != 4 {
		t.Errorf("ReadAt() n = %d, want 4", n)
	}
	if string(buf) != "0123" {
		t.Errorf("ReadAt() = %q, want %q", buf, "0123")
	}
}

func TestSfxExtracter_NotASfx(t *testing.T) {
	// File without magic number
	tmpDir := t.TempDir()
	badPath := tmpDir + "/not_sfx.exe"
	if err := os.WriteFile(badPath, []byte("just some random data"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newSfxExtracter(badPath)
	if err == nil {
		t.Error("newSfxExtracter() expected error for non-SFX file")
	}
}

func TestSfxExtracter_FileNotFound(t *testing.T) {
	_, err := newSfxExtracter("/nonexistent/file.exe")
	if err == nil {
		t.Error("newSfxExtracter() expected error for nonexistent file")
	}
}
