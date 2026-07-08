package updater

import (
	"testing"
)

func TestIsArchive(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		// Archive formats supported by mholt/archives
		{".zip", true},
		{".tar", true},
		{".tar.gz", true},
		{".tgz", true},
		{".tar.xz", true},
		{".txz", true},
		{".tar.bz2", true},
		{".tbz", true},
		{".tbz2", true},
		{".tar.zst", true},
		{".tzst", true},
		{".tar.lz4", true},
		{".tar.lz", true},
		{".tar.br", true},
		{".tar.z", true},
		{".tar.lzma", true},
		{".7z", true},
		{".rar", true},
		{".gz", true},
		{".bz2", true},
		{".zst", true},
		{".lz4", true},
		{".sz", true},
		{".s2", true},
		{".br", true},
		{".z", true},
		{".lz", true},
		{".lzma", true},
		{".xz", true},
		{".zlib", true},
		{".exe", true}, // SFX archives
		// Non-archive files
		{".apk", false},
		{".dmg", false},
		{".deb", false},
		{".rpm", false},
		{".txt", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isArchive(tt.ext)
			if got != tt.want {
				t.Errorf("isArchive(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}
