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
