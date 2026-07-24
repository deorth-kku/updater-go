package extractor

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/mholt/archives"
)

func TestFindSfxOffset(t *testing.T) {
	fakeSfx := append(append([]byte(nil), sevenZipMagic...), []byte("7z payload data here")...)

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

	offset, ty := findSfxOffset(f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find the 7z signature")
	}
	if offset != 0 {
		t.Errorf("findSfxOffset() = %d, want 0", offset)
	}
}

func TestFindSfxOffset_ShortHeaderAtEnd(t *testing.T) {
	fakeSfx := append([]byte("stub"), sevenZipMagic...)

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

	offset, ty := findSfxOffset(f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find trailing 7z signature")
	}
	if offset != 4 {
		t.Errorf("findSfxOffset() = %d, want 4", offset)
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

	offset, ty := findSfxOffset(f)
	if ty != sevenZipSfx {
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

	_, ty := findSfxOffset(f)
	if ty != notSfx {
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

func TestIdentify_Sfx7z(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "payload.7z")
	writeSevenZGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	sfxPath := filepath.Join(tmpDir, "payload.exe")
	stub := []byte("MZ\x90\x00stub")
	if err := os.WriteFile(sfxPath, append(stub, payload...), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(sfxPath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Fatal("Identify(.exe SFX) should return an archives.Extractor")
	}
}

func TestExtractFile_Sfx7z(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "payload.7z")
	contents := map[string]string{"hello.txt": "hello\n", "sub/dir/file.txt": "content\n"}
	writeSevenZGo(t, archivePath, contents)

	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	sfxPath := filepath.Join(tmpDir, "payload.exe")
	stub := []byte("MZ\x90\x00stub")
	if err := os.WriteFile(sfxPath, append(stub, payload...), 0o644); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	cfg := config.DecompressConfig{}
	d, err := New(t.Context(), sfxPath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	verifyExtracted(t, destDir, contents)
}

const testZipSfx = "/tmp/ReShade_Setup_6.7.3_Addon.exe"

func checkFile(t *testing.T) {
	t.Helper()
	_, err := os.Stat(testZipSfx)
	if errors.Is(err, os.ErrNotExist) {
		t.SkipNow()
	}
}

func TestFindSfxOffset_Zip(t *testing.T) {
	// curl https://reshade.me/downloads/ReShade_Setup_6.7.3_Addon.exe >/tmp/ReShade_Setup_6.7.3_Addon.exe
	checkFile(t)
	f, err := os.Open(testZipSfx)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := findSfxOffset(f)
	if ty != zipSfx {
		t.Fatal("findSfxOffset() expected to find trailing zip signature")
	}
	if offset != 154624 {
		t.Errorf("findSfxOffset() = %d, want 154624", offset)
	}
}

func TestExtractFile_SfxZip(t *testing.T) {
	checkFile(t)
	destDir := t.TempDir()
	cfg := config.DecompressConfig{}
	d, err := New(t.Context(), testZipSfx, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyJsonExtracted(t, destDir, []string{
		"ReShade32.json",
		"ReShade32_XR.json",
		"ReShade64.json",
		"ReShade64_XR.json",
	})
}

// verifyJsonExtracted checks that all expected JSON files exist in destDir and contain valid JSON.
func verifyJsonExtracted(t *testing.T, destDir string, jsonFiles []string) {
	t.Helper()
	for _, name := range jsonFiles {
		full := filepath.Join(destDir, name)
		content, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("%s: open: %v", name, err)
			continue
		}
		var v any
		if err := json.Unmarshal(content, &v); err != nil {
			t.Errorf("%s: invalid JSON: %v", name, err)
		}
	}
}
