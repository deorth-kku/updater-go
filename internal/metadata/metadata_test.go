package metadata

import (
	"context"
	"encoding/json"
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

func (m *mockHTTPDownloader) Get(_ context.Context, url string) (*api.HTTPResponse, error) {
	if resp, ok := m.responses[url]; ok {
		return resp, nil
	}
	return &api.HTTPResponse{StatusCode: 404, Body: []byte("not found")}, nil
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
	if err := store.Load(context.Background()); err != nil {
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
	if err := store.Load(context.Background()); err != nil {
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
	err := store.Load(context.Background())
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
	if err := store.Load(context.Background()); err != nil {
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
	if err := store.Load(context.Background()); err != nil {
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
