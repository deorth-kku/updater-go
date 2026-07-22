//go:build linux

package process

import (
	"context"
	"fmt"

	"github.com/coreos/go-systemd/v22/dbus"
)

func (c *Controller) stopService(ctx context.Context) error {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return fmt.Errorf("systemd stop: connect: %w", err)
	}
	defer conn.Close()
	_, err = conn.StopUnitContext(ctx, c.imageName, "replace", nil)
	if err != nil {
		return fmt.Errorf("systemd stop: %s: %w", c.imageName, err)
	}
	return nil
}

func (c *Controller) startService(ctx context.Context) error {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return fmt.Errorf("systemd start: connect: %w", err)
	}
	defer conn.Close()
	_, err = conn.StartUnitContext(ctx, c.imageName, "replace", nil)
	if err != nil {
		return fmt.Errorf("systemd start: %s: %w", c.imageName, err)
	}
	return nil
}
