package extractor

import (
	"log/slog"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/platform"
)

// FileSelector filters files based on keywords, exclude keywords, and file type.
type FileSelector struct {
	Keywords                  []string
	ExcludeKeywords           []string
	Filetype                  string
	ExcludeFileTypeWhenUpdate []string
	logger                    *slog.Logger
}

// NewFileSelector creates a FileSelector from the download and decompress configs.
func NewFileSelector(dlCfg config.DownloadConfig, dcCfg config.DecompressConfig, logger *slog.Logger) *FileSelector {
	return &FileSelector{
		Keywords:                  platform.ExpandKeywords(dlCfg.Keyword),
		ExcludeKeywords:           platform.ExpandKeywords(dlCfg.ExcludeKeyword),
		Filetype:                  dlCfg.Filetype.First(),
		ExcludeFileTypeWhenUpdate: dcCfg.ExcludeFileTypeWhenUpdate,
		logger:                    logger,
	}
}

// log returns the selector's logger, falling back to slog.Default when nil
// (e.g. in unit tests that construct a bare FileSelector struct literal).
func (fs *FileSelector) log() *slog.Logger {
	if fs.logger != nil {
		return fs.logger
	}
	return slog.Default()
}

// Match checks if a filename matches the selector criteria.
func (fs *FileSelector) Match(name string) bool {
	nameLower := strings.ToLower(name)

	// Check filetype
	if fs.Filetype != "" {
		ext := "." + strings.TrimPrefix(fs.Filetype, ".")
		if !strings.HasSuffix(nameLower, ext) {
			fs.log().Debug("file rejected",
				"name", name,
				"reason", "filetype does not match required extension",
				"result", "reject",
			)
			return false
		}
	}

	// Check exclude keywords (any match → reject)
	for _, ek := range fs.ExcludeKeywords {
		if strings.Contains(nameLower, strings.ToLower(ek)) {
			fs.log().Debug("file rejected",
				"name", name,
				"exclude_keyword", ek,
				"reason", "matched exclude keyword",
				"result", "reject",
			)
			return false
		}
	}

	// Check exclude file types when updating (any match → reject)
	for _, ext := range fs.ExcludeFileTypeWhenUpdate {
		if strings.HasSuffix(nameLower, strings.ToLower(ext)) {
			fs.log().Debug("file rejected",
				"name", name,
				"exclude_ext", ext,
				"reason", "matched exclude file type when updating",
				"result", "reject",
			)
			return false
		}
	}

	// Check keywords (all must match)
	for _, k := range fs.Keywords {
		if !strings.Contains(nameLower, strings.ToLower(k)) {
			fs.log().Debug("file rejected",
				"name", name,
				"keyword", k,
				"reason", "missing required keyword",
				"result", "reject",
			)
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
	fs.log().Debug("files selected",
		"total", len(names),
		"matched", len(result),
		"reason", "file selector applied to candidate list",
		"result", strings.Join(result, ","),
	)
	return result
}
