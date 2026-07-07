package extractor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/path/to/file.zip", ".zip"},
		{"/path/to/file.tar.gz", ".tar.gz"},
		{"/path/to/file.TAR.GZ", ".tar.gz"},
		{"/path/to/file.tgz", ".tgz"},
		{"/path/to/file.tar.xz", ".tar.xz"},
		{"/path/to/file.txz", ".txz"},
		{"/path/to/file.7z", ".7z"},
		{"/path/to/file.exe", ".exe"},
		{"/path/to/file.APK", ".apk"},
		{"/path/to/file", ""},
		{"/path/to/file.tar.gz.bak", ".bak"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectExt(tt.path)
			if got != tt.want {
				t.Errorf("detectExt(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	tests := []struct {
		target string
		dest   string
		want   bool
	}{
		{"/tmp/dest/hello.txt", "/tmp/dest", true},
		{"/tmp/dest/sub/hello.txt", "/tmp/dest", true},
		{"/tmp/other/hello.txt", "/tmp/dest", false},
		{"/tmp/dest../../etc/evil", "/tmp/dest", false},
		{"/tmp/dest", "/tmp/dest", false}, // exact match is not safe (no trailing separator)
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := safePath(tt.target, tt.dest)
			if got != tt.want {
				t.Errorf("safePath(%q, %q) = %v, want %v", tt.target, tt.dest, got, tt.want)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	src := filepath.Join(srcDir, "src.txt")
	dst := filepath.Join(dstDir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(content) != "hello" {
		t.Errorf("copyFile() content = %q, want %q", content, "hello")
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/file.txt", "/tmp/dst.txt")
	if err == nil {
		t.Error("copyFile() expected error for nonexistent source")
	}
}

func TestCleanInstall(t *testing.T) {
	destDir := t.TempDir()

	// Create some files in destDir
	os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(destDir, "b.txt"), []byte("b"), 0o644)
	os.MkdirAll(filepath.Join(destDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(destDir, "sub", "c.txt"), []byte("c"), 0o644)

	if err := cleanInstall(destDir); err != nil {
		t.Fatalf("cleanInstall() error = %v", err)
	}

	entries, _ := os.ReadDir(destDir)
	if len(entries) != 0 {
		t.Errorf("cleanInstall() left %d entries, want 0", len(entries))
	}
}

func TestCleanInstall_NonExistentDir(t *testing.T) {
	// Should not error for non-existent destDir (os.IsNotExist check)
	destDir := t.TempDir()
	os.RemoveAll(destDir)
	err := cleanInstall(destDir)
	if err != nil {
		t.Errorf("cleanInstall() on non-existent dir should not error, got %v", err)
	}
}

func TestMoveDirContents(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create files in srcDir
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0o644)

	if err := moveDirContents(srcDir, dstDir); err != nil {
		t.Fatalf("moveDirContents() error = %v", err)
	}

	// Verify files moved
	content, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(content) != "a" {
		t.Errorf("a.txt content = %q, want %q", content, "a")
	}

	content, err = os.ReadFile(filepath.Join(dstDir, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("read b.txt: %v", err)
	}
	if string(content) != "b" {
		t.Errorf("b.txt content = %q, want %q", content, "b")
	}

	// Verify srcDir still exists but is empty
	entries, _ := os.ReadDir(srcDir)
	if len(entries) != 0 {
		t.Errorf("srcDir should be empty after move, got %d entries", len(entries))
	}
}
