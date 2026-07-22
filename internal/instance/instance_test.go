package instance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	c, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	// Verify lock file exists.
	if _, err := os.Stat(c.Path()); os.IsNotExist(err) {
		t.Errorf("lock file %s does not exist", c.Path())
	}

	// Verify path is correct.
	if c.Path() != lockPath {
		t.Errorf("Path() = %q, want %q", c.Path(), lockPath)
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

	// Second instance with the same path should fail (flock prevents it).
	_, err = New(lockPath)
	if err == nil {
		t.Fatal("expected error from second New() with same lock path, got nil")
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

	// Acquire lock — should succeed (no PID check on Linux).
	_, err = New(lockPath)
	if err != nil {
		t.Fatalf("expected success from New(), got error: %v", err)
	}
}

func TestCloseRemovesLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	c, err := New(lockPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	lockP := c.Path()

	// Lock file should exist.
	if _, err := os.Stat(lockP); os.IsNotExist(err) {
		t.Errorf("lock file %s does not exist before Close()", lockP)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Lock file should be removed.
	if _, err := os.Stat(lockP); !os.IsNotExist(err) {
		t.Errorf("lock file %s still exists after Close()", lockP)
	}
}

func TestDefaultLockPath(t *testing.T) {
	c, err := New("")
	if err != nil {
		t.Fatalf("New(\"\") error = %v", err)
	}
	defer c.Close()

	// Path should contain the lock file name.
	if filepath.Base(c.Path()) != lockFileName {
		t.Errorf("default path base = %q, want %q", filepath.Base(c.Path()), lockFileName)
	}
}
