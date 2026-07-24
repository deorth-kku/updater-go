package metadata

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/api"
)

// mockHTTPDownloader returns predefined metadata responses.
type mockHTTPDownloader struct {
	responses map[string]*api.HTTPResponse
}

func newMockHTTPDownloader() *mockHTTPDownloader {
	return &mockHTTPDownloader{responses: make(map[string]*api.HTTPResponse)}
}

func (m *mockHTTPDownloader) On(url string, resp *api.HTTPResponse) {
	m.responses[url] = resp
}

func (m *mockHTTPDownloader) Get(_ context.Context, url string, _ map[string]string) (*api.HTTPResponse, error) {
	if resp, ok := m.responses[url]; ok {
		return resp, nil
	}
	return &api.HTTPResponse{StatusCode: 404, Body: []byte("not found")}, nil
}

func (m *mockHTTPDownloader) Post(ctx context.Context, url string, _ []byte, _ map[string]string) (*api.HTTPResponse, error) {
	return m.Get(ctx, url, nil)
}

func TestStore_Load(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json", Date: "2024-01-01"},
		"project-b": {ConfigPath: "configs/project-b.json", Date: "2024-02-01"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify entries
	entry, ok := store.GetEntry("project-a")
	if !ok {
		t.Fatal("project-a not found in entries")
	}
	if entry.ConfigPath != "configs/project-a.json" {
		t.Errorf("ConfigPath = %q, want %q", entry.ConfigPath, "configs/project-a.json")
	}

	entry, ok = store.GetEntry("project-b")
	if !ok {
		t.Fatal("project-b not found in entries")
	}
	if entry.Date != "2024-02-01" {
		t.Errorf("Date = %q, want %q", entry.Date, "2024-02-01")
	}

	// Verify non-existent entry
	_, ok = store.GetEntry("nonexistent")
	if ok {
		t.Error("nonexistent should not be found")
	}
}

func TestStore_LoadMultipleRepos(t *testing.T) {
	entries1 := map[string]Entry{
		"project-a": {ConfigPath: "repo1/configs/project-a.json"},
	}
	body1, _ := json.Marshal(entries1)

	entries2 := map[string]Entry{
		"project-b": {ConfigPath: "repo2/configs/project-b.json"},
		"project-a": {ConfigPath: "repo2/configs/project-a.json"}, // Override
	}
	body2, _ := json.Marshal(entries2)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo1/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body1,
	})
	mock.On("https://example.com/repo2/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body2,
	})

	store := NewStore([]string{"https://example.com/repo1", "https://example.com/repo2"}, mock)
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// project-a should be from repo2 (last one wins)
	entry, ok := store.GetEntry("project-a")
	if !ok {
		t.Fatal("project-a not found")
	}
	if entry.ConfigPath != "repo2/configs/project-a.json" {
		t.Errorf("ConfigPath = %q, want %q", entry.ConfigPath, "repo2/configs/project-a.json")
	}

	// project-b should be from repo2
	entry, ok = store.GetEntry("project-b")
	if !ok {
		t.Fatal("project-b not found")
	}
	if entry.ConfigPath != "repo2/configs/project-b.json" {
		t.Errorf("ConfigPath = %q, want %q", entry.ConfigPath, "repo2/configs/project-b.json")
	}
}

func TestStore_LoadFailure(t *testing.T) {
	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 500, Body: []byte("server error"),
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	err := store.Load(t.Context())
	if err == nil {
		t.Error("Load() should return error for 500 response")
	}
}

func TestStore_Entries(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	allEntries := store.Entries()
	if len(allEntries) != 1 {
		t.Errorf("len(Entries()) = %d, want 1", len(allEntries))
	}
	if allEntries["project-a"].ConfigPath != "configs/project-a.json" {
		t.Errorf("Entries()[project-a].ConfigPath = %q", allEntries["project-a"].ConfigPath)
	}
}

func TestStore_EntriesReturnsCopy(t *testing.T) {
	entries := map[string]Entry{
		"project-a": {ConfigPath: "configs/project-a.json"},
	}
	body, _ := json.Marshal(entries)

	mock := newMockHTTPDownloader()
	mock.On("https://example.com/repo/metadata.json", &api.HTTPResponse{
		StatusCode: 200, Body: body,
	})

	store := NewStore([]string{"https://example.com/repo"}, mock)
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Get entries and modify the returned map
	allEntries := store.Entries()
	allEntries["project-b"] = Entry{ConfigPath: "should-not-affect-store"}

	// Verify original store is unchanged
	_, ok := store.GetEntry("project-b")
	if ok {
		t.Error("Modifying returned map should not affect store")
	}
}
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Try to ensure local config for a project not in metadata
	err := store.EnsureLocalConfig(t.Context(), "nonexistent")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config file
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not error even though remote is newer (we're just testing it doesn't crash)
	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config file that is newer than remote date
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not download since local is newer
	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create an old local config
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	// Create a local config
	cfgPath := filepath.Join(tmpDir, "project-a.json")
	os.WriteFile(cfgPath, []byte(`{"basic": {"api_type": "github"}}`), 0o644)

	// Should not download since date is empty
	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(t.Context(), "project-a")
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
	if err := store.Load(t.Context()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tmpDir := t.TempDir()
	store.SetLocalConfigDir(tmpDir)

	err := store.EnsureLocalConfig(t.Context(), "my-project")
	if err != nil {
		t.Fatalf("EnsureLocalConfig() error = %v", err)
	}

	// Verify the file was written with the correct name
	cfgPath := filepath.Join(tmpDir, "my-project.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file should have been created at my-project.json")
	}
}
