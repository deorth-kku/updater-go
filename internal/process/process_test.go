package process

import (
	"testing"
	"time"
)

func TestNewWithConfig(t *testing.T) {
	ctrl := NewWithConfig("test-app", "stop-cmd", "start-cmd", true, 5)
	if ctrl.imageName != "test-app" {
		t.Errorf("imageName = %q, want %q", ctrl.imageName, "test-app")
	}
	if ctrl.stopCmd != "stop-cmd" {
		t.Errorf("stopCmd = %q, want %q", ctrl.stopCmd, "stop-cmd")
	}
	if ctrl.startCmd != "start-cmd" {
		t.Errorf("startCmd = %q, want %q", ctrl.startCmd, "start-cmd")
	}
	if !ctrl.service {
		t.Error("service should be true")
	}
	if ctrl.restartWait != 5 {
		t.Errorf("restartWait = %d, want 5", ctrl.restartWait)
	}
}

func TestStop_EmptyImageName(t *testing.T) {
	ctrl := New("")
	err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestStop_CustomStopCmd(t *testing.T) {
	ctrl := NewWithConfig("test", "echo stop", "", false, 0)
	// This will run "echo stop" which should succeed
	err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with custom cmd error = %v, want nil", err)
	}
}

func TestStop_CustomStopCmdFailure(t *testing.T) {
	ctrl := NewWithConfig("test", "false", "", false, 0)
	// "false" command always fails
	err := ctrl.Stop(t.Context())
	if err == nil {
		t.Error("Stop() with failing cmd should return error")
	}
}

func TestStop_ServiceMode(t *testing.T) {
	ctrl := NewWithConfig("test-service", "", "", true, 0)
	// systemctl/sc will fail in test environment, but we just verify it doesn't panic
	err := ctrl.Stop(t.Context())
	// We expect an error since systemctl/sc isn't available in test env
	if err == nil {
		t.Log("Stop() service mode succeeded (unexpected in test env)")
	}
}

func TestStart_EmptyImageName(t *testing.T) {
	ctrl := New("")
	err := ctrl.Start(t.Context(), "/path/to/binary")
	if err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
}

func TestStart_EmptyPath(t *testing.T) {
	ctrl := New("test-app")
	err := ctrl.Start(t.Context(), "")
	if err == nil {
		t.Error("Start() with empty path should return error")
	}
}

func TestStart_CustomStartCmd(t *testing.T) {
	ctrl := NewWithConfig("test", "", "echo start", false, 0)
	err := ctrl.Start(t.Context(), "")
	if err != nil {
		t.Errorf("Start() with custom cmd error = %v, want nil", err)
	}
}

func TestStart_ServiceMode(t *testing.T) {
	ctrl := NewWithConfig("test-service", "", "", true, 0)
	err := ctrl.Start(t.Context(), "")
	// sc/systemctl will fail in test environment
	if err == nil {
		t.Log("Start() service mode succeeded (unexpected in test env)")
	}
}

func TestRestartWait(t *testing.T) {
	// Test that restart_wait actually waits
	ctrl := NewWithConfig("test", "true", "", false, 1)
	start := time.Now()
	err := ctrl.Stop(t.Context())
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
	// Should have waited at least 900ms (1 second with some tolerance)
	if elapsed < 900*time.Millisecond {
		t.Errorf("Stop() took %v, expected at least 900ms for restart_wait=1", elapsed)
	}
}

func TestIsRunning(t *testing.T) {
	// Test with a non-existent process
	ctrl := New("nonexistent-process-12345")
	if ctrl.IsRunning() {
		t.Error("IsRunning() should return false for non-existent process")
	}
}
