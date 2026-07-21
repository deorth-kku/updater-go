package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		t.Errorf("srcDir not empty after move, got %d entries", len(entries))
	}
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a nested directory structure
	os.MkdirAll(filepath.Join(srcDir, "sub1", "sub2"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "sub1", "file1.txt"), []byte("file1"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "sub1", "sub2", "file2.txt"), []byte("file2"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0o644)

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify root file
	content, err := os.ReadFile(filepath.Join(dstDir, "root.txt"))
	if err != nil {
		t.Fatalf("read root.txt: %v", err)
	}
	if string(content) != "root" {
		t.Errorf("root.txt content = %q, want %q", content, "root")
	}

	// Verify nested files
	content, err = os.ReadFile(filepath.Join(dstDir, "sub1", "file1.txt"))
	if err != nil {
		t.Fatalf("read sub1/file1.txt: %v", err)
	}
	if string(content) != "file1" {
		t.Errorf("sub1/file1.txt content = %q, want %q", content, "file1")
	}

	content, err = os.ReadFile(filepath.Join(dstDir, "sub1", "sub2", "file2.txt"))
	if err != nil {
		t.Fatalf("read sub1/sub2/file2.txt: %v", err)
	}
	if string(content) != "file2" {
		t.Errorf("sub1/sub2/file2.txt content = %q, want %q", content, "file2")
	}
}

func TestCopyDir_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	entries, _ := os.ReadDir(dstDir)
	if len(entries) != 0 {
		t.Errorf("expected empty dstDir, got %d entries", len(entries))
	}
}

func TestCopyDir_SourceNotFound(t *testing.T) {
	err := copyDir("/nonexistent/dir", "/tmp/dst")
	if err == nil {
		t.Error("copyDir() expected error for nonexistent source")
	}
}

func TestCopyDir_SymlinkRelative(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a target file and a relative symlink to it
	targetFile := filepath.Join(srcDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(srcDir, "link.txt")
	if err := os.Symlink("target.txt", symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Also test a nested relative symlink
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	nestedTarget := filepath.Join(srcDir, "sub", "nested.txt")
	if err := os.WriteFile(nestedTarget, []byte("nested-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	nestedLink := filepath.Join(srcDir, "sub", "nested-link.txt")
	if err := os.Symlink("../target.txt", nestedLink); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify relative symlink is preserved (not resolved)
	linkDst := filepath.Join(dstDir, "link.txt")
	info, err := os.Lstat(linkDst)
	if err != nil {
		t.Fatalf("Lstat link.txt: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("link.txt should be a symlink, got regular file")
	}
	target, err := os.Readlink(linkDst)
	if err != nil {
		t.Fatalf("Readlink link.txt: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("link.txt target = %q, want %q", target, "target.txt")
	}

	// Verify nested relative symlink is preserved
	nestedLinkDst := filepath.Join(dstDir, "sub", "nested-link.txt")
	info, err = os.Lstat(nestedLinkDst)
	if err != nil {
		t.Fatalf("Lstat nested-link.txt: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("nested-link.txt should be a symlink, got regular file")
	}
	target, err = os.Readlink(nestedLinkDst)
	if err != nil {
		t.Fatalf("Readlink nested-link.txt: %v", err)
	}
	if target != "../target.txt" {
		t.Errorf("nested-link.txt target = %q, want %q", target, "../target.txt")
	}
}

func TestCopyDir_SymlinkAbsolute(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a target file and an absolute symlink to it
	targetFile := filepath.Join(srcDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	absTarget, _ := filepath.Abs(targetFile)
	symlinkPath := filepath.Join(srcDir, "abs-link.txt")
	if err := os.Symlink(absTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify absolute symlink is preserved as-is
	linkDst := filepath.Join(dstDir, "abs-link.txt")
	info, err := os.Lstat(linkDst)
	if err != nil {
		t.Fatalf("Lstat abs-link.txt: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("abs-link.txt should be a symlink, got regular file")
	}
	target, err := os.Readlink(linkDst)
	if err != nil {
		t.Fatalf("Readlink abs-link.txt: %v", err)
	}
	if target != absTarget {
		t.Errorf("abs-link.txt target = %q, want %q", target, absTarget)
	}
}

func TestCopyDir_SymlinkAndRegularFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Mix of regular file, directory, and symlinks
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "regular.txt"), []byte("regular"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "sub", "data.txt"), []byte("data"), 0o644)
	os.Symlink("regular.txt", filepath.Join(srcDir, "rel-link.txt"))
	absTarget, _ := filepath.Abs(filepath.Join(srcDir, "regular.txt"))
	os.Symlink(absTarget, filepath.Join(srcDir, "abs-link.txt"))

	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify regular file is copied as regular file
	regularInfo, _ := os.Lstat(filepath.Join(dstDir, "regular.txt"))
	if regularInfo.Mode()&os.ModeSymlink != 0 {
		t.Error("regular.txt should NOT be a symlink")
	}
	content, _ := os.ReadFile(filepath.Join(dstDir, "regular.txt"))
	if string(content) != "regular" {
		t.Errorf("regular.txt content = %q, want %q", content, "regular")
	}

	// Verify relative symlink
	relLinkInfo, _ := os.Lstat(filepath.Join(dstDir, "rel-link.txt"))
	if relLinkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("rel-link.txt should be a symlink")
	}

	// Verify absolute symlink
	absLinkInfo, _ := os.Lstat(filepath.Join(dstDir, "abs-link.txt"))
	if absLinkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("abs-link.txt should be a symlink")
	}
}

// --- prefixSkipper / mergeSkipper tests ---

type mockSkipper struct {
	skipFn func(string) bool
}

func (m *mockSkipper) shouldSkipFile(name string) bool {
	return m.skipFn(name)
}

func TestPrefixSkipper(t *testing.T) {
	ps := prefixSkipper("app/")

	tests := []struct {
		name string
		want bool
	}{
		{"app/file.txt", false},
		{"app/sub/file.txt", false},
		{"other/file.txt", true},
		{"application/file.txt", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ps.shouldSkipFile(tt.name)
			if got != tt.want {
				t.Errorf("prefixSkipper(%q).shouldSkipFile() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMergeSkipper(t *testing.T) {
	// prefixSkipper skips files that DON'T have the prefix
	ms := mergeSkipper{
		&mockSkipper{skipFn: func(name string) bool { return !strings.HasPrefix(name, "app/") }},
		&mockSkipper{skipFn: func(name string) bool { return strings.HasSuffix(name, ".tmp") }},
	}

	tests := []struct {
		name string
		want bool
	}{
		{"app/file.txt", false},  // has prefix, no .tmp suffix
		{"app/file.tmp", true},   // has prefix, but has .tmp suffix
		{"other/file.txt", true}, // no prefix
		{"other/file.tmp", true}, // no prefix, has .tmp suffix
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ms.shouldSkipFile(tt.name)
			if got != tt.want {
				t.Errorf("mergeSkipper(%q).shouldSkipFile() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMergeSkipper_Empty(t *testing.T) {
	ms := mergeSkipper{}
	if ms.shouldSkipFile("any/file.txt") {
		t.Error("empty mergeSkipper should not skip any file")
	}
}

func TestMergeSkipper_AllSkip(t *testing.T) {
	ms := mergeSkipper{
		&mockSkipper{skipFn: func(name string) bool { return true }},
		&mockSkipper{skipFn: func(name string) bool { return true }},
	}
	if !ms.shouldSkipFile("any/file.txt") {
		t.Error("mergeSkipper with all-true skipper should skip all files")
	}
}
