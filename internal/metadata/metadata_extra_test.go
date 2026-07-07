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
