package extractor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/mholt/archives"
	"github.com/ulikunitz/xz"
)

// defaultDecompressConfig returns the default DecompressConfig used by all tests.
func defaultDecompressConfig(exclude ...string) config.DecompressConfig {
	return config.DecompressConfig{
		Skip:            config.BoolOrString{BoolVal: false},
		ExcludeFileType: exclude,
		SingleDir:       config.BoolOrString{BoolVal: false},
		CleanInstall:    false,
	}
}

// verifyExtracted checks that all expected files exist with correct content.
func verifyExtracted(t *testing.T, destDir string, expected map[string]string) {
	t.Helper()
	for rel, want := range expected {
		full := filepath.Join(destDir, rel)
		content, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("%s: open: %v", rel, err)
			continue
		}
		if string(content) != want {
			t.Errorf("%s: content = %q, want %q", rel, content, want)
		}
	}
}

// --- Helper functions to generate archives in-code ---

// writeZipGo creates a zip archive using Go's archive/zip.
func writeZipGo(t *testing.T, path string, contents map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range contents {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
}

// writeTarGzGo creates a tar.gz archive using Go's archive/tar and compress/gzip.
func writeTarGzGo(t *testing.T, path string, contents map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for name, content := range contents {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
}

// writeTarXzGo creates a tar.xz archive using Go's archive/tar and ulikunitz/xz.
func writeTarXzGo(t *testing.T, path string, contents map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	xzw, err := xz.NewWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(xzw)
	for name, content := range contents {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	xzw.Close()
}

// writeSevenZGo creates a 7z archive using the system 7z command.
func writeSevenZGo(t *testing.T, path string, contents map[string]string) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "write7z-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	for name, content := range contents {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(srcDir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Use 7z to archive the entire directory structure, excluding the archive file itself
	cmd := exec.Command("7z", "a", "-x!"+filepath.Base(path), path, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z a: %v\n%s", err, out)
	}
}

// --- Phase 2: Normal extraction tests for all 4 extractors ---

func TestZipExtractor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt":        "hello world\n",
		"sub/dir/file.txt": "content\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt":        "hello world\n",
		"sub/dir/file.txt": "content\n",
	})
}

func TestZipExtractor_SkipFilter(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt":        "hello world\n",
		"sub/dir/file.txt": "content\n",
		"bin/go":           "binary\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig(".txt")
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	entries, _ := os.ReadDir(destDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file extracted (bin/go), got %d", len(entries))
	} else if entries[0].Name() != "bin" {
		t.Errorf("expected bin directory, got %s", entries[0].Name())
	}
}

func TestTarGzExtractor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	writeTarGzGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})
}

func TestTarGzExtractor_SkipFilter(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	writeTarGzGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig(".txt")
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	entries, _ := os.ReadDir(destDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file extracted (bin/go), got %d", len(entries))
	} else if entries[0].Name() != "bin" {
		t.Errorf("expected bin directory, got %s", entries[0].Name())
	}
}

func TestTarXzExtractor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.xz")
	writeTarXzGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})
}

func TestTarXzExtractor_SkipFilter(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.tar.xz")
	writeTarXzGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig(".txt")
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	entries, _ := os.ReadDir(destDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file extracted (bin/go), got %d", len(entries))
	} else if entries[0].Name() != "bin" {
		t.Errorf("expected bin directory, got %s", entries[0].Name())
	}
}

func TestSevenZExtractor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.7z")
	writeSevenZGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})
}

func TestSevenZExtractor_SkipFilter(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.7z")
	writeSevenZGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig(".txt")
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Note: directories are created even if files inside are skipped
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 directories (bin, doc), got %d", len(entries))
		return
	}

	// Verify bin/go exists and doc/readme.txt does not
	if _, err := os.ReadFile(filepath.Join(destDir, "bin/go")); err != nil {
		t.Errorf("bin/go should exist: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(destDir, "doc/readme.txt")); err == nil {
		t.Error("doc/readme.txt should have been skipped")
	}
}

// --- SFX extractor tests ---

// writeSfxGo creates an SFX executable using the system 7z command.
func writeSfxGo(t *testing.T, path string, contents map[string]string) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "writesfx-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	for name, content := range contents {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(srcDir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 7z -sfx creates the archive in the current working directory (srcDir).
	// We use a fixed output name and then rename it to the desired path.
	cmd := exec.Command("7z", "a", "-sfx", "SFX_EXE", ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z a -sfx: %v\n%s", err, out)
	}

	// Move the created SFX file to the desired path
	srcSfx := filepath.Join(srcDir, "SFX_EXE")
	if err := os.Rename(srcSfx, path); err != nil {
		t.Fatalf("rename sfx: %v", err)
	}
}

func TestSfxExtractor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.exe")
	writeSfxGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary content\n",
		"doc/readme.txt": "readme\n",
	})
}

