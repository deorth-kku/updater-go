//go:build windows

package instance

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

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
