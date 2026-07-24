package downloader

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aria2rpc "github.com/deorth-kku/aria2rpc-go"
)

// TestRemoteAria2Download starts a real aria2c subprocess and verifies
// the remote-dir/local-dir path mapping during download.
func TestRemoteAria2Download(t *testing.T) {
	if _, err := exec.LookPath("aria2c"); err != nil {
		t.Skip("aria2c not found in PATH")
	}

	logger := slog.Default()

	// --- Setup: create remote and local directories ---
	tmpDir := t.TempDir()
	remoteDir := filepath.Join(tmpDir, "remote-downloads")
	localDir := filepath.Join(tmpDir, "local-downloads")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatalf("create remote dir: %v", err)
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("create local dir: %v", err)
	}

	// Simulate NFS/SMB mount: symlink local -> remote so files are "visible" at both paths
	if err := os.Remove(localDir); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove existing local dir for symlink: %v", err)
	}
	if err := os.Symlink(remoteDir, localDir); err != nil {
		t.Fatalf("symlink local -> remote: %v", err)
	}

	// --- Setup: serve a test file via HTTP ---
	testContent := "hello aria2 remote download test"
	testFile := filepath.Join(tmpDir, "testfile.zip")
	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(tmpDir)))
	defer server.Close()
	dlURL := server.URL + "/testfile.zip"

	// --- Setup: start aria2c subprocess ---
	port := findFreePort(t)
	addr := fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
	secret := generateSecret()

	la, newSecret, err := StartLocalAria2(t.Context(), addr, secret, "", logger)
	if err != nil {
		t.Fatalf("start local aria2c: %v", err)
	}
	defer la.Stop()
	if newSecret != secret {
		t.Logf("using generated secret: %s", newSecret)
	}
	secret = newSecret

	// --- Create downloader with remote-dir and local-dir ---
	downloader, err := NewAria2Downloader(
		t.Context(),
		addr,
		secret,
		remoteDir,
		localDir,
		"",
		5,
		logger,
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("create downloader: %v", err)
	}
	defer downloader.Close()

	// --- Download ---
	filename := "testfile.zip"
	localPath, gid, err := downloader.Download(t.Context(), dlURL, filename, remoteDir+"/proj", nil)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	t.Logf("download completed: gid=%s, localPath=%s", gid, localPath)

	// --- Verify: the returned path should map remote -> local ---
	expectedLocalPath := strings.Replace(
		filepath.Join(remoteDir, "proj", filename),
		remoteDir,
		localDir,
		1,
	)
	if localPath != expectedLocalPath {
		t.Errorf("localPath = %q, want %q", localPath, expectedLocalPath)
	}

	// --- Verify: the file actually exists on the local filesystem ---
	// The file should be in the local dir (simulating NFS mount)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Errorf("file not found at local path: %s", localPath)
	} else if err != nil {
		t.Errorf("stat local path: %v", err)
	}

	// --- Verify: the file content is correct ---
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read local file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}

	// --- Verify: aria2 also has the file in remote dir ---
	remotePath := filepath.Join(remoteDir, "proj", filename)
	if _, err := os.Stat(remotePath); os.IsNotExist(err) {
		t.Logf("remote file not at %s (aria2 may use a different subdirectory)", remotePath)
	} else {
		t.Logf("remote file found at %s", remotePath)
	}
}

// TestRemoteAria2Download_PathMapping verifies the resolveLocalPath logic
// with various remote/local directory configurations.
func TestRemoteAria2Download_PathMapping(t *testing.T) {
	tests := []struct {
		name      string
		remoteDir string
		localDir  string
		aria2Path string
		wantLocal string
	}{
		{
			name:      "simple mapping",
			remoteDir: "/mnt/aria2/downloads",
			localDir:  "/mnt/nfs/downloads",
			aria2Path: "/mnt/aria2/downloads/proj/file.zip",
			wantLocal: "/mnt/nfs/downloads/proj/file.zip",
		},
		{
			name:      "trailing slash",
			remoteDir: "/mnt/aria2/downloads/",
			localDir:  "/mnt/nfs/downloads/",
			aria2Path: "/mnt/aria2/downloads/proj/file.zip",
			wantLocal: "/mnt/nfs/downloads/proj/file.zip",
		},
		{
			name:      "no remote dir - passthrough",
			remoteDir: "",
			localDir:  "",
			aria2Path: "/home/user/downloads/file.zip",
			wantLocal: "/home/user/downloads/file.zip",
		},
		{
			name:      "only remote dir set",
			remoteDir: "/remote",
			localDir:  "",
			aria2Path: "/remote/proj/file.zip",
			wantLocal: "/remote/proj/file.zip",
		},
		{
			name:      "only local dir set",
			remoteDir: "",
			localDir:  "/local",
			aria2Path: "/some/path/file.zip",
			wantLocal: "/some/path/file.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Aria2Downloader{
				remoteDir: tt.remoteDir,
				localDir:  tt.localDir,
			}
			stat := &aria2rpc.Status{
				Files: []aria2rpc.FileInfo{
					{Path: tt.aria2Path},
				},
			}
			got := d.resolveLocalPath(stat)
			if got != tt.wantLocal {
				t.Errorf("resolveLocalPath() = %q, want %q", got, tt.wantLocal)
			}
		})
	}
}

// findFreePort finds a free TCP port on localhost.
func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
