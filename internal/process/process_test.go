package process

import (
	"log/slog"
	"testing"
	"time"
)

func TestNewWithConfig(t *testing.T) {
	ctrl := NewWithConfig("test-app", "/tmp", "stop-cmd", "start-cmd", true, 5, slog.Default())
	if ctrl.imageName != "test-app" {
		t.Errorf("imageName = %q, want %q", ctrl.imageName, "test-app")
	}
	if ctrl.savePath != "/tmp" {
		t.Errorf("savePath = %q, want %q", ctrl.savePath, "/tmp")
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
	ctrl := New("", slog.Default())
	_, err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestStop_CustomStopCmd(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "echo stop", "", false, 0, slog.Default())
	// This will run "echo stop" which should succeed
	_, err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with custom cmd error = %v, want nil", err)
	}
}

func TestStop_CustomStopCmdFailure(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "false", "", false, 0, slog.Default())
	// "false" command always fails
	_, err := ctrl.Stop(t.Context())
	if err == nil {
		t.Error("Stop() with failing cmd should return error")
	}
}

func TestStop_ServiceMode(t *testing.T) {
	ctrl := NewWithConfig("test-service", "/tmp", "", "", true, 0, slog.Default())
	// systemctl/sc will fail in test environment, but we just verify it doesn't panic
	_, err := ctrl.Stop(t.Context())
	// We expect an error since systemctl/sc isn't available in test env
	if err == nil {
		t.Log("Stop() service mode succeeded (unexpected in test env)")
	}
}

func TestStart_EmptyImageName(t *testing.T) {
	ctrl := New("", slog.Default())
	err := ctrl.Start(t.Context())
	if err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
}

func TestStart_EmptyPath(t *testing.T) {
	ctrl := NewWithConfig("test-app", "/tmp", "", "", false, 0, slog.Default())
	err := ctrl.Start(t.Context())
	// Should attempt to launch /tmp/test-app (which doesn't exist in test env)
	if err == nil {
		t.Log("Start() succeeded (unexpected in test env)")
	}
}

func TestStart_CustomStartCmd(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "", "echo start", false, 0, slog.Default())
	err := ctrl.Start(t.Context())
	if err != nil {
		t.Errorf("Start() with custom cmd error = %v, want nil", err)
	}
}

func TestStart_ServiceMode(t *testing.T) {
	ctrl := NewWithConfig("test-service", "/tmp", "", "", true, 0, slog.Default())
	err := ctrl.Start(t.Context())
	// sc/systemctl will fail in test environment
	if err == nil {
		t.Log("Start() service mode succeeded (unexpected in test env)")
	}
}

func TestRestartWait(t *testing.T) {
	// Test that restart_wait actually waits
	ctrl := NewWithConfig("test", "/tmp", "true", "", false, 1, slog.Default())
	start := time.Now()
	_, err := ctrl.Stop(t.Context())
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
	ctrl := New("nonexistent-process-12345", slog.Default())
	if ctrl.IsRunning() {
		t.Error("IsRunning() should return false for non-existent process")
	}
}
func TestNew_Empty(t *testing.T) {
	ctrl := New("", slog.Default())
	if ctrl.imageName != "" {
		t.Errorf("imageName = %q, want empty", ctrl.imageName)
	}
	if ctrl.savePath != "" {
		t.Errorf("savePath = %q, want empty", ctrl.savePath)
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
	ctrl := New("", slog.Default())
	_, err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with nothing configured should return nil, got %v", err)
	}
}

func TestStop_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "", "", true, 0, slog.Default())
	// Should attempt service stop (will fail in test env but not panic)
	_, err := ctrl.Stop(t.Context())
	if err == nil {
		t.Log("Stop() service mode succeeded (unexpected in test env)")
	}
}

func TestStop_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("nonexistent-process-xyz", "/tmp", "", "", false, 0, slog.Default())
	_, err := ctrl.Stop(t.Context())
	// pkill/taskkill will fail but shouldn't panic
	if err == nil {
		t.Log("Stop() by image name succeeded (unexpected in test env)")
	}
}

func TestStop_CustomCmdWithArgs(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "echo hello && echo world", "", false, 0, slog.Default())
	_, err := ctrl.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with multi-cmd should succeed, got %v", err)
	}
}

func TestStart_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "", "", true, 0, slog.Default())
	err := ctrl.Start(t.Context())
	// Should attempt service start (will fail in test env)
	if err == nil {
		t.Log("Start() service mode succeeded (unexpected in test env)")
	}
}

func TestStart_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("test", "/tmp", "", "", false, 0, slog.Default())
	err := ctrl.Start(t.Context())
	// Should attempt to start the binary (will fail in test env)
	if err == nil {
		t.Log("Start() by path succeeded (unexpected in test env)")
	}
}

func TestStart_EmptyImageAndCmd(t *testing.T) {
	ctrl := New("", slog.Default())
	err := ctrl.Start(t.Context())
	if err != nil {
		t.Errorf("Start() with nothing configured should return nil, got %v", err)
	}
}

func TestWaitForStop_Immediate(t *testing.T) {
	ctrl := New("nonexistent-process-xyz", slog.Default())
	err := ctrl.WaitForStop(t.Context(), 1*time.Second)
	if err != nil {
		t.Errorf("WaitForStop() should return nil for non-running process, got %v", err)
	}
}

func TestWaitForStop_Timeout(t *testing.T) {
	ctrl := New("nonexistent-process-xyz", slog.Default())
	// Process is not running, so this should succeed quickly
	err := ctrl.WaitForStop(t.Context(), 500*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForStop() should succeed for non-running process, got %v", err)
	}
}

func TestIsRunning_NonExistent(t *testing.T) {
	ctrl := New("nonexistent-process-xyz-12345", slog.Default())
	if ctrl.IsRunning() {
		t.Error("IsRunning() should return false for non-existent process")
	}
}

func TestNewWithConfig_ZeroValues(t *testing.T) {
	ctrl := NewWithConfig("", "/tmp", "", "", false, 0, slog.Default())
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
