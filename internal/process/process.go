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
	imageName   string
	stopCmd     string
	startCmd    string
	service     bool
	restartWait int
}

// New creates a new process Controller.
func New(imageName string) *Controller {
	return &Controller{imageName: imageName}
}

// NewWithConfig creates a Controller with stop/start commands, service mode, and restart wait.
func NewWithConfig(imageName, stopCmd, startCmd string, service bool, restartWait int) *Controller {
	return &Controller{
		imageName:   imageName,
		stopCmd:     stopCmd,
		startCmd:    startCmd,
		service:     service,
		restartWait: restartWait,
	}
}

// Stop terminates the process by image name, stop_cmd, or service.
// After stopping, waits for RestartWait seconds before returning.
func (c *Controller) Stop(ctx context.Context) error {
	if c.imageName == "" && c.stopCmd == "" && !c.service {
		return nil
	}

	// Custom stop command takes priority
	if c.stopCmd != "" {
		err := c.runCustomCmd(ctx, c.stopCmd)
		if err != nil {
			return err
		}
	} else if c.service {
		err := c.stopService(ctx)
		if err != nil {
			return err
		}
	} else {
		switch runtime.GOOS {
		case "windows":
			err := c.stopWindows(ctx)
			if err != nil {
				return err
			}
		default:
			err := c.stopUnix(ctx)
			if err != nil {
				return err
			}
		}
	}

	// Wait for restart_wait seconds
	if c.restartWait > 0 {
		time.Sleep(time.Duration(c.restartWait) * time.Second)
	}

	return nil
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

func (c *Controller) stopService(ctx context.Context) error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.CommandContext(ctx, "sc", "stop", c.imageName)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run()
	default:
		cmd := exec.CommandContext(ctx, "systemctl", "stop", c.imageName)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run()
	}
}

func (c *Controller) runCustomCmd(ctx context.Context, cmdStr string) error {
	// Split the command string into args (simple space-split, no shell expansion)
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// Start launches the process by image name, start_cmd, or service.
func (c *Controller) Start(ctx context.Context, path string) error {
	if c.imageName == "" && c.startCmd == "" && !c.service {
		return nil
	}

	// Custom start command takes priority
	if c.startCmd != "" {
		return c.runCustomCmd(ctx, c.startCmd)
	}

	// Service mode
	if c.service {
		return c.startService(ctx)
	}

	// Launch by path (image_name is used for identification, path is the executable)
	if path == "" {
		return fmt.Errorf("process start: no path provided for %s", c.imageName)
	}

	switch runtime.GOOS {
	case "windows":
		cmd := exec.CommandContext(ctx, path)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Start()
	default:
		cmd := exec.CommandContext(ctx, path)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Start()
	}
}

// StartService starts a system service.
func (c *Controller) startService(ctx context.Context) error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.CommandContext(ctx, "sc", "start", c.imageName)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run()
	default:
		cmd := exec.CommandContext(ctx, "systemctl", "start", c.imageName)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run()
	}
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
