package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deorth-kku/updater-go/internal/config"
)

func TestExtractFilename(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"simple", "https://example.com/file.zip", "file.zip"},
		{"with query", "https://example.com/file.zip?token=abc", "file.zip"},
		{"with path", "https://example.com/path/to/file.zip", "file.zip"},
		{"no extension", "https://example.com/file", "file"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilename(tt.url)
			if got != tt.want {
				t.Errorf("extractFilename(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestUnescapeHTML(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"no entities", "https://example.com", "https://example.com"},
		{"amp", "https://example.com?a=1&amp;b=2", "https://example.com?a=1&b=2"},
		{"lt gt", "&lt;tag&gt;", "<tag>"},
		{"quot", "&quot;hello&quot;", `"hello"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeHTML(tt.s)
			if got != tt.want {
				t.Errorf("unescapeHTML(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestSiteName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"simple", "https://example.com/path", "https://example.com"},
		{"with port", "https://example.com:8080/path", "https://example.com:8080"},
		{"with subdomain", "https://api.example.com/v1", "https://api.example.com"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := siteName(tt.url)
			if got != tt.want {
				t.Errorf("siteName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestJoinURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		rel  string
		want string
	}{
		{"simple", "https://example.com", "file.zip", "https://example.com/file.zip"},
		{"with path", "https://example.com/path", "file.zip", "https://example.com/path/file.zip"},
		{"absolute", "https://example.com", "/file.zip", "https://example.com/file.zip"},
		{"empty base", "", "file.zip", "file.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinURL(tt.base, tt.rel)
			if got != tt.want {
				t.Errorf("joinURL(%q, %q) = %q, want %q", tt.base, tt.rel, got, tt.want)
			}
		})
	}
}
func TestBuildFromDirectURL_Basic(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg:  config.DownloadConfig{},
		verCfg: config.VersionConfig{},
	}

	_, err := s.buildFromDirectURL(t.Context(), "https://example.com/v1.0.0/app.zip", "")
	if err == nil {
		t.Fatalf("expected error when no regex configured, got nil")
	}
	t.Logf("Error = %v", err)
}

func TestBuildFromDirectURL_WithVersionRegex(t *testing.T) {
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{},
		verCfg: config.VersionConfig{
			Regex: `v(\d+\.\d+\.\d+)`,
		},
	}

	release, err := s.buildFromDirectURL(t.Context(), "https://example.com/v2.1.3.zip", "")
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

	release, err := s.buildFromDirectURL(t.Context(), "https://example.com/whatever/file.zip", "")
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

	// Python's direct-URL branch does NOT apply try_redirect (HEAD-follow);
	// a plain redirect is left as-is. Verify the URL is unchanged.
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			TryRedirect: true,
		},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(t.Context(), serverURL+"/original", "")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.URL != serverURL+"/original" {
		t.Errorf("URL = %q, want %q", release.URL, serverURL+"/original")
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

	release, err := s.buildFromDirectURL(t.Context(), server.URL+"/broken", "")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if release.URL != server.URL+"/broken" {
		t.Errorf("URL = %q, want %q", release.URL, server.URL+"/broken")
	}
}

// TestBuildFromDirectURL_DataPost verifies download.data triggers a POST that
// follows the 302/303 Location (gap #6).
func TestBuildFromDirectURL_DataPost(t *testing.T) {
	var gotMethod string
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = r.ParseForm()
		gotForm = r.Form
		if r.Method == http.MethodPost {
			w.Header().Set("Location", "/real/app.zip")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			URL:  server.URL + "/dl",
			Data: map[string]any{"token": "abc", "id": 42},
		},
		verCfg: config.VersionConfig{},
	}

	release, err := s.buildFromDirectURL(t.Context(), server.URL+"/dl", "")
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if got := gotForm.Get("token"); got != "abc" {
		t.Errorf("form token = %q, want %q", got, "abc")
	}
	if got := gotForm.Get("id"); got != "42" {
		t.Errorf("form id = %q, want %q", got, "42")
	}
	// Python returns the raw Location header (relative) unchanged.
	if release.URL != "/real/app.zip" {
		t.Errorf("URL = %q, want %q", release.URL, "/real/app.zip")
	}
}

// TestExtractURLFromPage_PerLevelIndexes verifies each regex level can select
// a specific match index via download.indexes (gap #17). Level 0 matches on
// the page; level 1 is fetched from the resolved URL and matched there.
func TestExtractURLFromPage_PerLevelIndexes(t *testing.T) {
	page := `<a href="/level1.html">next</a>`
	level1 := `<a href="https://final.example.com/app.zip">app</a>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/level1.html" {
			w.Write([]byte(level1))
			return
		}
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	s := &SimpleSpiderAPI{
		pageURL: srv.URL + "/",
		dlCfg: config.DownloadConfig{
			Regexes: []string{`href="([^"]+)"`, `href="([^"]+)"`},
			Indexes: []int{0, 0},
		},
		verCfg:     config.VersionConfig{},
		downloader: NewHTTPClient(time.Second),
	}
	got, err := s.extractURLFromPage(t.Context(), page)
	if err != nil {
		t.Fatalf("extractURLFromPage() error = %v", err)
	}
	// Level 0 resolves /level1.html; level 1 is fetched and resolves the
	// absolute final URL. The final resolved URL is returned.
	want := "https://final.example.com/app.zip"
	if got != want {
		t.Errorf("extractURLFromPage() = %q, want %q", got, want)
	}
}

// TestExtractVersion_Index verifies version.index selects the Nth regex match
// on the filename or page (gap #16).
func TestExtractVersion_Index(t *testing.T) {
	page := `version 1.2.3 and version 9.8.7`
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{},
		verCfg: config.VersionConfig{
			Regex:    `(\d+\.\d+\.\d+)`,
			FromPage: true,
			Index:    1,
		},
	}
	got, err := s.extractVersion("https://example.com/x.zip", page)
	if err != nil {
		t.Fatalf("extractVersion() error = %v", err)
	}
	if got != "9.8.7" {
		t.Errorf("extractVersion() = %q, want %q", got, "9.8.7")
	}
}

func TestBuildFromDirectURL_AndroidPlatformTools(t *testing.T) {
	// Reproduces the android-platform-tools_windows.json config issue:
	//   version.regex: data-text="([.0-9]{3,})
	//   version.from_page: true
	//   download.url: https://dl.google.com/android/repository/platform-tools-latest-windows.zip
	//
	// buildFromDirectURL currently ignores FromPage and always extracts version
	// from the filename. The regex pattern "data-text="..." will never match
	// "platform-tools-latest-windows.zip", so version comes back as "unknown".
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{
			URL: "https://dl.google.com/android/repository/platform-tools-latest-windows.zip",
		},
		verCfg: config.VersionConfig{
			Regex:    `data-text="([.0-9]{3,})`,
			FromPage: true,
		},
	}

	release, err := s.buildFromDirectURL(t.Context(), "https://dl.google.com/android/repository/platform-tools-latest-windows.zip", `<!DOCTYPE html><html><body><span data-text="34.0.7">34.0.7</span></body></html>`)
	if err != nil {
		t.Fatalf("buildFromDirectURL() error = %v", err)
	}
	t.Logf("Version = %q, URL = %q, Asset.Name = %q", release.Version, release.URL, release.Assets[0].Name)

	// With FromPage=true and page content provided, the regex should match
	// "data-text="34.0.7"" and extract "34.0.7".
	if release.Version != "34.0.7" {
		t.Errorf("Version = %q, want %q (regex should match page content when FromPage=true)", release.Version, "34.0.7")
	}
}

