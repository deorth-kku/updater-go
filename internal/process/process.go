// Package process handles stopping and starting of application processes.
package process

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Controller manages a named process.
type Controller struct {
	imageName string
}

// New creates a new process Controller.
func New(imageName string) *Controller {
	return &Controller{imageName: imageName}
}

// Stop terminates the process by image name.
func (c *Controller) Stop(ctx context.Context) error {
	if c.imageName == "" {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return c.stopWindows(ctx)
	default:
		return c.stopUnix(ctx)
	}
}

func (c *Controller) stopUnix(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pkill", "-f", c.imageName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (c *Controller) stopWindows(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "taskkill", "/IM", c.imageName, "/F")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// Start launches the process by image name (placeholder — actual path needed).
func (c *Controller) Start(ctx context.Context, path string) error {
	if c.imageName == "" {
		return nil
	}

	// In a full implementation, this would resolve the binary path and start it.
	// For now, log that we would start the process.
	return fmt.Errorf("process start not yet implemented for %s", c.imageName)
}

// WaitForStop waits for the process to actually terminate.
func (c *Controller) WaitForStop(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for process %s to stop", c.imageName)
		case <-ticker.C:
			if !c.IsRunning() {
				return nil
			}
		}
	}
}

// IsRunning checks if a process with the given image name is running.
func (c *Controller) IsRunning() bool {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", c.imageName))
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), c.imageName)
	default:
		cmd := exec.Command("pgrep", "-x", c.imageName)
		return cmd.Run() == nil
	}
}
