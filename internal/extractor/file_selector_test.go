package extractor

import (
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

func TestFileSelector_Match(t *testing.T) {
	tests := []struct {
		name      string
		selectors FileSelector
		filename  string
		want      bool
	}{
		{
			name:      "keyword match",
			selectors: FileSelector{Keywords: []string{"PortableGit", "64-bit"}, Filetype: "exe"},
			filename:  "PortableGit-2.47.0-64-bit.7z.exe",
			want:      true,
		},
		{
			name:      "keyword no match",
			selectors: FileSelector{Keywords: []string{"PortableGit", "32-bit"}, Filetype: "exe"},
			filename:  "PortableGit-2.47.0-64-bit.7z.exe",
			want:      false,
		},
		{
			name:      "exclude keyword",
			selectors: FileSelector{Keywords: []string{"win"}, Filetype: "zip", ExcludeKeywords: []string{"cudart"}},
			filename:  "llama-win-cuda-12-cudart.zip",
			want:      false,
		},
		{
			name:      "exclude keyword pass",
			selectors: FileSelector{Keywords: []string{"win"}, Filetype: "zip", ExcludeKeywords: []string{"cudart"}},
			filename:  "llama-win-vulkan.zip",
			want:      true,
		},
		{
			name:      "filetype match",
			selectors: FileSelector{Keywords: []string{"x64"}, Filetype: "exe"},
			filename:  "7zip-x64.exe",
			want:      true,
		},
		{
			name:      "filetype no match",
			selectors: FileSelector{Keywords: []string{"x64"}, Filetype: "zip"},
			filename:  "7zip-x64.exe",
			want:      false,
		},
		{
			name:      "empty keywords",
			selectors: FileSelector{Filetype: "zip"},
			filename:  "anything.zip",
			want:      true,
		},
		{
			name:      "case insensitive",
			selectors: FileSelector{Keywords: []string{"win"}, Filetype: "zip"},
			filename:  "test-Win.Zip",
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.selectors.Match(tt.filename); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestFileSelector_SelectFiles(t *testing.T) {
	fs := NewFileSelector(config.DownloadConfig{
		Keyword:        config.StringOrSlice{"win", "vulkan"},
		Filetype:       config.StringOrSlice{"zip"},
		ExcludeKeyword: config.StringOrSlice{"cudart"},
	}, config.DecompressConfig{})

	input := []string{
		"llama-win-vulkan.zip",
		"llama-win-cuda-12-cudart.zip",
		"llama-linux-vulkan.zip",
		"llama-win-vulkan.exe",
	}
	got := fs.SelectFiles(input)
	want := []string{"llama-win-vulkan.zip"}
	if len(got) != len(want) {
		t.Fatalf("SelectFiles() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("SelectFiles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNewFileSelector_ExpandKeywords(t *testing.T) {
	fs := NewFileSelector(config.DownloadConfig{
		Keyword:  config.StringOrSlice{"%arch", "release"},
		Filetype: config.StringOrSlice{"zip"},
	}, config.DecompressConfig{})

	if len(fs.Keywords) != 2 {
		t.Fatalf("Keywords len = %d, want 2", len(fs.Keywords))
	}
	if fs.Keywords[0] == "%arch" {
		t.Errorf("Keywords[0] not expanded, still %s", fs.Keywords[0])
	}
}

func TestFileSelector_ExcludeFileTypeWhenUpdate(t *testing.T) {
	fs := NewFileSelector(
		config.DownloadConfig{
			Keyword:  config.StringOrSlice{"app"},
			Filetype: config.StringOrSlice{"zip"},
		},
		config.DecompressConfig{
			ExcludeFileTypeWhenUpdate: []string{".sig", ".sha256"},
		},
	)

	tests := []struct {
		name string
		want bool
	}{
		{"app-v1.0.zip", true},
		{"app-v1.0.zip.sig", false},
		{"app-v1.0.zip.sha256", false},
		{"other-v1.0.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fs.Match(tt.name); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
