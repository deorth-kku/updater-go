package extractor

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/platform"
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
	}, config.DecompressConfig{}, false, slog.Default())

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
	}, config.DecompressConfig{}, false, slog.Default())

	// "%arch" expands to the full architecture candidate list (gap #27), so the
	// keyword count is len(ArchCandidates)+1 for "release".
	want := len(platform.ArchCandidates()) + 1
	if len(fs.Keywords) != want {
		t.Fatalf("Keywords len = %d, want %d", len(fs.Keywords), want)
	}
	if fs.Keywords[0] == "%arch" {
		t.Errorf("Keywords[0] not expanded, still %s", fs.Keywords[0])
	}
	if !slices.Contains(fs.Keywords, "release") {
		t.Errorf("Keywords missing verbatim entry %q: %v", "release", fs.Keywords)
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
		false,
		slog.Default(),
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

// TestFileSelector_UpdateKeywordSwitching verifies the install/update keyword
// branch (gap #9), mirroring updater-rpc's getDlUrl logic.
func TestFileSelector_UpdateKeywordSwitching(t *testing.T) {
	dl := config.DownloadConfig{
		Keyword:        config.StringOrSlice{"install-build"},
		UpdateKeyword:  config.StringOrSlice{"update-build"},
		ExcludeKeyword: config.StringOrSlice{"beta"},
		Filetype:       config.StringOrSlice{"zip"},
	}

	// Re-update mode: update_keyword replaces keyword; exclude_keyword kept.
	fsUpdate := NewFileSelector(dl, config.DecompressConfig{}, false, slog.Default())
	if !slices.Contains(fsUpdate.Keywords, "update-build") {
		t.Errorf("update mode keywords missing update-build: %v", fsUpdate.Keywords)
	}
	if slices.Contains(fsUpdate.Keywords, "install-build") {
		t.Errorf("update mode keywords should not contain install-build: %v", fsUpdate.Keywords)
	}

	// Install mode: keyword used; update_keyword appended to exclude list.
	fsInstall := NewFileSelector(dl, config.DecompressConfig{}, true, slog.Default())
	if !slices.Contains(fsInstall.Keywords, "install-build") {
		t.Errorf("install mode keywords missing install-build: %v", fsInstall.Keywords)
	}
	if !slices.Contains(fsInstall.ExcludeKeywords, "update-build") {
		t.Errorf("install mode exclude missing update-build: %v", fsInstall.ExcludeKeywords)
	}
	if !slices.Contains(fsInstall.ExcludeKeywords, "beta") {
		t.Errorf("install mode exclude missing beta: %v", fsInstall.ExcludeKeywords)
	}
}

// TestFileSelector_UpdateKeywordEmptyUsesKeyword verifies that when
// update_keyword is empty the normal keyword branch is always used.
func TestFileSelector_UpdateKeywordEmptyUsesKeyword(t *testing.T) {
	dl := config.DownloadConfig{
		Keyword:  config.StringOrSlice{"install-build"},
		Filetype: config.StringOrSlice{"zip"},
	}
	fs := NewFileSelector(dl, config.DecompressConfig{}, false, slog.Default())
	if !slices.Contains(fs.Keywords, "install-build") {
		t.Errorf("expected install-build in keywords: %v", fs.Keywords)
	}
}
