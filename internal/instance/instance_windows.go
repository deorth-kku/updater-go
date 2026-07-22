//go:build windows

package instance

import (
	"fmt"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

type windowsLock struct {
	path string
	h    windows.Handle
}

func new(path string) (Lock, error) {
	if path == "" {
		path = lockFileName
	}
	name := filepath.Join("Global", path)
	pw, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("instance: UTF16PtrFromString: %w", err)
	}
	h, err := windows.CreateMutex(nil, false, pw)
	if err != nil {
		return nil, fmt.Errorf("instance: CreateMutexW: %w", err)
	}
	if err := windows.GetLastError(); err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("instance: another updater is running (mutex=%s)", name)
	}
	return &windowsLock{path: path, h: h}, nil
}

func (w *windowsLock) Close() error {
	return windows.CloseHandle(w.h)
}

func (w *windowsLock) Path() string { return w.path }
