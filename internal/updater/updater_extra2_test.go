package updater

import (
	"testing"
)

func TestIsArchive(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".zip", true},
		{".tar.gz", true},
		{".tgz", true},
		{".tar.xz", true},
		{".txz", true},
		{".exe", false},
		{".apk", false},
		{".dmg", false},
		{".deb", false},
		{".rpm", false},
		{".txt", false},
		{"", false},
		{".rar", false},
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
