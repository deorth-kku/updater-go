//go:build !windows

package instance

import (
	"os"
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
