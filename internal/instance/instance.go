// Package instance provides single-instance locking to prevent multiple
// updater processes from running simultaneously.
//
// Implementation:
//   - Linux: flock(2) on a lock file in the OS temp directory
//   - Windows: CreateMutexW global mutex (path prefixed with "Global\")
package instance

import (
	"io"
)

const lockFileName = "updater-rpc.lock"

// Lock is the return type of New(). It combines Close() with Path().
type Lock interface {
	io.Closer
	Path() string
}

// New creates and acquires a single-instance lock. If another updater instance
// is already running, an error is returned.
func New(lockPath string) (Lock, error) {
	return new(lockPath)
}
