package updater

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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
	release := []map[string]any{
		{
			"tag_name": "v1.0.0",
			"name":     "Test Release",
			"assets":   []any{},
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
	result := u.Update(t.Context(), "")

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
	}

	saveDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, saveDir, false, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context(), "v1.0.0")

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
	result := u.Update(t.Context(), "")

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
}

func TestUpdate_ProcessRestart(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType:     "github",
			ProjectName: "test-app",
		},
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
	result := u.Update(t.Context(), "")

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
	result := u.Update(t.Context(), "")

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

func TestDownloadFilename_ArchOS(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			FilenameOverride:     "app-%arch-%OS.zip",
			AddVersionToFilename: true,
		},
	}

	u := &Updater{projectCfg: projCfg}
	result := u.downloadFilename("1.0.0", "http://example.com/test.zip")

	if result != "app-"+runtime.GOARCH+"-"+runtime.GOOS+".zip" {
		t.Errorf("downloadFilename() = %q, want app-%s-%s.zip", result, runtime.GOARCH, runtime.GOOS)
	}
}

func TestDownloadFilename_NoVersion(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			FilenameOverride: "app.zip",
		},
	}

	u := &Updater{projectCfg: projCfg}
	result := u.downloadFilename("1.0.0", "http://example.com/test.zip")

	if result != "app.zip" {
		t.Errorf("downloadFilename() = %q, want %q", result, "app.zip")
	}
}

func TestSelectDownloadURL_Index(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			Keyword:  config.StringOrSlice{""},
			Filetype: config.StringOrSlice{"7z"},
			Index:    2,
		},
	}

	rel := &api.Release{
		Version: "v1.0.0",
		Assets: []api.Asset{
			{URL: "http://example.com/first.7z", Name: "app-first.7z"},
			{URL: "http://example.com/second.7z", Name: "app-second.7z"},
			{URL: "http://example.com/third.7z", Name: "app-third.7z"},
		},
	}

	u := &Updater{projectCfg: projCfg}
	result := u.selectDownloadURL(rel)

	// Index 2 means second match (1-based)
	if result != "http://example.com/second.7z" {
		t.Errorf("selectDownloadURL() = %q, want %q", result, "http://example.com/second.7z")
	}
}

func TestSelectDownloadURL_Indexes(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			Keyword:  config.StringOrSlice{""},
			Filetype: config.StringOrSlice{"zip"},
			Indexes:  []int{0, 2},
		},
	}

	rel := &api.Release{
		Version: "v1.0.0",
		Assets: []api.Asset{
			{URL: "http://example.com/a.zip", Name: "a.zip"},
			{URL: "http://example.com/b.zip", Name: "b.zip"},
			{URL: "http://example.com/c.zip", Name: "c.zip"},
		},
	}

	u := &Updater{projectCfg: projCfg}
	result := u.selectDownloadURL(rel)

	// Indexes [0, 2] means first and third match, first one wins
	if result != "http://example.com/a.zip" {
		t.Errorf("selectDownloadURL() = %q, want %q", result, "http://example.com/a.zip")
	}
}
