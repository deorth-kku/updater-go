package process

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSplitCmdline verifies /proc cmdline parsing.
func TestSplitCmdline(t *testing.T) {
	got := splitCmdline([]byte("foo\x00bar baz\x00qux\x00"))
	want := []string{"foo", "bar baz", "qux"}
	if len(got) != len(want) {
		t.Fatalf("splitCmdline() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitCmdline()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestFindProcLaunches_RecordsCmdlineCwd verifies gap #22: processes matching
// the image name are recorded with their cmdline and cwd.
func TestFindProcLaunches_RecordsCmdlineCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("findProcLaunches is implemented for unix /proc")
	}
	cwd := t.TempDir()
	// A script whose comm matches the looked-up name; scripts keep a readable
	// /proc/<pid>/cmdline in this environment (unlike direct /bin/sleep).
	procprobe := filepath.Join(cwd, "procprobe")
	if err := os.WriteFile(procprobe, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(procprobe, "arg1", "arg two")
	cmd.Dir = cwd
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Wait for it to appear in /proc.
	var launches []procLaunch
	deadline := time.After(5 * time.Second)
	for {
		launches = findProcLaunches("procprobe")
		if len(launches) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("procprobe never appeared in /proc")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(launches[0].cmdline) < 2 {
		t.Errorf("cmdline = %v, want at least 2 args", launches[0].cmdline)
	}
	// The script is launched via its shebang interpreter; argv[1] is the script
	// path which embeds the comm name "procprobe".
	if !strings.Contains(launches[0].cmdline[1], "procprobe") {
		t.Errorf("cmdline = %v, want it to contain the script path", launches[0].cmdline)
	}
	if launches[0].cwd != cwd {
		t.Errorf("cwd = %q, want %q", launches[0].cwd, cwd)
	}
}

// TestStopStart_PreservesCmdlineCwd verifies gap #22 end-to-end: stopping a
// process by image name records it, and a subsequent Start relaunches it (the
// process becomes running again with the recorded identity).
func TestStopStart_PreservesCmdlineCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmdline/cwd preservation is implemented for unix")
	}
	workdir := t.TempDir()
	marker := filepath.Join(workdir, "relaunched-marker")

	// White-box: simulate that Stop recorded a process launch whose cmdline
	// writes a marker file into its cwd. This exercises the relaunch path of
	// gap #22 (Start uses the recorded cmdline + cwd, not just the exe path).
	ctrl := New("longrun", slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	ctrl.stored = []procLaunch{
		{
			cmdline: []string{"/bin/sh", "-c", "touch '" + marker + "'"},
			cwd:     workdir,
		},
	}

	if err := ctrl.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give the relaunched process a moment to create the marker.
	deadline := time.After(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("relaunched process did not write marker (cmdline/cwd not preserved)")
		case <-time.After(50 * time.Millisecond):
		}
	}
}
