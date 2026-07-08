package extractor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/mholt/archives"
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
	if err := d.Extract(t.Context(), archivePath, destDir); err != nil {
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
	if err := d.Extract(t.Context(), srcPath, destDir); err != nil {
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
	if err := d.Extract(t.Context(), archivePath, destDir); err != nil {
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
	if err := d.Extract(t.Context(), archivePath, destDir); err != nil {
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

// --- Identify tests ---

func TestIdentify_Zip(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(archivePath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Error("Identify(.zip) should return an archives.Extractor")
	}
}

func TestIdentify_TarGz(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	writeTarGzGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(archivePath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Error("Identify(.tar.gz) should return an archives.Extractor")
	}
}

func TestIdentify_TarXz(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.xz")
	writeTarXzGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(archivePath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Error("Identify(.tar.xz) should return an archives.Extractor")
	}
}

func TestIdentify_SevenZ(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.7z")
	writeSevenZGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(archivePath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Error("Identify(.7z) should return an archives.Extractor")
	}
}

func TestIdentify_NonArchive(t *testing.T) {
	// A plain .exe with no embedded archive should not match any format.
	nonSfxPath := filepath.Join(t.TempDir(), "fake.exe")
	os.WriteFile(nonSfxPath, []byte("not an sfx"), 0o644)

	f, err := os.Open(nonSfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, _, err = archives.Identify(t.Context(), filepath.Base(nonSfxPath), f)
	if err != archives.NoMatch {
		t.Errorf("Identify() for non-SFX .exe should return NoMatch, got %v", err)
	}
}