func TestSfxExtractor_NotASfx(t *testing.T) {
	// Create a short text file with .exe extension — should not be detected as SFX
	exePath := filepath.Join(t.TempDir(), "fake.exe")
	os.WriteFile(exePath, []byte("this is just a short text file"), 0o644)

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), exePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "fake.exe")); err != nil {
		t.Errorf("non-SFX .exe should be copied as-is: %v", err)
	}
}

// --- Phase 3: Path traversal / evil file tests for all 4 extractors ---

func TestZipExtractor_EvilPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "evil.zip")
	writeZipGo(t, archivePath, map[string]string{
		"../../etc/evil.txt": "evil content",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	err = d.Extract(t.Context(), destDir)
	if err == nil {
		t.Error("Extract() expected error for zip slip")
	}
}

func TestTarGzExtractor_EvilPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "evil.tar.gz")
	writeTarGzGo(t, archivePath, map[string]string{
		"../../../etc/evil.txt": "evil content",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	err = d.Extract(t.Context(), destDir)
	if err == nil {
		t.Error("Extract() expected error for tar slip")
	}
}

func TestTarXzExtractor_EvilPath(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "evil.tar.xz")
	writeTarXzGo(t, archivePath, map[string]string{
		"../../../etc/evil.txt": "evil content",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	err = d.Extract(t.Context(), destDir)
	if err == nil {
		t.Error("Extract() expected error for tar slip")
	}
}

// Note: 7z evil path test skipped - bodgit/sevenzip library doesn't support creating archives with path traversal entries

// --- Phase 4: Decompressor public API tests ---

func TestDecompressor_Extract(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt": "hello world\n",
		"bin/go":    "binary\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"hello.txt": "hello world\n",
		"bin/go":    "binary\n",
	})
}

func TestDecompressor_ExcludeFileType(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt":      "hello world\n",
		"bin/go":         "binary\n",
		"doc/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig(".txt")
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	entries, _ := os.ReadDir(destDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file extracted (bin/go), got %d", len(entries))
	} else if entries[0].Name() != "bin" {
		t.Errorf("expected bin directory, got %s", entries[0].Name())
	}
}

func TestDecompressor_SingleDir_True(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"app/bin/go":     "binary\n",
		"app/readme.txt": "readme\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		SingleDir: config.BoolOrString{BoolVal: true},
	}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// SingleDir should move contents of "app" up to destDir
	verifyExtracted(t, destDir, map[string]string{
		"bin/go":     "binary\n",
		"readme.txt": "readme\n",
	})
}

func TestDecompressor_SingleDir_String(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"prefix/app/bin/go":     "binary\n",
		"prefix/app/readme.txt": "readme\n",
		"other/file.txt":        "other\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		SingleDir: config.BoolOrString{IsString: true, StringVal: "prefix/"},
	}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// SingleDir with prefix should:
	// 1. Filter files to only those starting with "prefix/"
	// 2. Detect single subdirectory and move contents up
	verifyExtracted(t, destDir, map[string]string{
		"app/bin/go":     "binary\n",
		"app/readme.txt": "readme\n",
	})
}

func TestDecompressor_Skip_True(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt": "hello world\n",
	})

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	cfg.Skip = config.BoolOrString{BoolVal: true}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Skip=true should not extract anything
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 0 {
		t.Errorf("expected no files extracted, got %d", len(entries))
	}
}

func TestDecompressor_CleanInstall(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt": "hello world\n",
	})

	// Pre-populate destDir with old files
	destDir := t.TempDir()
	os.WriteFile(filepath.Join(destDir, "old.txt"), []byte("old content"), 0o644)

	cfg := config.DecompressConfig{
		SingleDir:    config.BoolOrString{BoolVal: true},
		CleanInstall: true,
	}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// CleanInstall should remove old files and extract new ones
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file extracted, got %d", len(entries))
	} else if entries[0].Name() != "hello.txt" {
		t.Errorf("expected hello.txt, got %s", entries[0].Name())
	}
}

func TestDecompressor_NonArchive(t *testing.T) {
	// Create a non-archive file (e.g., .exe)
	srcFile := filepath.Join(t.TempDir(), "program.exe")
	os.WriteFile(srcFile, []byte("executable content"), 0o644)

	destDir := t.TempDir()
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), srcFile, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Non-archive files should be copied
	verifyExtracted(t, destDir, map[string]string{
		"program.exe": "executable content",
	})
}

// --- Decompressor Nop tests ---

func TestDecompressor_Nop(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "test.zip")
	writeZipGo(t, archivePath, map[string]string{
		"hello.txt": "hello world\n",
	})

	destDir := t.TempDir()
	cfg := config.DecompressConfig{
		Skip: config.BoolOrString{BoolVal: true},
	}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
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
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), srcPath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
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
		SingleDir: config.BoolOrString{IsString: true, StringVal: "app"},
	}
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
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
	cfg := defaultDecompressConfig()
	d, err := New(t.Context(), archivePath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
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
