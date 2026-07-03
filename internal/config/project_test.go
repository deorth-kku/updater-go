package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// gamingConfigDir is the path to the gaming configs in updater-config.
var gamingConfigDir = filepath.Join("..", "..", "..", "updater-config", "gaming")

func TestStringOrSlice_String(t *testing.T) {
	var s StringOrSlice
	if err := json.Unmarshal([]byte(`"rpcs3"`), &s); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if len(s) != 1 || s[0] != "rpcs3" {
		t.Errorf("StringOrSlice = %v, want [rpcs3]", s)
	}
}

func TestStringOrSlice_Slice(t *testing.T) {
	var s StringOrSlice
	if err := json.Unmarshal([]byte(`["%arch", "amd64"]`), &s); err != nil {
		t.Fatalf("unmarshal slice: %v", err)
	}
	if len(s) != 2 || s[0] != "%arch" || s[1] != "amd64" {
		t.Errorf("StringOrSlice = %v, want [%q, %q]", s, "%arch", "amd64")
	}
}

func TestStringOrSlice_EmptyString(t *testing.T) {
	var s StringOrSlice
	if err := json.Unmarshal([]byte(`""`), &s); err != nil {
		t.Fatalf("unmarshal empty string: %v", err)
	}
	if len(s) != 1 || s[0] != "" {
		t.Errorf("StringOrSlice = %v, want [\"\"]", s)
	}
}

func TestStringOrSlice_EmptyArray(t *testing.T) {
	var s StringOrSlice
	if err := json.Unmarshal([]byte(`[]`), &s); err != nil {
		t.Fatalf("unmarshal empty array: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("StringOrSlice = %v, want []", s)
	}
}

func TestStringOrSlice_Invalid(t *testing.T) {
	var s StringOrSlice
	err := json.Unmarshal([]byte(`123`), &s)
	if err == nil {
		t.Error("unmarshal number: expected error")
	}
}

// TestProjectConfig_Unmarshal_RealFiles loads every JSON from updater-config/gaming/
// and verifies it unmarshals into ProjectConfig without error.
func TestProjectConfig_Unmarshal_RealFiles(t *testing.T) {
	entries, err := os.ReadDir(gamingConfigDir)
	if err != nil {
		t.Fatalf("read updater-config/gaming: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(gamingConfigDir, entry.Name()))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			var pc ProjectConfig
			if err := json.Unmarshal(data, &pc); err != nil {
				t.Fatalf("unmarshal %s: %v", entry.Name(), err)
			}
			// Verify basic fields are populated
			if pc.Basic.APIType == "" {
				t.Error("APIType is empty")
			}
		})
	}
}

// TestProjectConfig_Unmarshal_AllSubdirs loads JSON from all subdirectories.
func TestProjectConfig_Unmarshal_AllSubdirs(t *testing.T) {
	baseDir := filepath.Join("..", "..", "..", "updater-config")
	subdirs := []string{"gaming", "android", "dev", "multimedia", "system-tools", "utils", "windows-tools"}

	for _, sub := range subdirs {
		dir := filepath.Join(baseDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // skip missing dirs
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			t.Run(sub+"/"+entry.Name(), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatalf("read file: %v", err)
				}
				var pc ProjectConfig
				if err := json.Unmarshal(data, &pc); err != nil {
					t.Fatalf("unmarshal %s: %v", entry.Name(), err)
				}
			})
		}
	}
}

// TestProjectConfig_ApplyDefaults verifies that ApplyDefaults fills in missing fields
// using hardcoded defaults and Config.Defaults.
func TestProjectConfig_ApplyDefaults(t *testing.T) {
	// Minimal config — only basic.api_type set
	data := []byte(`{"basic": {"api_type": "github", "account_name": "test", "project_name": "test"}}`)
	var pc ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := ApplyDefaults(&pc, data, nil); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}

	// TryRedirect should default to true
	if !pc.Download.TryRedirect {
		t.Error("TryRedirect should default to true")
	}
	// Decompress.Skip should default to false
	if pc.Decompress.Skip.Bool() {
		t.Error("Decompress.Skip should default to false")
	}
	// Process.AllowRestart should default to false
	if pc.Process.AllowRestart {
		t.Error("AllowRestart should default to false")
	}
	// Process.RestartWait should default to 3
	if pc.Process.RestartWait != 3 {
		t.Errorf("RestartWait = %d, want 3", pc.Process.RestartWait)
	}
}

// TestProjectConfig_ApplyDefaults_WithDefaults verifies that Config.Defaults overlay works.
func TestProjectConfig_ApplyDefaults_WithDefaults(t *testing.T) {
	data := []byte(`{"basic": {"api_type": "github", "account_name": "test", "project_name": "test"}}`)
	var pc ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	defaults := json.RawMessage(`{"download": {"try_redirect": false}, "process": {"restart_wait": 5}}`)
	if err := ApplyDefaults(&pc, data, defaults); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}

	if pc.Download.TryRedirect {
		t.Error("TryRedirect should be false (overridden by defaults)")
	}
	if pc.Process.RestartWait != 5 {
		t.Errorf("RestartWait = %d, want 5 (overridden by defaults)", pc.Process.RestartWait)
	}
}
