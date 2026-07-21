//go:build windows

package instance

import (
	"os"

	"golang.org/x/sys/windows"
)

// platformFlock acquires an exclusive non-blocking file lock using LockFileEx.
func platformFlock(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, // lock 1 byte
		0,
		&ol,
	)
}

// platformUnlock releases the file lock.
func platformUnlock(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		0,
		0,
		1,
		0,
		&ol,
	)
}

// isProcessAlive checks whether a process with the given PID is still running
// by attempting to open it with PROCESS_QUERY_INFORMATION access.
func isProcessAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}
