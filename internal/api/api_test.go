package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

// mockDownloader is an in-memory mock that returns predefined responses.
type mockDownloader struct {
	handlers map[string]*HTTPResponse
}

func newMockDownloader() *mockDownloader {
	return &mockDownloader{handlers: make(map[string]*HTTPResponse)}
}

func (m *mockDownloader) On(path string, resp *HTTPResponse) {
	m.handlers[path] = resp
}

func (m *mockDownloader) Get(_ context.Context, url string, _ map[string]string) (*HTTPResponse, error) {
	path := url
	if _, after, ok := strings.Cut(url, "://"); ok {
		path = after
		if slashIdx := strings.Index(path, "/"); slashIdx >= 0 {
			path = path[slashIdx:]
		}
	}
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if resp, ok := m.handlers[path]; ok {
		return resp, nil
	}
	return &HTTPResponse{StatusCode: 404, Body: []byte("not found")}, nil
}

func (m *mockDownloader) Post(ctx context.Context, url string, _ []byte, _ map[string]string) (*HTTPResponse, error) {
	return m.Get(ctx, url, nil)
}

func TestGitHubAPI_Latest(t *testing.T) {
	fixture := []githubRelease{
		{
			TagName: "v2.47.0",
			Name:    "Git 2.47.0",
			Assets: []githubAsset{
				{Name: "PortableGit-2.47.0-64-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable.7z"},
				{Name: "PortableGit-2.47.0-32-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable32.7z"},
			},
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(fixture)
	mdl.On("/repos/git-for-windows/git/releases", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewGitHubAPI(config.BasicConfig{
		AccountName: "git-for-windows",
		ProjectName: "git",
	}, mdl, slog.Default())

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Version != "v2.47.0" {
		t.Errorf("Version = %q, want %q", rel.Version, "v2.47.0")
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(rel.Assets))
	}
}

// TestGitHubAPI_LatestByVersion_MatchByName tests rollback when names are unique
// and the target matches a release name (not tag_name).
func TestGitHubAPI_LatestByVersion_MatchByName(t *testing.T) {
	fixture := []githubRelease{
		{
			TagName: "v2.47.0",
			Name:    "Git 2.47.0",
			Assets: []githubAsset{
				{Name: "PortableGit-2.47.0-64-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable.7z"},
			},
		},
		{
			TagName: "v2.46.0",
			Name:    "Git 2.46.0",
			Assets: []githubAsset{
				{Name: "PortableGit-2.46.0-64-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable2.7z"},
			},
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(fixture)
	mdl.On("/repos/git-for-windows/git/releases", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewGitHubAPI(config.BasicConfig{
		AccountName: "git-for-windows",
		ProjectName: "git",
	}, mdl, slog.Default())

	rel, err := api.LatestByVersion(t.Context(), "Git 2.46.0")
	if err != nil {
		t.Fatalf("LatestByVersion() error = %v", err)
	}
	if rel.Version != "Git 2.46.0" {
		t.Errorf("Version = %q, want %q", rel.Version, "Git 2.46.0")
	}
	if rel.Assets[0].Name != "PortableGit-2.46.0-64-bit.7z" {
		t.Errorf("Asset.Name = %q, want %q", rel.Assets[0].Name, "PortableGit-2.46.0-64-bit.7z")
	}
}

// TestGitHubAPI_LatestByVersion_MatchByTagName tests rollback when names are
// identical (single tag with multiple builds) so version falls back to tag_name.
func TestGitHubAPI_LatestByVersion_MatchByTagName(t *testing.T) {
	fixture := []githubRelease{
		{
			TagName: "v2.47.0",
			Name:    "v2.47.0",
			Assets: []githubAsset{
				{Name: "PortableGit-2.47.0-64-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable.7z"},
			},
		},
		{
			TagName: "v2.46.0",
			Name:    "v2.46.0",
			Assets: []githubAsset{
				{Name: "PortableGit-2.46.0-64-bit.7z", BrowserDownloadURL: "https://github.com/releases/portable2.7z"},
			},
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(fixture)
	mdl.On("/repos/git-for-windows/git/releases", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewGitHubAPI(config.BasicConfig{
		AccountName: "git-for-windows",
		ProjectName: "git",
	}, mdl, slog.Default())

	rel, err := api.LatestByVersion(t.Context(), "v2.46.0")
	if err != nil {
		t.Fatalf("LatestByVersion() error = %v", err)
	}
	if rel.Version != "v2.46.0" {
		t.Errorf("Version = %q, want %q", rel.Version, "v2.46.0")
	}
}

// TestGitHubAPI_LatestByVersion_NotFound tests that an error is returned when
// the target version is not found in the releases list.
func TestGitHubAPI_LatestByVersion_NotFound(t *testing.T) {
	fixture := []githubRelease{
		{
			TagName: "v2.47.0",
			Name:    "Git 2.47.0",
			Assets:  []githubAsset{},
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(fixture)
	mdl.On("/repos/git-for-windows/git/releases", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewGitHubAPI(config.BasicConfig{
		AccountName: "git-for-windows",
		ProjectName: "git",
	}, mdl, slog.Default())

	rel, err := api.LatestByVersion(t.Context(), "v9.9.9")
	if err == nil {
		t.Fatalf("LatestByVersion() expected error, got %v", rel)
	}
}

func TestAppveyorAPI_Latest(t *testing.T) {
	history := appveyorHistory{
		Builds: []appveyorBuild{
			{Version: "1.0.0", PullRequestID: "pr-1"},
			{Version: "1.0.1", PullRequestID: ""},
		},
	}
	buildDetail := appveyorBuildDetail{
		Build: struct {
			Jobs    []appveyorJob `json:"jobs"`
			Updated string        `json:"updated"`
		}{
			Jobs: []appveyorJob{{Name: "release", ID: "job-123"}},
		},
	}
	artifacts := []AppveyorArtifact{{FileName: "rpcs3-win64-vulkan.zip"}}

	mdl := newMockDownloader()
	hBody, _ := json.Marshal(history)
	bBody, _ := json.Marshal(buildDetail)
	aBody, _ := json.Marshal(artifacts)
	mdl.On("/api/projects/blueskythlikesclouds/mikumikulibrary/history", &HTTPResponse{StatusCode: 200, Body: hBody})
	mdl.On("/api/projects/blueskythlikesclouds/mikumikulibrary/build/1.0.1", &HTTPResponse{StatusCode: 200, Body: bBody})
	mdl.On("/api/buildjobs/job-123/artifacts", &HTTPResponse{StatusCode: 200, Body: aBody})

	api := NewAppveyorAPI(config.BasicConfig{
		AccountName: "blueskythlikesclouds",
		ProjectName: "mikumikulibrary",
	}, mdl, slog.Default())

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Version != "1.0.1" {
		t.Errorf("Version = %q, want %q (PR build 1.0.0 should be skipped)", rel.Version, "1.0.1")
	}
	if rel.JobID != "job-123" {
		t.Errorf("JobID = %q, want %q", rel.JobID, "job-123")
	}
}

func TestSourceforgeAPI_Latest(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><title>/7zip_23.01-x64.exe</title><pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate></item>
  <item><title>/7zip_23.01-x86.exe</title><pubDate>Sun, 31 Dec 2023 00:00:00 +0000</pubDate></item>
</channel></rss>`

	mdl := newMockDownloader()
	mdl.On("/projects/sevenzip/rss", &HTTPResponse{StatusCode: 200, Body: []byte(rss)})

	api := NewSourceforgeAPI(config.BasicConfig{ProjectName: "sevenzip"}, config.DownloadConfig{Filetype: config.Slice[string]{"exe"}}, mdl, slog.Default())

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.URL == "" {
		t.Error("URL is empty")
	}
	if rel.Version == "" {
		t.Error("Version is empty")
	}
}

func newFrag[T string | int](v T) config.PathSegment {
	switch v := any(v).(type) {
	case string:
		return config.PathSegment{Str: v, Int: -1}
	case int:
		return config.PathSegment{Int: v}
	default:
		panic("not possible")
	}
}

func TestApiJsonAPI_Latest(t *testing.T) {
	jsonData := []any{
		map[string]any{
			"id":        float64(12345),
			"apkName":   "skyline-v2024.3.11.r1.apk",
			"runNumber": float64(98765),
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(jsonData)
	mdl.On("/builds", &HTTPResponse{StatusCode: 200, Body: body})

	path := []config.StringOrJsonPath{
		{Str: "https://skyline-builds.alula.gay/cache"},
		{Path: []config.PathSegment{newFrag(0), newFrag("apkName")}},
	}

	api := NewApiJsonAPI(
		config.BasicConfig{APIURL: "https://skyline-builds.alula.gay/builds"},
		config.DownloadConfig{
			Path:     path,
			Filetype: config.Slice[string]{"apk"},
		},
		config.VersionConfig{Regex: "skyline-v(.*?)\\.apk"},
		mdl,
		slog.Default(),
	)

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.URL == "" {
		t.Error("URL is empty")
	}
}

func TestDictPathGet(t *testing.T) {
	data := []any{
		map[string]any{
			"id":   float64(42),
			"name": "test",
		},
	}

	tests := []struct {
		name    string
		path    []config.PathSegment
		wantErr bool
	}{
		{"array index", []config.PathSegment{{Int: 0}}, false},
		{"nested via index then key", []config.PathSegment{newFrag(0), newFrag("id")}, false},
		{"out of range", []config.PathSegment{{Int: 99}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dictPathGet(data, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("dictPathGet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAPI_UnknownType(t *testing.T) {
	_, err := NewAPI(config.BasicConfig{APIType: "unknown"}, config.DownloadConfig{}, config.VersionConfig{}, config.BuildConfig{}, nil, slog.Default())
	if err == nil {
		t.Error("NewAPI() expected error for unknown api_type")
	}
}

func TestGitHubAPI_NoPull(t *testing.T) {
	releases := []githubRelease{
		{
			TagName: "v3.0.0",
			Name:    "Latest Release",
			Assets: []githubAsset{
				{Name: "app-v3.0.0.zip", BrowserDownloadURL: "https://github.com/releases/latest.zip"},
			},
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(releases)
	mdl.On("/repos/test/project/releases", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewGitHubAPI(config.BasicConfig{
		AccountName: "test",
		ProjectName: "project",
	}, mdl, slog.Default())
	api.SetNoPreRelease(true)

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Version != "v3.0.0" {
		t.Errorf("Version = %q, want %q", rel.Version, "v3.0.0")
	}
}

func TestAppveyorAPI_BranchFilter(t *testing.T) {
	history := appveyorHistory{
		Builds: []appveyorBuild{
			{Version: "1.0.0", PullRequestID: ""},
		},
	}
	buildDetail := appveyorBuildDetail{
		Build: struct {
			Jobs    []appveyorJob `json:"jobs"`
			Updated string        `json:"updated"`
		}{
			Jobs: []appveyorJob{{Name: "release", ID: "job-123"}},
		},
	}
	artifacts := []AppveyorArtifact{{FileName: "app.zip"}}

	mdl := newMockDownloader()
	hBody, _ := json.Marshal(history)
	bBody, _ := json.Marshal(buildDetail)
	aBody, _ := json.Marshal(artifacts)
	mdl.On("/api/projects/test/project/history", &HTTPResponse{StatusCode: 200, Body: hBody})
	mdl.On("/api/projects/test/project/build/1.0.0", &HTTPResponse{StatusCode: 200, Body: bBody})
	mdl.On("/api/buildjobs/job-123/artifacts", &HTTPResponse{StatusCode: 200, Body: aBody})

	api := NewAppveyorAPI(config.BasicConfig{
		AccountName: "test",
		ProjectName: "project",
	}, mdl, slog.Default())
	api.SetBranch("main")

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", rel.Version, "1.0.0")
	}
}

func TestSimpleSpiderAPI_HeadersFromConfig(t *testing.T) {
	// Test that headers from BasicConfig are used by verifying the API is created correctly
	api := NewSimpleSpiderAPI(
		config.BasicConfig{
			PageURL: "https://example.com/page",
			Headers: map[string]string{
				"User-Agent": "CustomAgent/1.0",
				"Accept":     "text/html",
			},
		},
		config.DownloadConfig{Regexes: []string{`href="([^"]+\.zip)"`}},
		config.VersionConfig{},
		nil,
		slog.Default(),
	)

	// Verify the API was created with the correct headers
	if api.headers["User-Agent"] != "CustomAgent/1.0" {
		t.Errorf("headers[User-Agent] = %q, want %q", api.headers["User-Agent"], "CustomAgent/1.0")
	}
	if api.headers["Accept"] != "text/html" {
		t.Errorf("headers[Accept] = %q, want %q", api.headers["Accept"], "text/html")
	}
}

func TestSimpleSpiderAPI_PostBody(t *testing.T) {
	// Test that Data field is accepted without error during construction
	api := NewSimpleSpiderAPI(
		config.BasicConfig{PageURL: "https://example.com/api/search"},
		config.DownloadConfig{
			Data: map[string]any{
				"query": "test",
				"page":  float64(1),
			},
		},
		config.VersionConfig{},
		nil,
		slog.Default(),
	)

	// Verify the Data field is stored
	if api.dlCfg.Data["query"] != "test" {
		t.Errorf("Data[query] = %v, want %q", api.dlCfg.Data["query"], "test")
	}
	if api.dlCfg.Data["page"] != float64(1) {
		t.Errorf("Data[page] = %v, want %v", api.dlCfg.Data["page"], float64(1))
	}
}

func TestApiJsonAPI_VersionExtraction(t *testing.T) {
	jsonData := map[string]any{
		"version": "2.0.0",
		"download": map[string]any{
			"url": "https://example.com/app-v2.0.0.zip",
		},
	}

	mdl := newMockDownloader()
	body, _ := json.Marshal(jsonData)
	mdl.On("/api/versions", &HTTPResponse{
		StatusCode: 200,
		Body:       body,
	})

	api := NewApiJsonAPI(
		config.BasicConfig{APIURL: "https://example.com/api/versions"},
		config.DownloadConfig{
			Path: []config.StringOrJsonPath{
				{Path: []config.PathSegment{newFrag("download"), newFrag("url")}},
			},
		},
		config.VersionConfig{
			Path: []config.PathSegment{
				{Str: "version", Int: -1},
			},
		},
		mdl,
		slog.Default(),
	)

	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.URL == "" {
		t.Error("URL is empty")
	}
}

func TestApiJsonAPI_DictPathGet(t *testing.T) {
	data := []any{
		map[string]any{
			"id":   float64(42),
			"name": "test",
		},
	}

	tests := []struct {
		name    string
		path    []config.PathSegment
		wantErr bool
	}{
		{"array index", []config.PathSegment{{Int: 0}}, false},
		{"nested via index then key", []config.PathSegment{{Int: 0}, {Str: "id", Int: -1}}, false},
		{"out of range", []config.PathSegment{{Int: 99}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dictPathGet(data, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("dictPathGet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

var _ = http.MethodGet
