// Package process handles stopping and starting of application processes.
package process

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
	logger      *slog.Logger
	// stored holds the recorded (cmdline, cwd) of running processes captured
	// during a non-service stop, so they can be relaunched with the original
	// command line and working directory on start (gap #22).
	stored []procLaunch
}

// procLaunch is a recorded process launch (cmdline + cwd) for relaunch.
type procLaunch struct {
	cmdline []string
	cwd     string
}

// New creates a new process Controller.
func New(imageName string, logger *slog.Logger) *Controller {
	return &Controller{imageName: imageName, logger: logger}
}

// NewWithConfig creates a Controller with stop/start commands, service mode, and restart wait.
func NewWithConfig(imageName, stopCmd, startCmd string, service bool, restartWait int, logger *slog.Logger) *Controller {
	return &Controller{
		imageName:   imageName,
		stopCmd:     stopCmd,
		startCmd:    startCmd,
		service:     service,
		restartWait: restartWait,
		logger:      logger,
	}
}

// log returns the controller's logger, falling back to slog.Default when nil
// (e.g. in unit tests that construct a bare Controller struct literal).
func (c *Controller) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// Stop terminates the process by image name, stop_cmd, or service.
// After stopping, waits for RestartWait seconds before returning.
func (c *Controller) Stop(ctx context.Context) error {
	if c.imageName == "" && c.stopCmd == "" && !c.service {
		c.log().Debug("process stop skipped",
			"image", c.imageName,
			"reason", "no image_name, stop_cmd, or service configured",
			"result", "skip",
		)
		return nil
	}

	// Custom stop command takes priority
	if c.stopCmd != "" {
		c.log().Info("process stop strategy",
			"image", c.imageName,
			"reason", "custom stop_cmd configured, takes priority",
			"result", "run stop_cmd",
		)
		err := c.runCustomCmd(ctx, c.stopCmd)
		if err != nil {
			return err
		}
	} else if c.service {
		c.log().Info("process stop strategy",
			"image", c.imageName,
			"reason", "service mode enabled, no custom stop_cmd",
			"result", "stop service",
		)
		err := c.stopService(ctx)
		if err != nil {
			return err
		}
	} else {
		c.log().Info("process stop strategy",
			"image", c.imageName,
			"reason", "no stop_cmd and no service, terminate by image name",
			"result", "kill image",
		)
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
	// Record running processes' cmdline/cwd before killing so they can be
	// relaunched with the original command + working directory (gap #22).
	c.stored = findProcLaunches(c.imageName)
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

// findProcLaunches returns the recorded (cmdline, cwd) of processes whose
// comm (process name) matches name. This mirrors updater-rpc's psutil
// proc.name() matching. Implemented for Unix via /proc.
func findProcLaunches(name string) []procLaunch {
	var out []procLaunch
	if runtime.GOOS == "windows" {
		return out
	}
	procDir, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range procDir {
		if !e.IsDir() {
			continue
		}
		pidDir := filepath.Join("/proc", e.Name())
		// comm is the process name (matches psutil proc.name()).
		commBytes, err := os.ReadFile(filepath.Join(pidDir, "comm"))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(commBytes))
		if comm != name {
			continue
		}
		cl, err := os.ReadFile(filepath.Join(pidDir, "cmdline"))
		if err != nil {
			continue
		}
		cmdline := splitCmdline(cl)
		cwd, err := os.Readlink(filepath.Join(pidDir, "cwd"))
		if err != nil {
			cwd = ""
		}
		out = append(out, procLaunch{cmdline: cmdline, cwd: cwd})
	}
	return out
}

// splitCmdline splits a /proc/<pid>/cmdline null-separated blob into args.
func splitCmdline(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.Split(string(b), "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func (c *Controller) stopService(ctx context.Context) error {
	switch runtime.GOOS {
	case "windows":
		// updater-rpc uses `net <command> <service>` on Windows (gap #21).
		cmd := exec.CommandContext(ctx, "net", "stop", c.imageName)
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
		c.log().Debug("process start skipped",
			"image", c.imageName,
			"reason", "no image_name, start_cmd, or service configured",
			"result", "skip",
		)
		return nil
	}

	// Custom start command takes priority
	if c.startCmd != "" {
		c.log().Info("process start strategy",
			"image", c.imageName,
			"reason", "custom start_cmd configured, takes priority",
			"result", "run start_cmd",
		)
		return c.runCustomCmd(ctx, c.startCmd)
	}

	// Service mode
	if c.service {
		c.log().Info("process start strategy",
			"image", c.imageName,
			"reason", "service mode enabled, no custom start_cmd",
			"result", "start service",
		)
		return c.startService(ctx)
	}

	// Non-service restart: if we recorded the original (cmdline, cwd) of the
	// killed processes during stop (gap #22), relaunch them verbatim with
	// their original working directory. This preserves arguments/cwd that a
	// plain path-based launch would lose.
	if len(c.stored) > 0 && runtime.GOOS != "windows" {
		var firstErr error
		for _, pl := range c.stored {
			if len(pl.cmdline) == 0 {
				continue
			}
			c.log().Info("process start strategy",
				"image", c.imageName,
				"cmdline", strings.Join(pl.cmdline, " "),
				"cwd", pl.cwd,
				"reason", "relaunch recorded process with original cmdline/cwd (gap #22)",
				"result", "start recorded",
			)
			cmd := exec.CommandContext(ctx, pl.cmdline[0], pl.cmdline[1:]...)
			cmd.Stdout = nil
			cmd.Stderr = nil
			if pl.cwd != "" {
				cmd.Dir = pl.cwd
			}
			if err := cmd.Start(); err != nil {
				c.log().Warn("relaunch recorded process failed",
					"image", c.imageName, "error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		c.stored = nil
		return firstErr
	}

	// Launch by path (image_name is used for identification, path is the executable)
	if path == "" {
		c.log().Error("process start failed",
			"image", c.imageName,
			"reason", "no start_cmd/service and no executable path provided",
			"result", "error",
		)
		return fmt.Errorf("process start: no path provided for %s", c.imageName)
	}

	c.log().Info("process start strategy",
		"image", c.imageName,
		"path", path,
		"reason", "no start_cmd and no service, launch executable by path",
		"result", "start binary",
	)
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
		// updater-rpc uses `net <command> <service>` on Windows (gap #21).
		cmd := exec.CommandContext(ctx, "net", "start", c.imageName)
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
