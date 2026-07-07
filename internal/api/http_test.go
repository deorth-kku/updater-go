package api

import (
	"testing"
)

func TestParseProxyURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"with scheme", "http://proxy:8080", "http://proxy:8080"},
		{"without scheme", "proxy:8080", "http://proxy:8080"},
		{"with https", "https://proxy:8443", "https://proxy:8443"},
		{"invalid", "://invalid", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProxyURL(tt.raw)
			if tt.want == "" {
				if got != nil {
					t.Errorf("parseProxyURL(%q) = %v, want nil", tt.raw, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseProxyURL(%q) = nil, want %q", tt.raw, tt.want)
			}
			if got.String() != tt.want {
				t.Errorf("parseProxyURL(%q) = %q, want %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

func TestParseProxyURL_Invalid(t *testing.T) {
	got := parseProxyURL("://invalid")
	if got != nil {
		t.Errorf("parseProxyURL('://invalid') = %v, want nil", got)
	}
}
