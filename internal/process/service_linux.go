//go:build linux

package process

import (
	"context"
	"fmt"
	"os"

	"github.com/coreos/go-systemd/v22/dbus"
)

var (
	isRoot = os.Getuid() == 0
)

func getConn(ctx context.Context) (*dbus.Conn, error) {
	if isRoot {
		return dbus.NewSystemConnectionContext(ctx)
	}
	return dbus.NewUserConnectionContext(ctx)
}

func (c *Controller) stopService(ctx context.Context) error {
	conn, err := getConn(ctx)
	if err != nil {
		return fmt.Errorf("systemd stop: connect: %w", err)
	}
	defer conn.Close()

	unit := c.imageName + ".service"
	// Pass a result channel so the call blocks until the stop job finishes,
	// mirroring `systemctl stop` (which waits on the JobRemoved signal).
	// A buffered channel keeps the Conn free even if we stop reading (e.g. on
	// ctx cancellation).
	ch := make(chan string, 1)
	if _, err = conn.StopUnitContext(ctx, unit, "replace", ch); err != nil {
		return fmt.Errorf("systemd stop: %s: %w", c.imageName, err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-ch:
		// Only "done" (stopped) and "skipped" (already stopped) guarantee the
		// unit is no longer running, so it's safe to proceed (e.g. to
		// extraction). Any other result leaves the unit state unknown.
		if result != "done" && result != "skipped" {
			return fmt.Errorf("systemd stop: %s: job result %q", c.imageName, result)
		}
	}
	return nil
}

func (c *Controller) startService(ctx context.Context) error {
	conn, err := getConn(ctx)
	if err != nil {
		return fmt.Errorf("systemd start: connect: %w", err)
	}
	defer conn.Close()
	_, err = conn.StartUnitContext(ctx, c.imageName+".service", "replace", nil)
	if err != nil {
		return fmt.Errorf("systemd start: %s: %w", c.imageName, err)
	}
	return nil
}
