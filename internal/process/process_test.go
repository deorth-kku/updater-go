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
func TestNew_Empty(t *testing.T) {
	ctrl := New("")
	if ctrl.imageName != "" {
		t.Errorf("imageName = %q, want empty", ctrl.imageName)
	}
	if ctrl.stopCmd != "" {
		t.Errorf("stopCmd = %q, want empty", ctrl.stopCmd)
	}
	if ctrl.startCmd != "" {
		t.Errorf("startCmd = %q, want empty", ctrl.startCmd)
	}
	if ctrl.service {
		t.Error("service should be false")
	}
	if ctrl.restartWait != 0 {
		t.Errorf("restartWait = %d, want 0", ctrl.restartWait)
	}
}

func TestStop_NothingConfigured(t *testing.T) {
	ctrl := New("")
	err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with nothing configured should return nil, got %v", err)
	}
}

func TestStop_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", true, 0)
	// Should attempt service stop (will fail in test env but not panic)
	err := ctrl.Stop(t.Context())
	if err == nil {
		t.Log("Stop() service mode succeeded (unexpected in test env)")
	}
}

func TestStop_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("nonexistent-process-xyz", "", "", false, 0)
	err := ctrl.Stop(t.Context())
	// pkill/taskkill will fail but shouldn't panic
	if err == nil {
		t.Log("Stop() by image name succeeded (unexpected in test env)")
	}
}

func TestStop_CustomCmdWithArgs(t *testing.T) {
	ctrl := NewWithConfig("test", "echo hello && echo world", "", false, 0)
	err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with multi-cmd should succeed, got %v", err)
	}
}

func TestStart_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", true, 0)
	err := ctrl.Start(t.Context(), "")
	// Should attempt service start (will fail in test env)
	if err == nil {
		t.Log("Start() service mode succeeded (unexpected in test env)")
	}
}

func TestStart_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", false, 0)
	err := ctrl.Start(t.Context(), "/nonexistent/binary")
	// Should attempt to start the binary (will fail in test env)
	if err == nil {
		t.Log("Start() by path succeeded (unexpected in test env)")
	}
}

func TestStart_EmptyImageAndCmd(t *testing.T) {
	ctrl := New("")
	err := ctrl.Start(t.Context(), "")
	if err != nil {
		t.Errorf("Start() with nothing configured should return nil, got %v", err)
	}
}

func TestWaitForStop_Immediate(t *testing.T) {
	ctrl := New("nonexistent-process-xyz")
	err := ctrl.WaitForStop(t.Context(), 1*time.Second)
	if err != nil {
		t.Errorf("WaitForStop() should return nil for non-running process, got %v", err)
	}
}

func TestWaitForStop_Timeout(t *testing.T) {
	ctrl := New("nonexistent-process-xyz")
	// Process is not running, so this should succeed quickly
	err := ctrl.WaitForStop(t.Context(), 500*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForStop() should succeed for non-running process, got %v", err)
	}
}

func TestIsRunning_NonExistent(t *testing.T) {
	ctrl := New("nonexistent-process-xyz-12345")
	if ctrl.IsRunning() {
		t.Error("IsRunning() should return false for non-existent process")
	}
}

func TestNewWithConfig_ZeroValues(t *testing.T) {
	ctrl := NewWithConfig("", "", "", false, 0)
	if ctrl.imageName != "" {
		t.Errorf("imageName = %q, want empty", ctrl.imageName)
	}
	if ctrl.stopCmd != "" {
		t.Errorf("stopCmd = %q, want empty", ctrl.stopCmd)
	}
	if ctrl.startCmd != "" {
		t.Errorf("startCmd = %q, want empty", ctrl.startCmd)
	}
	if ctrl.service {
		t.Error("service should be false")
	}
	if ctrl.restartWait != 0 {
		t.Errorf("restartWait = %d, want 0", ctrl.restartWait)
	}
}
