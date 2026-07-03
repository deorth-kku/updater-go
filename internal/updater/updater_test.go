package updater

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/api"
	"github.com/deorth-kku/updater-go/internal/config"
)

// mockDownloader implements downloader.Downloader for testing.
type mockDownloader struct{}

func (m *mockDownloader) Download(_ context.Context, _ string, filename, saveDir string) (string, string, error) {
	localPath := filepath.Join(saveDir, filename)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(localPath, []byte("mock-download-content"), 0o644); err != nil {
		return "", "", err
	}
	return localPath, "mock-gid", nil
}

func (m *mockDownloader) Remove(_ string) error { return nil }
func (m *mockDownloader) Close() error          { return nil }

// mockHTTPDownloader implements api.Downloader for testing.
type mockHTTPDownloader struct{}

func (m *mockHTTPDownloader) Get(_ context.Context, url string) (*api.HTTPResponse, error) {
	release := []map[string]interface{}{
		{
			"tag_name": "v1.0.0",
			"name":     "Test Release",
			"assets":   []interface{}{},
		},
	}
	body, _ := json.Marshal(release)
	return &api.HTTPResponse{StatusCode: 200, Body: body}, nil
}

func TestUpdate_FullFlow(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			URL: "/test.zip",
		},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true, IsBool: true}},
	}

	saveDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, saveDir, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(context.Background())

	if result.Error != nil {
		t.Fatalf("Update() error = %v", result.Error)
	}
	if result.Downloaded == "" {
		t.Error("Downloaded is empty")
	}
}

func TestUpdate_NoUpdateNeeded(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		CurrentVersion: "v1.0.0",
	}

	saveDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, saveDir, false, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(context.Background())

	if result.Error != nil {
		t.Fatalf("Update() error = %v", result.Error)
	}
	if result.Downloaded != "" {
		t.Error("Downloaded should be empty for no-update case")
	}
}

func TestUpdate_ConfigWriteback(t *testing.T) {
	// Create a project config file
	saveDir := t.TempDir()
	configDir := filepath.Join(saveDir, "config")
	os.MkdirAll(configDir, 0o755)

	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "test-project",
		},
		CurrentVersion: "v1.0.0",
		Download: config.DownloadConfig{
			URL: "/test.zip",
		},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true, IsBool: true}},
	}

	// Write initial config
	cfgData, _ := json.MarshalIndent(projCfg, "", "  ")
	cfgPath := filepath.Join(configDir, "test-project.json")
	os.WriteFile(cfgPath, cfgData, 0o644)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	u := New(projCfg, saveDir, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(context.Background())

	if result.Error != nil {
		t.Fatalf("Update() error = %v", result.Error)
	}

	// Verify config was written back
	writtenData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var writtenCfg config.ProjectConfig
	if err := json.Unmarshal(writtenData, &writtenCfg); err != nil {
		t.Fatalf("Failed to parse written config: %v", err)
	}

	if writtenCfg.CurrentVersion != "v1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", writtenCfg.CurrentVersion, "v1.0.0")
	}
}

func TestUpdate_ProcessRestart(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "test-app",
		},
		CurrentVersion: "v0.0.0",
		Download: config.DownloadConfig{
			URL: "/test.zip",
		},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true, IsBool: true}},
		Process: config.ProcessConfig{
			ImageName:    "test-app",
			AllowRestart: true,
			RestartWait:  0,
		},
	}

	saveDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, saveDir, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(context.Background())

	if result.Error != nil {
		t.Fatalf("Update() error = %v", result.Error)
	}
}

func TestUpdate_CustomStopStartCmd(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "test-app",
		},
		CurrentVersion: "v0.0.0",
		Download: config.DownloadConfig{
			URL: "/test.zip",
		},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true, IsBool: true}},
		Process: config.ProcessConfig{
			ImageName:    "test-app",
			AllowRestart: true,
			StopCmd:      "true",
			StartCmd:     "true",
			RestartWait:  0,
		},
	}

	saveDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, saveDir, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(context.Background())

	if result.Error != nil {
		t.Fatalf("Update() error = %v", result.Error)
	}
}

func TestReplaceVars(t *testing.T) {
	tests := []struct {
		input    string
		path     string
		name     string
		dlFile   string
		version  string
		expected string
	}{
		{
			input:    "%PATH/%NAME",
			path:     "/opt/apps",
			name:     "myapp",
			expected: "/opt/apps/myapp",
		},
		{
			input:    "%DL_FILENAME",
			dlFile:   "app-v1.0.zip",
			expected: "app-v1.0.zip",
		},
		{
			input:    "%VER",
			version:  "2.0.0",
			expected: "2.0.0",
		},
		{
			input:    "%PATH/%NAME/%DL_FILENAME@%VER",
			path:     "/opt",
			name:     "app",
			dlFile:   "app.zip",
			version:  "1.0",
			expected: "/opt/app/app.zip@1.0",
		},
	}

	for _, tt := range tests {
		result := replaceVars(tt.input, tt.path, tt.name, tt.dlFile, tt.version)
		if result != tt.expected {
			t.Errorf("replaceVars(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
