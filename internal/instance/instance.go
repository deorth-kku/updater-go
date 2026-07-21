// Package instance provides single-instance locking to prevent multiple
// updater processes from running simultaneously.
//
// Implementation mirrors the Python InstanceLock (utils/_InstanceLock.py):
//   - Lock file in OS temp directory with PID-based conflict detection
//   - Cross-platform file locking: flock(2) on Unix, LockFileEx on Windows
//   - Stale lock recovery: if the recorded PID no longer exists, the lock
//     is overwritten (matching Python's psutil.pid_exists() fallback)
package instance

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const lockFileName = "updater-rpc.lock"

// Lock represents a single-instance lock.
type Lock struct {
	path  string
	pfd   *os.File
	pid   int
	stale bool
	mu    sync.Mutex
}

// lockDir returns the OS temp directory where the lock file lives.
// Mirrors Python's InstanceLock.get_temp():
//   - Windows: %TMP% env var
//   - Unix: /tmp
func lockDir() string {
	if runtime.GOOS == "windows" {
		if tmp := os.Getenv("TMP"); tmp != "" {
			return tmp
		}
		if tmp := os.Getenv("TEMP"); tmp != "" {
			return tmp
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "AppData", "Local", "Temp")
		}
	}
	return os.TempDir()
}

// New creates and acquires a single-instance lock. If another updater instance
// is already running (detected by PID + process existence check), an error is
// returned. The caller must call Close() to release the lock.
func New(lockPath string) (*Lock, error) {
	path := lockPath
	if path == "" {
		path = filepath.Join(lockDir(), lockFileName)
	}

	l := &Lock{path: path}
	if err := l.acquire(); err != nil {
		return nil, err
	}
	return l, nil
}

// acquire opens the lock file and performs an exclusive non-blocking lock.
func (l *Lock) acquire() error {
	if dir := filepath.Dir(l.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("instance: mkdir %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("instance: open lock file %s: %w", l.path, err)
	}
	l.pfd = f

	if err := platformFlock(f); err != nil {
		f.Close()
		return fmt.Errorf("instance: flock lock: %w", err)
	}

	pid, err := readPIDFile(f)
	if err != nil {
		platformUnlock(f)
		f.Close()
		return fmt.Errorf("instance: read pid: %w", err)
	}

	if pid != 0 && pid != os.Getpid() {
		if isProcessAlive(pid) {
			platformUnlock(f)
			f.Close()
			return fmt.Errorf("instance: another updater is running (pid=%d)", pid)
		}
		l.stale = true
	}

	if err := writePIDFile(f, os.Getpid()); err != nil {
		platformUnlock(f)
		f.Close()
		return fmt.Errorf("instance: write pid: %w", err)
	}
	l.pid = os.Getpid()
	return nil
}

// Close releases the lock and removes the lock file.
func (l *Lock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	platformUnlock(l.pfd)
	os.Remove(l.path)
	return l.pfd.Close()
}

// PID returns the PID of the locking process.
func (l *Lock) PID() int { return l.pid }

// IsStale returns true if the lock was acquired over a stale lock file
// (the previous owner's PID was no longer alive).
func (l *Lock) IsStale() bool { return l.stale }

// Path returns the absolute path of the lock file.
func (l *Lock) Path() string { return l.path }

// readPIDFile reads the first line of the lock file as a PID.
func readPIDFile(f *os.File) (int, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	buf := make([]byte, 32)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return 0, err
	}
	s := strings.TrimSpace(string(buf[:n]))
	if s == "" {
		return 0, err
	}
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return p, nil
}

// writePIDFile writes the PID to the lock file.
func writePIDFile(f *os.File, pid int) error {
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := fmt.Fprintf(f, "%d", pid)
	return err
}
