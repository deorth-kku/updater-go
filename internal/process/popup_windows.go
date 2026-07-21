//go:build windows

package process

import (
	"golang.org/x/sys/windows"
)

// PopupMsg shows a Windows message box with the given title and message.
// On non-Windows platforms this function is a no-op (see popup_other.go).
func (c *Controller) PopupMsg(title, msg string) error {
	if msg == "" {
		msg = title
	}
	titleW, _ := windows.UTF16PtrFromString(title)
	msgW, _ := windows.UTF16PtrFromString(msg)
	_, err := windows.MessageBox(0, msgW, titleW, windows.MB_OK|windows.MB_TOPMOST)
	return err
}
