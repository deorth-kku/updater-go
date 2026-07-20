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

func TestApplyIndex_ZeroBasedSingle(t *testing.T) {
	cases := []struct {
		name    string
		matched []string
		index   int
		indexes []int
		want    []string
	}{
		{"default no index", []string{"a", "b", "c"}, 0, nil, []string{"a", "b", "c"}},
		{"index 1 (python match_urls[1])", []string{"a", "b", "c"}, 1, nil, []string{"b"}},
		{"index 2", []string{"a", "b", "c"}, 2, nil, []string{"c"}},
		{"index out of range", []string{"a", "b"}, 5, nil, []string{"a", "b"}},
		{"indexes plural", []string{"a", "b", "c"}, 0, []int{0, 2}, []string{"a", "c"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := &Updater{projectCfg: config.ProjectConfig{
				Download: config.DownloadConfig{Index: c.index, Indexes: c.indexes},
			}}
			got := u.applyIndex(c.matched)
			if len(got) != len(c.want) {
				t.Fatalf("applyIndex() len = %d, want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("applyIndex()[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestUpdate_FullFlow(t *testing.T) {
	projCfg := config.ProjectConfig{
		Basic: config.BasicConfig{
			APIType: "github",
		},
		Download: config.DownloadConfig{
			URL: "/test.zip",
		},
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, config.ProjectEntry{SavePath: t.TempDir()}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context())

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

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, config.ProjectEntry{SavePath: t.TempDir(), Version: "v1.0.0"}, false, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context())

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
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
	}

	// Write initial config
	cfgData, _ := json.MarshalIndent(projCfg, "", "  ")
	cfgPath := filepath.Join(configDir, "test-project.json")
	os.WriteFile(cfgPath, cfgData, 0o644)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	u := New(projCfg, config.ProjectEntry{SavePath: saveDir}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context())

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
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
		Process: config.ProcessConfig{
			ImageName:    "test-app",
			AllowRestart: true,
			RestartWait:  0,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, config.ProjectEntry{SavePath: t.TempDir()}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context())

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
		Decompress: config.DecompressConfig{Skip: config.BoolOrString{BoolVal: true}},
		Process: config.ProcessConfig{
			ImageName:    "test-app",
			AllowRestart: true,
			StopCmd:      "true",
			StartCmd:     "true",
			RestartWait:  0,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	u := New(projCfg, config.ProjectEntry{SavePath: t.TempDir()}, true, &mockDownloader{}, &mockHTTPDownloader{}, logger)
	result := u.Update(t.Context())

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

	// Index 2 is 0-based per updater-rpc: match_urls[2] -> third asset
	if result != "http://example.com/third.7z" {
		t.Errorf("selectDownloadURL() = %q, want %q", result, "http://example.com/third.7z")
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
func TestAssetNames_NoDuplicates(t *testing.T) {
	assets := []api.Asset{
		{Name: "file1.zip"},
		{Name: "file2.zip"},
		{Name: "file3.zip"},
	}
	got := assetNames(assets)
	if len(got) != 3 {
		t.Fatalf("assetNames() len = %d, want 3", len(got))
	}
	if got[0] != "file1.zip" || got[1] != "file2.zip" || got[2] != "file3.zip" {
		t.Errorf("assetNames() = %v, want [file1.zip, file2.zip, file3.zip]", got)
	}
}

func TestReplaceVars_Extras(t *testing.T) {
	tests := []struct {
		testName string
		input    string
		path     string
		varName  string
		dlFile   string
		version  string
		expected string
	}{
		{
			testName: "all variables",
			input:    "%PATH/%NAME",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "/opt/app/myapp",
		},
		{
			testName: "version variable",
			input:    "app-%VER",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "2.0.0",
			expected: "app-2.0.0",
		},
		{
			testName: "dl filename variable",
			input:    "%DL_FILENAME",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "download.zip",
			version:  "1.0.0",
			expected: "download.zip",
		},
		{
			testName: "no variables",
			input:    "static string",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "static string",
		},
		{
			testName: "multiple occurrences",
			input:    "%NAME-%NAME",
			path:     "/opt/app",
			varName:  "test",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "test-test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			got := replaceVars(tt.input, tt.path, tt.varName, tt.dlFile, tt.version)
			if got != tt.expected {
				t.Errorf("replaceVars() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAssetNames_Empty(t *testing.T) {
	got := assetNames(nil)
	if len(got) != 0 {
		t.Errorf("assetNames(nil) len = %d, want 0", len(got))
	}
}

func TestArtifactNames(t *testing.T) {
	artifacts := []api.AppveyorArtifact{
		{FileName: "artifact1.zip"},
		{FileName: "artifact2.zip"},
	}
	got := artifactNames(artifacts)
	if len(got) != 2 {
		t.Fatalf("artifactNames() len = %d, want 2", len(got))
	}
	if got[0] != "artifact1.zip" || got[1] != "artifact2.zip" {
		t.Errorf("artifactNames() = %v, want [artifact1.zip, artifact2.zip]", got)
	}
}

func TestArtifactNames_Empty(t *testing.T) {
	got := artifactNames(nil)
	if len(got) != 0 {
		t.Errorf("artifactNames(nil) len = %d, want 0", len(got))
	}
}
