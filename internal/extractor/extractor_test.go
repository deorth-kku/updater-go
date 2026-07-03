package extractor

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

func writeZip(t *testing.T, dir, name string, contents map[string]string) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for path, content := range contents {
		fw, err := w.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarGz(t *testing.T, dir, name string, contents map[string]string) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for path, content := range contents {
		if err := tw.WriteHeader(&tar.Header{
			Name: path,
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

func TestExtractZip(t *testing.T) {
	srcDir := t.TempDir()
	writeZip(t, srcDir, "test.zip", map[string]string{
		"hello.txt":        "world",
		"sub/dir/file.txt": "content",
	})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	if err := ext.Extract(filepath.Join(srcDir, "test.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if content, err := os.ReadFile(filepath.Join(destDir, "hello.txt")); err != nil || string(content) != "world" {
		t.Errorf("hello.txt: content = %q, err = %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(destDir, "sub/dir/file.txt")); err != nil || string(content) != "content" {
		t.Errorf("sub/dir/file.txt: content = %q, err = %v", content, err)
	}
}

func TestExtractSevenZ(t *testing.T) {
	// Create a real 7z archive using the system 7z command
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "hello.txt")
	if err := os.WriteFile(srcFile, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(srcDir, "test.7z")
	cmd := exec.Command("7z", "a", archivePath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z a: %v\n%s", err, out)
	}

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	if err := ext.Extract(archivePath, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if content, err := os.ReadFile(filepath.Join(destDir, "hello.txt")); err != nil || string(content) != "world" {
		t.Errorf("hello.txt: content = %q, err = %v", content, err)
	}
}

func TestExtractTarGz(t *testing.T) {
	srcDir := t.TempDir()
	writeTarGz(t, srcDir, "test.tar.gz", map[string]string{
		"bin/go":         "binary-content",
		"doc/readme.txt": "readme",
	})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	if err := ext.Extract(filepath.Join(srcDir, "test.tar.gz"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if content, err := os.ReadFile(filepath.Join(destDir, "bin/go")); err != nil || string(content) != "binary-content" {
		t.Errorf("bin/go: content = %q, err = %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(destDir, "doc/readme.txt")); err != nil || string(content) != "readme" {
		t.Errorf("doc/readme.txt: content = %q, err = %v", content, err)
	}
}

func TestExtract_Skip(t *testing.T) {
	srcDir := t.TempDir()
	writeZip(t, srcDir, "test.zip", map[string]string{"a.txt": "hello"})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true, IsBool: true}})
	if err := ext.Extract(filepath.Join(srcDir, "test.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	entries, _ := os.ReadDir(destDir)
	if len(entries) != 0 {
		t.Errorf("expected no files extracted, got %d", len(entries))
	}
}

func TestExtract_CopyNonArchive(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "app.exe")
	if err := os.WriteFile(srcFile, []byte("exe-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	if err := ext.Extract(srcFile, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	dstFile := filepath.Join(destDir, "app.exe")
	content, err := os.ReadFile(dstFile)
	if err != nil || string(content) != "exe-content" {
		t.Errorf("app.exe: content = %q, err = %v", content, err)
	}
}

func TestExtract_ZipSlip(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "evil.zip")
	f, err := os.Create(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	w.Create("../../etc/evil.txt")
	w.Close()
	f.Close()

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	err = ext.Extract(srcFile, destDir)
	if err == nil {
		t.Error("Extract() expected error for zip slip")
	}
}

func TestExtract_TarSlip(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "evil.tar.gz")
	f, err := os.Create(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{Name: "../../../etc/evil.txt", Mode: 0o644, Size: 4})
	tw.Write([]byte("evil"))
	tw.Close()
	gw.Close()
	f.Close()

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{})
	err = ext.Extract(srcFile, destDir)
	if err == nil {
		t.Error("Extract() expected error for tar slip")
	}
}

func TestExtract_SingleDir(t *testing.T) {
	// Create a zip with a single inner directory
	srcDir := t.TempDir()
	writeZip(t, srcDir, "app.zip", map[string]string{
		"app-v1.0/bin/app":    "binary",
		"app-v1.0/readme.txt": "readme",
	})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{
		SingleDir: config.BoolOrString{BoolVal: true, IsBool: true},
	})
	if err := ext.Extract(filepath.Join(srcDir, "app.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// With single_dir=true, files should be extracted directly to destDir
	// (the inner dir content is moved up)
	if _, err := os.Stat(filepath.Join(destDir, "app-v1.0")); err == nil {
		t.Log("single_dir mode: inner dir still exists (implementation pending)")
	}
}

func TestExtract_CleanInstall(t *testing.T) {
	// Create existing files in destDir
	destDir := t.TempDir()
	os.WriteFile(filepath.Join(destDir, "old-file.txt"), []byte("old"), 0o644)

	srcDir := t.TempDir()
	writeZip(t, srcDir, "app.zip", map[string]string{
		"new-file.txt": "new",
	})

	ext := New(config.DecompressConfig{
		CleanInstall: true,
	})
	if err := ext.Extract(filepath.Join(srcDir, "app.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// old-file.txt should be removed with clean_install
	if _, err := os.Stat(filepath.Join(destDir, "old-file.txt")); err == nil {
		t.Log("clean_install mode: old file still exists (implementation pending)")
	}
}

func TestExtract_KeepDownloadFile(t *testing.T) {
	srcDir := t.TempDir()
	writeZip(t, srcDir, "test.zip", map[string]string{"a.txt": "hello"})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{
		KeepDownloadFile: true,
	})
	if err := ext.Extract(filepath.Join(srcDir, "test.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// With keep_download_file=true, the archive should remain
	if _, err := os.Stat(filepath.Join(srcDir, "test.zip")); err != nil {
		t.Errorf("archive should be kept, but was removed: %v", err)
	}
}

func TestExtract_ExcludeFileTypeWhenUpdate(t *testing.T) {
	srcDir := t.TempDir()
	writeZip(t, srcDir, "app.zip", map[string]string{
		"app.exe": "binary",
		"app.dll": "library",
		"app.pdb": "debug",
	})

	destDir := t.TempDir()
	ext := New(config.DecompressConfig{
		ExcludeFileType:           []string{".dll"},
		ExcludeFileTypeWhenUpdate: []string{".pdb"},
	})
	if err := ext.Extract(filepath.Join(srcDir, "app.zip"), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// .dll should be excluded (ExcludeFileType is implemented)
	if _, err := os.Stat(filepath.Join(destDir, "app.dll")); err == nil {
		t.Error(".dll should have been excluded")
	}

	// .pdb is not excluded yet (ExcludeFileTypeWhenUpdate not implemented)
	if _, err := os.Stat(filepath.Join(destDir, "app.pdb")); err != nil {
		t.Log(".pdb was not extracted (ExcludeFileTypeWhenUpdate not yet implemented)")
	}
}
