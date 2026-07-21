//go:build !windows

package instance

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// platformFlock acquires an exclusive non-blocking file lock using flock(2).
func platformFlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// platformUnlock releases the file lock.
func platformUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// isProcessAlive checks whether a process with the given PID is still running
// by sending signal 0 (no-op signal) via syscall.Kill.
func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

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
		return 0, nil
	}
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, nil
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
