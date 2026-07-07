package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

func TestBuildFromDirectURL_Basic(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg:  config.DownloadConfig{},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(context.Background(), "https://example.com/v1.0.0/app.zip")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.Version != "unknown" {
		t.Errorf("Version = %q, want %q", release.Version, "unknown")
	}
	if release.Assets[0].Name != "app.zip" {
		t.Errorf("Asset.Name = %q, want %q", release.Assets[0].Name, "app.zip")
	}
}

func TestBuildFromDirectURL_WithVersionRegex(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{},
		verCfg: config.VersionConfig{
			Regex: `v(\d+\.\d+\.\d+)`,
		},
	}

	release, err := s.buildFromDirectURL(context.Background(), "https://example.com/v2.1.3.zip")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.Version != "2.1.3" {
		t.Errorf("Version = %q, want %q", release.Version, "2.1.3")
	}
}

func TestBuildFromDirectURL_WithFilenameOverride(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			FilenameOverride: "custom.zip",
		},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(context.Background(), "https://example.com/whatever/file.zip")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.Assets[0].Name != "custom.zip" {
		t.Errorf("Asset.Name = %q, want %q", release.Assets[0].Name, "custom.zip")
	}
}

func TestBuildFromDirectURL_WithRedirect(t *testing.T) {
	// Server that redirects with absolute URL
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/original" {
			http.Redirect(w, r, serverURL+"/final", http.StatusFound)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	serverURL = server.URL

	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			TryRedirect: true,
		},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(context.Background(), serverURL+"/original")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.URL != serverURL+"/final" {
		t.Errorf("URL = %q, want %q", release.URL, serverURL+"/final")
	}
}

func TestBuildFromDirectURL_RedirectFail(t *testing.T) {
	// Server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			TryRedirect: true,
		},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(context.Background(), server.URL+"/broken")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	// Should still return the original URL (redirect failed silently)
	if release.URL != server.URL+"/broken" {
		t.Errorf("URL = %q, want %q", release.URL, server.URL+"/broken")
	}
}

func TestBuildFromDirectURL_FilenameOverrideWithVersion(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			FilenameOverride:     "app-{version}.zip",
			AddVersionToFilename: true,
		},
		verCfg: config.VersionConfig{
			Regex: `v(\d+\.\d+\.\d+)`,
		},
	}

	release, err := s.buildFromDirectURL(context.Background(), "https://example.com/v3.2.1.zip")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.Assets[0].Name != "app-3.2.1.zip" {
		t.Errorf("Asset.Name = %q, want %q", release.Assets[0].Name, "app-3.2.1.zip")
	}
}
