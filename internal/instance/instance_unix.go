//go:build !windows

package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type unixLock struct {
	f *os.File
}

func new(path string) (Lock, error) {
	if path == "" {
		path = filepath.Join(os.TempDir(), lockFileName)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("instance: mkdir %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("instance: open lock file %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("instance: flock lock: %w", err)
	}
	return unixLock{f: f}, nil
}

func (u unixLock) Close() error {
	err := syscall.Flock(int(u.f.Fd()), syscall.LOCK_UN)
	ferr := u.f.Close()
	rerr := os.Remove(u.Path())
	return errors.Join(err, ferr, rerr)
}

func (u unixLock) Path() string { return u.f.Name() }
