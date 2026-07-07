package process

import (
	"context"
	"testing"
	"time"
)

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
	err := ctrl.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() with nothing configured should return nil, got %v", err)
	}
}

func TestStop_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", true, 0)
	// Should attempt service stop (will fail in test env but not panic)
	err := ctrl.Stop(context.Background())
	if err == nil {
		t.Log("Stop() service mode succeeded (unexpected in test env)")
	}
}

func TestStop_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("nonexistent-process-xyz", "", "", false, 0)
	err := ctrl.Stop(context.Background())
	// pkill/taskkill will fail but shouldn't panic
	if err == nil {
		t.Log("Stop() by image name succeeded (unexpected in test env)")
	}
}

func TestStop_CustomCmdWithArgs(t *testing.T) {
	ctrl := NewWithConfig("test", "echo hello && echo world", "", false, 0)
	err := ctrl.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() with multi-cmd should succeed, got %v", err)
	}
}

func TestStart_OnlyService(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", true, 0)
	err := ctrl.Start(context.Background(), "")
	// Should attempt service start (will fail in test env)
	if err == nil {
		t.Log("Start() service mode succeeded (unexpected in test env)")
	}
}

func TestStart_OnlyImageName(t *testing.T) {
	ctrl := NewWithConfig("test", "", "", false, 0)
	err := ctrl.Start(context.Background(), "/nonexistent/binary")
	// Should attempt to start the binary (will fail in test env)
	if err == nil {
		t.Log("Start() by path succeeded (unexpected in test env)")
	}
}

func TestStart_EmptyImageAndCmd(t *testing.T) {
	ctrl := New("")
	err := ctrl.Start(context.Background(), "")
	if err != nil {
		t.Errorf("Start() with nothing configured should return nil, got %v", err)
	}
}

func TestWaitForStop_Immediate(t *testing.T) {
	ctrl := New("nonexistent-process-xyz")
	err := ctrl.WaitForStop(context.Background(), 1*time.Second)
	if err != nil {
		t.Errorf("WaitForStop() should return nil for non-running process, got %v", err)
	}
}

func TestWaitForStop_Timeout(t *testing.T) {
	ctrl := New("nonexistent-process-xyz")
	// Process is not running, so this should succeed quickly
	err := ctrl.WaitForStop(context.Background(), 500*time.Millisecond)
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
