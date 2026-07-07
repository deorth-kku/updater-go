package api

import (
	"testing"
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
