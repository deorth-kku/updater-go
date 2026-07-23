//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
)

const (
	ATTACH_PARENT_PROCESS uint32 = ^uint32(0) // -1
)

func InitConsoleHandles() error {
	// Retrieve standard handles.
	hIn, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard input handler.")
	}
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard output handler.")
	}
	hErr, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard error handler.")
	}

	// Wrap handles in files. /dev/ prefix just to make it conventional.
	stdInF := os.NewFile(uintptr(hIn), "/dev/stdin")
	if stdInF == nil {
		return errors.New("Failed to create a new file for standard input.")
	}
	stdOutF := os.NewFile(uintptr(hOut), "/dev/stdout")
	if stdOutF == nil {
		return errors.New("Failed to create a new file for standard output.")
	}
	stdErrF := os.NewFile(uintptr(hErr), "/dev/stderr")
	if stdErrF == nil {
		return errors.New("Failed to create a new file for standard error.")
	}

	// Set handles for standard input, output and error devices.
	if err := windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(stdInF.Fd())); err != nil {
		return errors.New("Failed to set standard input handler.")
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(stdOutF.Fd())); err != nil {
		return errors.New("Failed to set standard output handler.")
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(stdErrF.Fd())); err != nil {
		return errors.New("Failed to set standard error handler.")
	}

	// Update golang standard IO file descriptors.
	os.Stdin = stdInF
	os.Stdout = stdOutF
	os.Stderr = stdErrF

	return nil
}

func init() {
	r1, _, _ := procAttachConsole.Call(uintptr(ATTACH_PARENT_PROCESS))

	if r1 == 0 {
		return
	}

	InitConsoleHandles()
}
