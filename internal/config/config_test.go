package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// updaterConfigDir is the path to the real updater-config repo.
var updaterConfigDir = filepath.Join("..", "..", "..", "updater-config")

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
        "aria2": {
            "ip": "127.0.0.1",
            "rpc-listen-port": "6800"
        },
        "projects": [
            {"name": "git", "path": "/opt/git", "enabled": true}
        ]
    }`
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Aria2.IP != "127.0.0.1" {
		t.Errorf("Aria2.IP = %q, want %q", got.Aria2.IP, "127.0.0.1")
	}
	if len(got.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(got.Projects))
	}
	if got.Projects[0].Name != "git" {
		t.Errorf("Projects[0].Name = %q, want %q", got.Projects[0].Name, "git")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"projects": []}`
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Aria2.IP != "127.0.0.1" {
		t.Errorf("Aria2.IP default = %q, want %q", got.Aria2.IP, "127.0.0.1")
	}
	if got.Aria2.RPCListenPort != "6800" {
		t.Errorf("Aria2.RPCListenPort default = %q, want %q", got.Aria2.RPCListenPort, "6800")
	}
}

func TestLoad_LegacyMigration(t *testing.T) {
	dir := t.TempDir()
	cfg := `{
        "aria2": {"ip": "127.0.0.1", "rpc-listen-port": "6800"},
        "projects": {
            "git": {"path": "/opt/git", "hold": false},
            "7zip": {"path": "/opt/7z", "hold": true}
        }
    }`
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(got.Projects))
	}
	names := map[string]bool{}
	for _, p := range got.Projects {
		names[p.Name] = true
	}
	if !names["git"] || !names["7zip"] {
		t.Errorf("Projects names = %v, want git and 7zip", names)
	}
	for _, p := range got.Projects {
		if p.Name == "7zip" && p.Enabled() {
			t.Error("7zip should be disabled")
		}
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Error("Load() expected error for missing file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for invalid JSON")
	}
}

func TestAria2_RPCAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  Aria2Config
		want string
	}{
		{name: "local http", cfg: Aria2Config{IP: "127.0.0.1", RPCListenPort: "6800"}, want: "http://127.0.0.1:6800/jsonrpc"},
		{name: "local localhost", cfg: Aria2Config{IP: "localhost", RPCListenPort: "6800"}, want: "http://localhost:6800/jsonrpc"},
		{name: "remote ws", cfg: Aria2Config{IP: "192.168.1.100", RPCListenPort: "6800"}, want: "ws://192.168.1.100:6800/jsonrpc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.RPCAddr(); got != tt.want {
				t.Errorf("RPCAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAria2_IsRemote(t *testing.T) {
	tests := []struct {
		cfg  Aria2Config
		want bool
	}{
		{Aria2Config{IP: "127.0.0.1"}, false},
		{Aria2Config{IP: "localhost"}, false},
		{Aria2Config{IP: "192.168.1.100"}, true},
	}
	for _, tt := range tests {
		if got := tt.cfg.IsRemote(); got != tt.want {
			t.Errorf("IsRemote() = %v, want %v", got, tt.want)
		}
	}
}

func TestProjectConfigPath(t *testing.T) {
	got := ProjectConfigPath("/etc/updater", "git")
	want := "/etc/updater/config/git.json"
	if got != want {
		t.Errorf("ProjectConfigPath() = %q, want %q", got, want)
	}
}

// collectJSONFiles walks all subdirectories under root and returns .json file paths.
// Skips metadata.json which is an index file, not a project config.
func collectJSONFiles(root string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			subFiles, err := collectJSONFiles(filepath.Join(root, entry.Name()))
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		} else if filepath.Ext(entry.Name()) == ".json" && entry.Name() != "metadata.json" {
			files = append(files, filepath.Join(root, entry.Name()))
		}
	}
	return files, nil
}

// TestLoad_RealConfig_Unmarshal loads real project configs from all subdirectories of updater-config/.
func TestLoad_RealConfig_Unmarshal(t *testing.T) {
	subdirs, err := os.ReadDir(updaterConfigDir)
	if err != nil {
		t.Fatalf("read updater-config dir: %v", err)
	}

	for _, sub := range subdirs {
		if !sub.IsDir() {
			continue
		}
		subPath := filepath.Join(updaterConfigDir, sub.Name())
		entries, err := os.ReadDir(subPath)
		if err != nil {
			t.Fatalf("read %s: %v", sub.Name(), err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			t.Run(filepath.Join(sub.Name(), entry.Name()), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(subPath, entry.Name()))
				if err != nil {
					t.Fatalf("read %s: %v", entry.Name(), err)
				}

				var pc ProjectConfig
				if err := json.Unmarshal(data, &pc); err != nil {
					t.Fatalf("unmarshal %s: %v", entry.Name(), err)
				}
			})
		}
	}
}

// TestLoad_RealConfig_ApplyDefaults applies defaults to real project configs from all subdirectories.
func TestLoad_RealConfig_ApplyDefaults(t *testing.T) {
	subdirs, err := os.ReadDir(updaterConfigDir)
	if err != nil {
		t.Fatalf("read updater-config dir: %v", err)
	}

	for _, sub := range subdirs {
		if !sub.IsDir() {
			continue
		}
		subPath := filepath.Join(updaterConfigDir, sub.Name())
		entries, err := os.ReadDir(subPath)
		if err != nil {
			t.Fatalf("read %s: %v", sub.Name(), err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			t.Run(filepath.Join(sub.Name(), entry.Name()), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(subPath, entry.Name()))
				if err != nil {
					t.Fatalf("read %s: %v", entry.Name(), err)
				}

				var pc ProjectConfig
				if err := json.Unmarshal(data, &pc); err != nil {
					t.Fatalf("unmarshal %s: %v", entry.Name(), err)
				}

				if err := ApplyDefaults(&pc, data, nil); err != nil {
					t.Fatalf("ApplyDefaults %s: %v", entry.Name(), err)
				}

				if pc.Basic.APIType == "" {
					t.Error("APIType is empty after ApplyDefaults")
				}
			})
		}
	}
}
