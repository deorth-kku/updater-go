//go:build !windows

package process

// PopupMsg is a no-op on non-Windows platforms.
func (c *Controller) PopupMsg(title, msg string) {}