func TestBuildFromDirectURL_FromPageIgnored(t *testing.T) {
	// Direct test: does buildFromDirectURL respect verCfg.FromPage?
	// When FromPage=true but page is empty, extractVersion applies the regex
	// to the filename (since page is ""), which doesn't match → returns error.
	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{},
		verCfg: config.VersionConfig{
			Regex:    `data-text="([.0-9]{3,})`,
			FromPage: true,
		},
	}

	_, err := s.buildFromDirectURL(t.Context(), "https://dl.google.com/android/repository/platform-tools-latest-windows.zip", "")
	if err == nil {
		t.Error("expected error when regex doesn't match, got nil")
	}
	t.Logf("Error = %v", err)
}

func TestExtractVersion_FromPageVsFilename(t *testing.T) {
	// Compare extractVersion (which correctly uses FromPage) vs
	// buildFromDirectURL (which does not).
	page := `<span data-text="34.0.7">34.0.7</span>`
	filename := "platform-tools-latest-windows.zip"

	s := &SimpleSpiderAPI{
		dlCfg: config.DownloadConfig{},
		verCfg: config.VersionConfig{
			Regex:    `data-text="([.0-9]{3,})`,
			FromPage: true,
		},
	}

	// extractVersion with FromPage=true should match on page content
	got, err := s.extractVersion(filename, page)
	if err != nil {
		t.Fatalf("extractVersion() error = %v", err)
	}
	t.Logf("extractVersion (FromPage=true): version = %q", got)
	if got != "34.0.7" {
		t.Errorf("extractVersion with FromPage=true = %q, want %q", got, "34.0.7")
	}

	// extractVersion with FromPage=false should NOT match (filename has no data-text)
	s.verCfg.FromPage = false
	got2, err2 := s.extractVersion(filename, page)
	if err2 != nil {
		t.Logf("extractVersion (FromPage=false): error = %v (expected, no match in filename)", err2)
	} else {
		t.Errorf("extractVersion with FromPage=false = %q, expected error", got2)
	}
}
