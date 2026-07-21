package instance

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	l, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer l.Close()

	// Verify lock file exists.
	if _, err := os.Stat(l.path); os.IsNotExist(err) {
		t.Errorf("lock file %s does not exist", l.path)
	}

	// Verify PID is recorded.
	if l.PID() == 0 {
		t.Error("PID is zero")
	}
	if l.PID() != os.Getpid() {
		t.Errorf("PID = %d, want %d", l.PID(), os.Getpid())
	}

	// Verify path is correct.
	if l.Path() != lockPath {
		t.Errorf("Path() = %q, want %q", l.Path(), lockPath)
	}
}

func TestLockPreventsSecondInstance(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	l1, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() first instance error = %v", err)
	}
	defer l1.Close()

	// Second instance with the same path should fail.
	_, err = New(lockPath)
	if err == nil {
		t.Fatal("expected error from second New() with same lock path, got nil")
	}
}

func TestLockRecoveryOnStaleProcess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	// Write a PID that doesn't exist (use a very high PID).
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	// Write a known-dead PID.
	f.Truncate(0)
	f.Seek(0, 0)
	f.WriteString("99999999")
	f.Close()

	// Acquire lock — should succeed (stale lock recovery).
	l, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer l.Close()

	if !l.IsStale() {
		t.Error("expected IsStale() = true for recovered stale lock")
	}
}

func TestLockRecoveryOnAliveProcess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	// Spawn a child process that sleeps, so we have a known-alive PID.
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer cmd.Process.Kill()

	// Write child's PID to the lock file.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	f.Truncate(0)
	f.Seek(0, 0)
	f.WriteString(strconv.Itoa(cmd.Process.Pid))
	f.Close()

	// Acquire lock — should fail because child PID is alive.
	_, err = New(lockPath)
	if err == nil {
		t.Fatal("expected error from New() when recorded PID is alive, got nil")
	}
}

func TestLockDir(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		// On Windows, TMP or TEMP should be set.
		// Just verify lockDir returns something non-empty.
		d := lockDir()
		if d == "" {
			t.Error("lockDir() returned empty string")
		}
	default:
		if got := lockDir(); got != "/tmp" {
			t.Errorf("lockDir() = %q, want /tmp", got)
		}
	}
}

func TestCloseRemovesLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	l, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Lock file should exist.
	if _, err := os.Stat(l.path); os.IsNotExist(err) {
		t.Errorf("lock file %s does not exist before Close()", l.path)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Lock file should be removed.
	if _, err := os.Stat(l.path); !os.IsNotExist(err) {
		t.Errorf("lock file %s still exists after Close()", l.path)
	}
}

func TestDefaultLockPath(t *testing.T) {
	l, err := New("")
	if err != nil {
		t.Fatalf("New(\"\") error = %v", err)
	}
	defer l.Close()

	// Path should contain the lock file name.
	if filepath.Base(l.Path()) != lockFileName {
		t.Errorf("default path base = %q, want %q", filepath.Base(l.Path()), lockFileName)
	}
}
