package extractor

import (
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/platform"
)

// FileSelector filters files based on keywords, exclude keywords, and file type.
type FileSelector struct {
	Keywords        []string
	ExcludeKeywords []string
	Filetype        string
}

// NewFileSelector creates a FileSelector from the download config.
func NewFileSelector(cfg config.DownloadConfig) *FileSelector {
	return &FileSelector{
		Keywords:        platform.ExpandKeywords(cfg.Keyword),
		ExcludeKeywords: platform.ExpandKeywords(cfg.ExcludeKeyword),
		Filetype:        cfg.Filetype.First(),
	}
}

// Match checks if a filename matches the selector criteria.
func (fs *FileSelector) Match(name string) bool {
	nameLower := strings.ToLower(name)

	// Check filetype
	if fs.Filetype != "" {
		ext := "." + strings.TrimPrefix(fs.Filetype, ".")
		if !strings.HasSuffix(nameLower, ext) {
			return false
		}
	}

	// Check exclude keywords (any match → reject)
	for _, ek := range fs.ExcludeKeywords {
		if strings.Contains(nameLower, strings.ToLower(ek)) {
			return false
		}
	}

	// Check keywords (all must match)
	for _, k := range fs.Keywords {
		if !strings.Contains(nameLower, strings.ToLower(k)) {
			return false
		}
	}

	return true
}

// SelectFiles filters a list of filenames, returning those that match.
func (fs *FileSelector) SelectFiles(names []string) []string {
	var result []string
	for _, name := range names {
		if fs.Match(name) {
			result = append(result, name)
		}
	}
	return result
}
