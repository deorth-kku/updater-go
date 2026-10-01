// Package adb wraps go-adbkit to provide ADB-based APK installation. It adds a
// local-only server auto-start guard (mirroring the aria2 local fallback) and a
// flag-configurable Install that prefers streaming install with a push fallback.
package adb

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gadb "github.com/codeskyblue/go-adbkit/adb"
)

// installResultTimeout bounds the final read of the install result after the
// APK has been streamed, in case the server does not close the connection.
const installResultTimeout = 10 * time.Minute

// tempSeq makes push-install temp file names unique within a process.
var tempSeq atomic.Uint64

// Installer installs an APK to a device. The updater depends on this interface
// so it can be satisfied by the real ADB client or a test double.
type Installer interface {
	// Install pushes/streams apkPath to the device identified by serial and
	// installs it with the given pm install flags (e.g. "-r -d").
	Install(ctx context.Context, serial, apkPath, flags string) (string, error)
}

// Client is a thin wrapper over go-adbkit's *adb.Client that adds a local-only
// server auto-start guard and a flag-configurable Install.
type Client struct {
	inner  *gadb.Client
	logger *slog.Logger

	// deviceLocks serializes installs per device serial: concurrent pm install
	// / push operations on the same device would corrupt each other.
	lockMu      sync.Mutex
	deviceLocks map[string]*sync.Mutex
}

// deviceLock returns the mutex guarding installs for the given serial,
// creating it on first use.
func (c *Client) deviceLock(serial string) *sync.Mutex {
	c.lockMu.Lock()
	defer c.lockMu.Unlock()
	if c.deviceLocks == nil {
		c.deviceLocks = make(map[string]*sync.Mutex)
	}
	m, ok := c.deviceLocks[serial]
	if !ok {
		m = &sync.Mutex{}
		c.deviceLocks[serial] = m
	}
	return m
}

var _ Installer = (*Client)(nil)

// connector dials the ADB server. It implements go-adbkit's Connector
// interface and, for local addresses, starts the server via the adb binary when
// the initial connection fails (mirroring the aria2 local-only guard).
type connector struct {
	host   string
	port   string
	bin    string
	local  bool
	logger *slog.Logger
}

