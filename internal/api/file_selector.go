package api

import (
	"log/slog"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/platform"
)

// FileSelector filters files based on keywords, exclude keywords, and file type.
type FileSelector struct {
	Keywords        config.Keywords
	ExcludeKeywords config.Keywords
	Filetype        config.Slice[string]
	logger          *slog.Logger
}

// NewFileSelector creates a FileSelector from the download configs.
// install mirrors updater-rpc's install mode: when false and update_keyword is
// configured, the update_keyword/exclude_keyword pair is used instead of
// keyword/exclude_keyword (gap #9). When install is true (or update_keyword is
// empty) the normal keyword is used and update_keyword is appended to the
// exclude list, exactly like Python's getDlUrl branching.
func NewFileSelector(dlCfg config.DownloadConfig, install bool, logger *slog.Logger) *FileSelector {
	fs := &FileSelector{
		Keywords:        dlCfg.Keyword,
		ExcludeKeywords: platform.ExpandKeywords(dlCfg.ExcludeKeyword),
		Filetype:        toLower(dlCfg.Filetype),
		logger:          logger,
	}
	if !install && len(dlCfg.UpdateKeyword) > 0 {
		fs.Keywords = dlCfg.UpdateKeyword
	}
	fs.Keywords = platform.ExpandKeywords(fs.Keywords)
	return fs
}

func toLower(in []string) (out []string) {
	out = make([]string, len(in))
	for i, v := range in {
		out[i] = strings.ToLower(v)
	}
	return
}

// log returns the selector's logger, falling back to slog.Default when nil
// (e.g. in unit tests that construct a bare FileSelector struct literal).
func (fs *FileSelector) log() *slog.Logger {
	if fs.logger != nil {
		return fs.logger
	}
	return slog.Default()
}

func matchOneOf(keys []string, name string) bool {
	for _, v := range keys {
		if strings.Contains(name, v) {
			return true
		}
	}
	return false
}

func matchKeywords(kw config.Keywords, name string) bool {
	for _, v := range kw {
		if !matchOneOf(v, name) {
			return false
		}
	}
	return true
}

func matchExcludeKeywords(kw config.Keywords, name string) bool {
	for _, v := range kw {
		if matchOneOf(v, name) {
			return true
		}
	}
	return false
}

func matchSuffix(exts []string, name string) bool {
	name = strings.ToLower(name)
	for _, ext := range exts {
		ext = strings.ToLower(ext)
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// Match checks if a filename matches the selector criteria.
func (fs *FileSelector) Match(name string) bool {
	if len(fs.Filetype) > 0 {
		if !matchSuffix(fs.Filetype, name) {
			fs.log().Debug("skip file because ext not matched", "name", name)
			return false
		}
	}

	if matchExcludeKeywords(fs.ExcludeKeywords, name) {
		fs.log().Debug("skip file because exclude matched", "name", name)
		return false
	}

	if len(fs.Keywords) == 0 {
		fs.log().Debug("matched file because no include given", "name", name)
		return true
	}

	if matchKeywords(fs.Keywords, name) {
		fs.log().Debug("matched file because include matched", "name", name)
		return true
	}
	fs.log().Debug("skip file because no include matched", "name", name)
	return false
}
