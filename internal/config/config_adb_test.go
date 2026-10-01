package config

import (
	"encoding/json"
	"testing"
)

func TestAdbConfig_Addr(t *testing.T) {
	c := AdbConfig{IP: "127.0.0.1", Port: "5037"}
	if got := c.Addr(); got != "127.0.0.1:5037" {
		t.Errorf("Addr() = %q, want %q", got, "127.0.0.1:5037")
	}
}

func TestAdbConfig_IsLocal(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"", true},
		{"127.0.0.1", true},
		{"localhost", true},
		{"127.1", true},
		{"192.168.1.10", false},
		{"10.0.0.5", false},
	}
	for _, tc := range cases {
		c := AdbConfig{IP: tc.ip}
		if got := c.IsLocal(); got != tc.want {
			t.Errorf("IsLocal(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestAdbDefaults(t *testing.T) {
	cfg := GetDefault()
	if cfg.Adb.IP != "127.0.0.1" {
		t.Errorf("Adb.IP default = %q, want 127.0.0.1", cfg.Adb.IP)
	}
	if cfg.Adb.Port != "5037" {
		t.Errorf("Adb.Port default = %q, want 5037", cfg.Adb.Port)
	}
	if cfg.Adb.Bin != "adb" {
		t.Errorf("Adb.Bin default = %q, want adb", cfg.Adb.Bin)
	}
}

func TestProjectConfig_AdbDefaultFlags(t *testing.T) {
	pc, err := GetProjectConfig()
	if err != nil {
		t.Fatalf("GetProjectConfig() error = %v", err)
	}
	if pc.Adb.InstallFlags != "-r -d" {
		t.Errorf("Adb.InstallFlags default = %q, want %q", pc.Adb.InstallFlags, "-r -d")
	}
	if pc.Adb.Enabled {
		t.Error("Adb.Enabled default should be false")
	}
}

func TestProjectEntry_DeviceJSON(t *testing.T) {
	data := []byte(`{"name":"testproj","device":"testdevice01"}`)
	var e ProjectEntry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Device != "testdevice01" {
		t.Errorf("Device = %q, want testdevice01", e.Device)
	}
}