// ConnectionContext implements gadb.Connector.
func (c *connector) ConnectionContext(ctx context.Context) (net.Conn, error) {
	addr := net.JoinHostPort(c.host, c.port)
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err == nil {
		return conn, nil
	}

	// Only attempt to auto-start the server for local addresses. A remote
	// server must already be running; spawning a local adb would not help.
	if !c.local {
		return nil, fmt.Errorf("connect adb server at %s: %w", addr, err)
	}

	c.logger.Info("adb server unreachable, starting local server",
		"addr", addr,
		"bin", c.bin,
		"reason", "connection failed on local address",
		"result", "start-server",
	)
	if err := exec.CommandContext(ctx, c.bin, "start-server").Run(); err != nil {
		c.logger.Warn("adb start-server failed", "bin", c.bin, "error", err)
	}

	// Retry a few times while the server comes up.
	var lastErr error
	for range 10 {
		conn, lastErr = dialer.DialContext(ctx, "tcp", addr)
		if lastErr == nil {
			return conn, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("connect adb server at %s after start-server: %w", addr, lastErr)
}

// NewClientOrLocal creates an ADB client connected to host:port. When the
// server is unreachable and the address is local, it starts the server via the
// adb binary (bin) and retries. It returns an error if the server cannot be
// reached.
func NewClientOrLocal(ctx context.Context, host, port, bin string, local bool, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	conn := &connector{host: host, port: port, bin: bin, local: local, logger: logger}
	inner := gadb.NewClientWithConnector(conn)

	// Verify the server is reachable and responsive.
	if _, err := inner.Version(); err != nil {
		return nil, fmt.Errorf("connect adb server at %s:%s: %w", host, port, err)
	}

	return &Client{inner: inner, logger: logger}, nil
}

// DeviceList returns the attached devices (for diagnostics).
func (c *Client) DeviceList(ctx context.Context) ([]gadb.DeviceInfo, error) {
	return c.inner.ListDevices()
}

// Install installs apkPath to the device with the given serial, using a
// streaming install (abb_exec) when the server advertises it, otherwise a push +
// pm install fallback. flags are forwarded to pm install (e.g. "-r -d").
func (c *Client) Install(ctx context.Context, serial, apkPath, flags string) (string, error) {
	// Serialize installs per device: concurrent push/pm install on the same
	// serial would overwrite or delete each other's APK.
	lock := c.deviceLock(serial)
	lock.Lock()
	defer lock.Unlock()

	dev := c.inner.Device(gadb.DeviceWithSerial(serial))

	if c.hasAbbExec() {
		if out, err := c.installStreaming(ctx, dev, apkPath, flags); err == nil {
			return out, nil
		} else {
			c.logger.Warn("streaming install failed, falling back to push",
				"serial", serial,
				"error", err,
				"result", "fallback",
			)
		}
	}
	return c.installPush(ctx, dev, apkPath, flags)
}

// hasAbbExec reports whether the ADB server advertises the abb_exec feature
// (streaming install).
func (c *Client) hasAbbExec() bool {
	features, err := c.inner.Features()
	if err != nil {
		return false
	}
	for _, f := range features {
		if f == "abb_exec" {
			return true
		}
	}
	return false
}

// installStreaming performs a streaming install via the abb_exec protocol:
//
//	abb_exec:package install <flags> -S <size>
//
// followed by streaming the APK bytes. No temp file is written to the device.
func (c *Client) installStreaming(ctx context.Context, dev *gadb.Device, apkPath, flags string) (string, error) {
	file, err := os.Open(apkPath)
	if err != nil {
		return "", fmt.Errorf("open apk %s: %w", apkPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat apk %s: %w", apkPath, err)
	}

	transport, err := dev.TransportContext(ctx)
	if err != nil {
		return "", fmt.Errorf("open transport: %w", err)
	}
	defer transport.Close()

	cmd := fmt.Sprintf("abb_exec:package install -S %d", info.Size())
	if f := strings.TrimSpace(flags); f != "" {
		cmd = fmt.Sprintf("abb_exec:package install %s -S %d", f, info.Size())
	}
	if _, err := transport.Write([]byte(fmt.Sprintf("%04x%s", len(cmd), cmd))); err != nil {
		return "", fmt.Errorf("send abb_exec command: %w", err)
	}
	status, err := transport.ReadStatus()
	if err != nil {
		return "", fmt.Errorf("read abb_exec status: %w", err)
	}
	if status != "OKAY" {
		return "", fmt.Errorf("abb_exec rejected: %s", status)
	}

	if _, err := io.Copy(transport, file); err != nil {
		return "", fmt.Errorf("stream apk: %w", err)
	}
	// The server is expected to close the connection once the install
	// finishes; bound the final read so a misbehaving server cannot hang us.
	if err := transport.SetDeadline(time.Now().Add(installResultTimeout)); err != nil {
		return "", fmt.Errorf("set read deadline: %w", err)
	}
	out, err := io.ReadAll(transport)
	if err != nil {
		return "", fmt.Errorf("read install result: %w", err)
	}
	result := string(out)
	if !strings.Contains(result, "Success") {
		return result, fmt.Errorf("install failed: %s", strings.TrimSpace(result))
	}
	return result, nil
}

// installPush performs a traditional install: push the APK to a temp path on the
// device, run pm install, then clean up.
func (c *Client) installPush(ctx context.Context, dev *gadb.Device, apkPath, flags string) (string, error) {
	file, err := os.Open(apkPath)
	if err != nil {
		return "", fmt.Errorf("open apk %s: %w", apkPath, err)
	}
	defer file.Close()

	// Unique per-install temp path so parallel installs (different devices,
	// or a retry after a failure) never share a file.
	tmpPath := fmt.Sprintf("/data/local/tmp/updater-install-%d-%d.apk", os.Getpid(), tempSeq.Add(1))
	if err := dev.Push(file, tmpPath, 0644); err != nil {
		return "", fmt.Errorf("push apk: %w", err)
	}
	defer dev.RunCommandContext(ctx, "rm -f "+tmpPath)

	installCmd := "pm install " + tmpPath
	if f := strings.TrimSpace(flags); f != "" {
		installCmd = "pm install " + f + " " + tmpPath
	}
	out, err := dev.RunCommandContext(ctx, installCmd)
	if err != nil {
		return "", fmt.Errorf("pm install: %w", err)
	}
	if !strings.Contains(out, "Success") {
		return out, fmt.Errorf("pm install failed: %s", strings.TrimSpace(out))
	}
	return out, nil
}
