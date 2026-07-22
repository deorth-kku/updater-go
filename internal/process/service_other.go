//go:build !linux && !windows

package process

import "context"

func (c *Controller) stopService(ctx context.Context) error {
	return nil
}

func (c *Controller) startService(ctx context.Context) error {
	return nil
}
