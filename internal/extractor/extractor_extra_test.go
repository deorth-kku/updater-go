package extractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

// --- Decompressor Nop tests ---

func TestDecompressor_Nop(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt": "hello world\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		Skip:            config.BoolOrString{BoolVal: true, IsBool: true},
		ExcludeFileType: []string{},
		SingleDir:       config.BoolOrString{BoolVal: false, IsBool: true},
	}
	d := New(cfg)
	if err := d.Extract(archivePath, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// destDir should be empty since skip is true
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 0 {
		t.Errorf("expected 0 files extracted (skip=true), got %d", len(entries))
	}
}

func TestDecompressor_NonArchiveFile(t *testing.T) {
	// Create a fake .exe file
	srcPath := filepath.Join(t.TempDir(), "fake.exe")
	os.WriteFile(srcPath, []byte("fake exe content"), 0o644)

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		Skip:            config.BoolOrString{BoolVal: false, IsBool: true},
		ExcludeFileType: []string{},
		SingleDir:       config.BoolOrString{BoolVal: false, IsBool: true},
	}
	d := New(cfg)
	if err := d.Extract(srcPath, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// The file should be copied to destDir
	content, err := os.ReadFile(filepath.Join(destDir, "fake.exe"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != "fake exe content" {
		t.Errorf("content = %q, want %q", content, "fake exe content")
	}
}

func TestDecompressor_SingleDir_WithPrefix(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"app/bin/go":     "binary\n",
		"app/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		Skip:            config.BoolOrString{BoolVal: false, IsBool: true},
		ExcludeFileType: []string{},
		SingleDir:       config.BoolOrString{BoolVal: true, IsBool: false, StringVal: "app"},
	}
	d := New(cfg)
	if err := d.Extract(archivePath, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Files should be extracted directly to destDir (single dir prefix "app" removed)
	verifyExtracted(t, destDir, map[string]string{
		"bin/go":     "binary\n",
		"readme.txt": "readme\n",
	})
}

func TestDecompressor_SingleDir_NoSingleDir(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"app/bin/go":     "binary\n",
		"app/readme.txt": "readme\n",
		"other/file.txt": "other\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		Skip:            config.BoolOrString{BoolVal: false, IsBool: true},
		ExcludeFileType: []string{},
		SingleDir:       config.BoolOrString{BoolVal: true, IsBool: true},
	}
	d := New(cfg)
	if err := d.Extract(archivePath, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Since there are multiple subdirs, all contents should be moved up
	// The single_dir logic moves contents of the temp dir (which has app/ and other/) to destDir
	// So we should see the files directly in destDir
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (app/, other/), got %d", len(entries))
	}
}

// --- newExtractor tests ---

func TestNewExtractor_Zip(t *testing.T) {
	ex := newExtractor(".zip", "/tmp/test.zip")
	if ex == nil {
		t.Error("newExtractor(.zip) should return non-nil")
	}
}

func TestNewExtractor_TarGz(t *testing.T) {
	ex := newExtractor(".tar.gz", "/tmp/test.tar.gz")
	if ex == nil {
		t.Error("newExtractor(.tar.gz) should return non-nil")
	}
}

func TestNewExtractor_TarXz(t *testing.T) {
	ex := newExtractor(".tar.xz", "/tmp/test.tar.xz")
	if ex == nil {
		t.Error("newExtractor(.tar.xz) should return non-nil")
	}
}

func TestNewExtractor_Tgz(t *testing.T) {
	ex := newExtractor(".tgz", "/tmp/test.tgz")
	if ex == nil {
		t.Error("newExtractor(.tgz) should return non-nil")
	}
}

func TestNewExtractor_Txz(t *testing.T) {
	ex := newExtractor(".txz", "/tmp/test.txz")
	if ex == nil {
		t.Error("newExtractor(.txz) should return non-nil")
	}
}

func TestNewExtractor_SevenZ(t *testing.T) {
	ex := newExtractor(".7z", "/tmp/test.7z")
	if ex == nil {
		t.Error("newExtractor(.7z) should return non-nil")
	}
}

func TestNewExtractor_Exe(t *testing.T) {
	// .exe that is not an SFX should return nil (falls through to default)
	nonSfxPath := filepath.Join(t.TempDir(), "fake.exe")
	os.WriteFile(nonSfxPath, []byte("not an sfx"), 0o644)
	ex := newExtractor(".exe", nonSfxPath)
	if ex != nil {
		t.Error("newExtractor(.exe) for non-SFX file should return nil")
	}
}

func TestNewExtractor_Unsupported(t *testing.T) {
	ex := newExtractor(".rar", "/tmp/test.rar")
	if ex != nil {
		t.Error("newExtractor(.rar) should return nil for unsupported format")
	}
}

func TestNewExtractor_Empty(t *testing.T) {
	ex := newExtractor("", "/tmp/test")
	if ex != nil {
		t.Error("newExtractor(\"\") should return nil")
	}
}
