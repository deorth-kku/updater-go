//go:build windows

package process

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func (c *Controller) stopService(ctx context.Context) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("windows service stop: connect SCM: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(c.imageName)
	if err != nil {
		return fmt.Errorf("windows service stop: open %s: %w", c.imageName, err)
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("windows service stop: %s: %w", c.imageName, err)
	}
	return nil
}

func (c *Controller) startService(ctx context.Context) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("windows service start: connect SCM: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(c.imageName)
	if err != nil {
		return fmt.Errorf("windows service start: open %s: %w", c.imageName, err)
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		return fmt.Errorf("windows service start: %s: %w", c.imageName, err)
	}
	return nil
}
