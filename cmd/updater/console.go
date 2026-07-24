//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleProcessList = modkernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = modkernel32.NewProc("FreeConsole")
)

func init() {
	var processIDs [2]uint32
	r1, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&processIDs[0])),
		uintptr(2),
	)

	if r1 <= 1 {
		procFreeConsole.Call()
	}
}
