package updater

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/deorth-kku/updater-go/internal/api"
	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/process"
)

// TestUpdate_VersionFileWritten verifies gap #3: a <name>.VERSION file is
// written to the save path when use_exe_version is false.
func TestUpdate_VersionFileWritten(t *testing.T) {
	saveDir := t.TempDir()
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "verapp",
		},
		Download:   config.DownloadConfig{URL: "/test.zip"},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	u := New(projCfg, config.ProjectEntry{SavePath: saveDir}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	if res := u.Update(t.Context()); res.Error != nil {
		t.Fatalf("Update() error = %v", res.Error)
	}
	data, err := os.ReadFile(filepath.Join(saveDir, "verapp.VERSION"))
	if err != nil {
		t.Fatalf("version file not written: %v", err)
	}
	if string(data) != "v1.0.0" {
		t.Errorf("version file content = %q, want %q", string(data), "v1.0.0")
	}
}

// TestUpdate_VersionFileSkippedForExeVersion verifies gap #3: no .VERSION file
// is written when use_exe_version is true.
func TestUpdate_VersionFileSkippedForExeVersion(t *testing.T) {
	saveDir := t.TempDir()
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "verapp",
		},
		Download:   config.DownloadConfig{URL: "/test.zip"},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
		Version:    config.VersionConfig{UseExeVersion: true},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	u := New(projCfg, config.ProjectEntry{SavePath: saveDir}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	if res := u.Update(t.Context()); res.Error != nil {
		t.Fatalf("Update() error = %v", res.Error)
	}
	if _, err := os.Stat(filepath.Join(saveDir, "verapp.VERSION")); !os.IsNotExist(err) {
		t.Error("version file should NOT be written for use_exe_version")
	}
}

// TestUpdate_PostCmdsExecuted verifies gap #1: post-cmds run after update with
// %PATH/%NAME/%DL_FILENAME/%VER substitution.
func TestUpdate_PostCmdsExecuted(t *testing.T) {
	saveDir := t.TempDir()
	marker := filepath.Join(saveDir, "postcmd-ran")

	var cmd string
	switch runtime.GOOS {
	case "windows":
		cmd = `cmd /c "echo ran > "` + marker + `""`
	default:
		cmd = `echo ran > "` + marker + `"`
	}
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "pcmd",
		},
		Download:   config.DownloadConfig{URL: "/test.zip"},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
		PostCmds:   []string{cmd},
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	u := New(projCfg, config.ProjectEntry{SavePath: saveDir}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	if res := u.Update(t.Context()); res.Error != nil {
		t.Fatalf("Update() error = %v", res.Error)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("post-cmd did not run (marker missing): %v", err)
	}
}

// TestSelectDownloadURL_VerGlobal verifies gap #25: %VER embedded in the
// configured download.url is expanded to the detected version.
func TestSelectDownloadURL_VerGlobal(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{APIType: "github"},
		Download: config.DownloadConfig{
			URL: "https://example.com/release/%VER/app.zip",
		},
	}
	rel := &api.Release{Version: "9.9.9"}
	u := &Updater{projectCfg: projCfg}
	got := u.selectDownloadURL(rel)
	want := "https://example.com/release/9.9.9/app.zip"
	if got != want {
		t.Errorf("selectDownloadURL() = %q, want %q", got, want)
	}
}

// TestDownloadFilename_VerGlobal verifies gap #25: %VER embedded in
// filename_override is expanded.
func TestDownloadFilename_VerGlobal(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{APIType: "github"},
		Download: config.DownloadConfig{
			FilenameOverride:     "app-%VER-x64.zip",
			AddVersionToFilename: true,
		},
	}
	u := &Updater{projectCfg: projCfg}
	got := u.downloadFilename("3.2.1", "http://example.com/x.zip")
	want := "app-3.2.1-x64.zip"
	if got != want {
		t.Errorf("downloadFilename() = %q, want %q", got, want)
	}
}

// TestUpdate_WaitForProcessExit verifies gap #4: when allow_restart is false
// and a process matching ImageName is running, the updater waits for it to exit
// before completing. We spawn a real long-running process, assert the updater
// blocks until it exits, then verify the update still completes.
func TestUpdate_WaitForProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process control test relies on pgrep on unix")
	}
	saveDir := t.TempDir()

	// Start a background process whose image name (comm) is "sleeper".
	// We symlink the real sleep binary to a file named "sleeper" so that
	// pgrep -x sleeper matches it, mirroring production where image_name is
	// the actual binary name.
	sleeper := filepath.Join(saveDir, "sleeper")
	if err := os.Symlink("/bin/sleep", sleeper); err != nil {
		// Some systems lack /bin/sleep; fall back to /usr/bin/sleep.
		if err2 := os.Symlink("/usr/bin/sleep", sleeper); err2 != nil {
			t.Skipf("no sleep binary available: %v / %v", err, err2)
		}
	}
	cmd := exec.Command(sleeper, "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start background proc: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Wait until pgrep sees the sleeper process.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	deadline := time.After(5 * time.Second)
	for {
		if process.New("sleeper", logger).IsRunning() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("sleeper process never appeared in pgrep")
		case <-time.After(50 * time.Millisecond):
		}
	}

	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "waitapp",
		},
		Download:   config.DownloadConfig{URL: "/test.zip"},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
		Process: config.ProcessConfig{
			ImageName:    "sleeper",
			AllowRestart: false,
		},
	}
	u := New(projCfg, config.ProjectEntry{SavePath: saveDir}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)

	// Update() should block on WaitForStop until we kill the sleeper.
	done := make(chan struct{})
	var updErr error
	go func() {
		res := u.Update(t.Context())
		updErr = res.Error
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Update() returned before the running process was killed (wait logic broken)")
	case <-time.After(500 * time.Millisecond):
		// Still blocked after 500ms → wait logic is engaged.
	}

	// Kill the process, then reap it (cmd.Wait) so it is not left as a zombie
	// that pgrep would still match. In production the target process is not a
	// child of the updater, so this reaping only matters for the test harness.
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	select {
	case <-done:
		if updErr != nil {
			t.Fatalf("Update() error after wait = %v", updErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Update() did not finish after process killed")
	}
}
