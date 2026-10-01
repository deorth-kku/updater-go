package process

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// TestStopService_WaitsForUnitToStop verifies that stopService blocks until the
// systemd unit has actually stopped, not merely until the stop job is
// enqueued. It starts a user service whose ExecStop sleeps 2s, so a correct
// (blocking) stop takes ~2s while a non-blocking stop would return almost
// immediately.
//
// The test uses the go-systemd D-Bus API directly (no shelling out to
// systemctl): getConn to connect, StartUnitContext (with a result channel) to
// start and wait, SetSubStateSubscriber (PropertiesChanged signal) to wait for
// the running state, and GetUnitPropertyContext to read the final state.
func TestStopService_WaitsForUnitToStop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("stopService systemd implementation is linux-only")
	}
	if isRoot {
		t.Skip("running as root; stopService would use the system bus, not the user bus")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := getConn(ctx)
	if err != nil {
		t.Skipf("connect to systemd user bus: %v", err)
	}
	// Cleanups run in LIFO order, so register the connection close first: it
	// runs last, after the unit has been stopped and the file removed.
	t.Cleanup(func() { conn.Close() })

	// Install the unit into the user's systemd unit directory.
	unit := fmt.Sprintf("updater-gotest-%d.service", os.Getpid())
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("resolve user config dir: %v", err)
	}
	unitDir := filepath.Join(cfgDir, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("create unit dir: %v", err)
	}
	unitPath := filepath.Join(unitDir, unit)

	// ExecStop runs a 2s sleep, so stopping the unit deterministically takes
	// ~2s. This is robust: it does not depend on the main process trapping
	// SIGTERM (a shell trap can be missed if the unit is stopped before the
	// trap is armed).
	const unitFile = `[Unit]
Description=updater-go stopService test

[Service]
Type=simple
ExecStart=/bin/sleep 30
ExecStop=/bin/sleep 2
TimeoutStopSec=30
`
	if err := os.WriteFile(unitPath, []byte(unitFile), 0o644); err != nil {
		t.Fatalf("write unit file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(unitPath) })
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		stopCh := make(chan string, 1)
		if _, err := conn.StopUnitContext(stopCtx, unit, "replace", stopCh); err == nil {
			select {
			case <-stopCtx.Done():
			case <-stopCh:
			}
		}
	})

	// Start the unit and wait for the start job to complete.
	startCh := make(chan string, 1)
	if _, err := conn.StartUnitContext(ctx, unit, "replace", startCh); err != nil {
		t.Fatalf("start unit: %v", err)
	}
	select {
	case <-ctx.Done():
		t.Fatalf("start unit: %v", ctx.Err())
	case result := <-startCh:
		if result != "done" {
			t.Fatalf("start unit: job result %q", result)
		}
	}

	// Wait until the unit is actually running before testing the stop, so we
	// don't stop it during the start->running transition.
	if err := waitUnitSubState(ctx, conn, unit, "running"); err != nil {
		t.Fatalf("wait for unit running: %v", err)
	}

	ctrl := NewWithConfig(strings.TrimSuffix(unit, ".service"), "", "", "", true, 0, slog.Default())

	start := time.Now()
	if err := ctrl.stopService(ctx); err != nil {
		t.Fatalf("stopService: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 1500*time.Millisecond {
		t.Errorf("stopService returned after %v; expected it to block until the service actually stopped (the test service sleeps ~2s on SIGTERM)", elapsed)
	}

	// Confirm the unit is no longer active.
	prop, err := conn.GetUnitPropertyContext(ctx, unit, "ActiveState")
	if err != nil {
		t.Fatalf("get ActiveState: %v", err)
	}
	if state, _ := prop.Value.Value().(string); state != "inactive" {
		t.Errorf("unit state after stop = %q, want inactive", state)
	}
}

// waitUnitSubState blocks until the unit reports the given SubState. It is
// event-driven: it subscribes to systemd's PropertiesChanged signal via
// SetSubStateSubscriber (no polling). The current state is checked first to
// handle the case where the unit is already in the desired state.
func waitUnitSubState(ctx context.Context, conn *dbus.Conn, unit, want string) error {
	if err := conn.Subscribe(); err != nil {
		return err
	}
	updateCh := make(chan *dbus.SubStateUpdate, 256)
	errCh := make(chan error, 1)
	conn.SetSubStateSubscriber(updateCh, errCh)

	if state, err := unitSubState(ctx, conn, unit); err == nil && state == want {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case u := <-updateCh:
			if u.UnitName == unit && u.SubState == want {
				return nil
			}
		}
	}
}

// unitSubState returns the unit's SubState property as a string.
func unitSubState(ctx context.Context, conn *dbus.Conn, unit string) (string, error) {
	prop, err := conn.GetUnitPropertyContext(ctx, unit, "SubState")
	if err != nil {
		return "", err
	}
	state, _ := prop.Value.Value().(string)
	return state, nil
}
