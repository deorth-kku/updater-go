package metadata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/api"
)

func TestStore_EnsureLocalConfig_NotFound(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Try to ensure local config for a project not in metadata
	err := store.EnsureLocalConfig(context.Background(), "nonexistent")
	if err == nil {
		t.Error("EnsureLocalConfig() should return error for nonexistent project")
	}
}

func TestStore_EnsureLocalConfig_DownloadsNewConfig(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json", Date: "2024-01-01"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})
	mock.On("https://example.com/configs/project-a.json", &api.HTTPResponse{
		StatusCode: 200, Body: []byte(`{"basic": {"api_type": "github"}}`),
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify config was downloaded
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file should have been created")
	}
}

func TestStore_EnsureLocalConfig_LocalConfigExists(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json", Date: "2024-01-01"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config file
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not error even though remote is newer (we're just testing it doesn't crash)
	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}
}

func TestStore_SetLocalConfigDir(t *testing.T) {
	store := NewStore([]string{"https://example.com/repo"}, newMockHTTPDownloader())

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Verify directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("SetLocalConfigDir() should create the directory")
	}
}

func TestStore_SetLocalConfigDir_Nested(t *testing.T) {
	store := NewStore([]string{"https://example.com/repo"}, newMockHTTPDownloader())

	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "dir")
	store.SetLocalConfigDir(nestedDir)

	// Verify nested directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("SetLocalConfigDir() should create nested directories")
	}
}

func TestStore_EnsureLocalConfig_LocalConfigUpToDate(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json", Date: "2020-01-01"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config file that is newer than remote date
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not download since local is newer
	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify config file was not modified (still has original content)
	cfgContent, _ := os.ReadFile(cfgPath)
	if string(cfgContent) != `{"basic": {"api_type": "github"}}` {
		t.Error("Config should not have been re-downloaded when local is up-to-date")
	}
}

func TestStore_EnsureLocalConfig_LocalConfigOlder(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json", Date: "2099-12-31"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})
	mock.On("https://example.com/configs/project-a.json", &api.HTTPResponse{
		StatusCode: 200, Body: []byte(`{"basic": {"api_type": "github"}}`),
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create an old local config
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify config was re-downloaded
	cfgContent, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(cfgContent) != `{"basic": {"api_type": "github"}}` {
		t.Errorf("config content = %q, want %q", string(cfgContent), `{"basic": {"api_type": "github"}}`)
	}
}

func TestStore_EnsureLocalConfig_EmptyDate(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", URL: "https://example.com/configs/project-a.json", Date: ""},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not download since date is empty
	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify config file was not modified
	cfgContent, _ := os.ReadFile(cfgPath)
	if string(cfgContent) != `{"basic": {"api_type": "github"}}` {
		t.Error("Config should not have been re-downloaded when date is empty")
	}
}

func TestStore_DownloadConfig(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})
	mock.On("https://example.com/repo/configs/project-a.json", &api.HTTPResponse{
		StatusCode: 200, Body: []byte(`{"basic": {"api_type": "github"}}`),
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(context.Background(), "project-a")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify the file was written
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(content) != `{"basic": {"api_type": "github"}}` {
		t.Errorf("config content = %q, want %q", string(content), `{"basic": {"api_type": "github"}}`)
	}
}

func TestStore_DownloadConfig_DownloadsToCorrectPath(t *testing.T) {
	entries := map[string]Entry{
		"my-project": {ConfigPath: "configs/my-project.json"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})
	mock.On("https://example.com/repo/configs/my-project.json", &api.HTTPResponse{
		StatusCode: 200, Body: []byte(`{"basic": {"api_type": "github"}}`),
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(context.Background(), "my-project")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify the file was written with the correct name
	cfgPath := filepath.Join(tmpDir, "my-project.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file should have been created at my-project.json")
	}
}
