package api

import (
	"log/slog"
	"os"
	"runtime"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func skipIfNotLinuxAmd64(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("skipping %%arch/%%OS tests on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestMatch_NoKeywords_NoExclude_NoFiletype(t *testing.T) {
	fs := &FileSelector{
		Keywords:        nil,
		ExcludeKeywords: nil,
		Filetype:        nil,
		logger:          newTestLogger(t),
	}

	if !fs.Match("anything.exe") {
		t.Error("expected match with no criteria")
	}
	if !fs.Match("random.zip") {
		t.Error("expected match with no criteria")
	}
}

func TestMatch_KeywordsInclude(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{
			{"x64", "windows"},
		},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("myapp_x64_windows.zip") {
		t.Error("expected match for name containing keyword")
	}
	if fs.Match("myapp_x86_linux.zip") {
		t.Error("expected no match when keyword not in name")
	}
}

func TestMatch_KeywordsANDAcrossGroups(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{
			{"x64"},
			{"windows"},
		},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("myapp_x64_windows.zip") {
		t.Error("expected match — both groups present")
	}
	if fs.Match("myapp_x64_linux.zip") {
		t.Error("expected no match — second group missing")
	}
	if fs.Match("myapp_x86_windows.zip") {
		t.Error("expected no match — first group missing")
	}
}

func TestMatch_KeywordsORWithinGroup(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{
			{"x64", "amd64"},
		},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("myapp_x64.zip") {
		t.Error("expected match — first OR option present")
	}
	if !fs.Match("myapp_amd64.zip") {
		t.Error("expected match — second OR option present")
	}
	if fs.Match("myapp_arm64.zip") {
		t.Error("expected no match — neither OR option present")
	}
}

func TestMatch_ExcludeKeywords(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"test"}},
		ExcludeKeyword: config.Keywords{{"debug", "dev"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("test_release.zip") {
		t.Error("expected match — exclude not in name")
	}
	if fs.Match("test_debug_build.zip") {
		t.Error("expected no match — exclude keyword present")
	}
	if fs.Match("test_dev_version.zip") {
		t.Error("expected no match — second exclude keyword present")
	}
}

func TestMatch_ExcludeOverridesKeywords(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"app"}},
		ExcludeKeyword: config.Keywords{{"app"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if fs.Match("myapp.zip") {
		t.Error("expected no match — exclude should override keyword")
	}
}

func TestMatch_Filetype(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:  config.Keywords{{"test"}},
		Filetype: config.Slice[string]{"zip", "7z"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("test_file.zip") {
		t.Error("expected match — filetype matches")
	}
	if !fs.Match("test_file.7z") {
		t.Error("expected match — filetype matches")
	}
	if fs.Match("test_file.tar.gz") {
		t.Error("expected no match — filetype does not match")
	}
}

func TestMatch_FiletypeCaseInsensitive(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:  config.Keywords{{"test"}},
		Filetype: config.Slice[string]{"ZIP"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("test_file.zip") {
		t.Error("expected match — extension case should be normalized")
	}
}

func TestMatch_FiletypeExtends(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:  config.Keywords{{"test"}},
		Filetype: config.Slice[string]{"tar.gz"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("test_file.tar.gz") {
		t.Error("expected match — multi-part extension should match")
	}
}

func TestMatch_UpdateKeyword_WhenInstallFalse(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"full"}},
		UpdateKeyword:  config.Keywords{{"lite"}},
		ExcludeKeyword: config.Keywords{{"debug"}},
	}
	fs := NewFileSelector(dlCfg, false, newTestLogger(t))

	// update_keyword replaces keyword
	if fs.Match("full_release.zip") {
		t.Error("expected no match — keyword should be replaced by update_keyword")
	}
	if !fs.Match("lite_release.zip") {
		t.Error("expected match — update_keyword is used instead")
	}

	// exclude_keyword is appended to exclude list
	if fs.Match("full_debug.zip") {
		t.Error("expected no match — keyword 'full' is now in exclude list")
	}
}

func TestMatch_UpdateKeyword_WhenInstallTrue(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"full"}},
		UpdateKeyword:  config.Keywords{{"lite"}},
		ExcludeKeyword: config.Keywords{{"debug"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	// keyword is used as-is
	if !fs.Match("full_release.zip") {
		t.Error("expected match — normal keyword used when install=true")
	}
	if fs.Match("lite_release.zip") {
		t.Error("expected no match — update_keyword not used when install=true")
	}
}

func TestMatch_UpdateKeywordEmpty(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"full"}},
		UpdateKeyword:  config.Keywords{},
		ExcludeKeyword: config.Keywords{{"debug"}},
	}
	fs := NewFileSelector(dlCfg, false, newTestLogger(t))

	// When update_keyword is empty, normal keyword is used
	if !fs.Match("full_release.zip") {
		t.Error("expected match — keyword used when update_keyword is empty")
	}
}

func TestMatch_FiletypeOnly_NoKeywords(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Filetype: config.Slice[string]{"exe"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("setup.exe") {
		t.Error("expected match — only filetype filter applied")
	}
	if fs.Match("readme.txt") {
		t.Error("expected no match — filetype does not match")
	}
}

func TestMatch_ExcludeOnly_NoKeywords(t *testing.T) {
	dlCfg := config.DownloadConfig{
		ExcludeKeyword: config.Keywords{{"debug"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("release.zip") {
		t.Error("expected match — no keywords means everything passes if not excluded")
	}
	if fs.Match("debug_build.zip") {
		t.Error("expected no match — excluded")
	}
}

func TestMatch_AllFiltersCombined(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"app"}},
		ExcludeKeyword: config.Keywords{{"debug", "test"}},
		Filetype:       config.Slice[string]{"zip"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("app_release.zip") {
		t.Error("expected match — all filters pass")
	}
	if fs.Match("app_debug.zip") {
		t.Error("expected no match — exclude keyword")
	}
	if fs.Match("app_release.tar.gz") {
		t.Error("expected no match — filetype does not match")
	}
	if fs.Match("other_release.zip") {
		t.Error("expected no match — keyword not in name")
	}
}

func TestMatch_EmptyExcludeKeyword(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"app"}},
		ExcludeKeyword: config.Keywords{},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("app_release.zip") {
		t.Error("expected match — empty exclude keyword should not filter anything")
	}
}

func TestMatch_EmptyFiletype(t *testing.T) {
	dlCfg := config.DownloadConfig{
		Keyword:  config.Keywords{{"app"}},
		Filetype: config.Slice[string]{},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("app_release.zip") {
		t.Error("expected match — empty filetype means no extension filter")
	}
	if !fs.Match("app_release.exe") {
		t.Error("expected match — empty filetype means no extension filter")
	}
}

func TestMatch_PctArch(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{{"%arch"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	// Should match files containing any expanded arch candidate
	if !fs.Match("myapp_x64.zip") {
		t.Error("expected match — %arch expands to include x64")
	}
	if !fs.Match("myapp_amd64.zip") {
		t.Error("expected match — %arch expands to include amd64")
	}
	if !fs.Match("myapp_x86_64.zip") {
		t.Error("expected match — %arch expands to include x86_64")
	}
	if !fs.Match("myapp_linux-64.zip") {
		t.Error("expected match — %arch expands to include linux-64")
	}
	// Should not match an arch that isn't a candidate on this platform
	if fs.Match("myapp_arm64.zip") {
		t.Error("expected no match — arm64 not a candidate on this platform")
	}
}

func TestMatch_PctOS(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{{"%OS"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	// Should match files containing any expanded OS candidate
	if !fs.Match("myapp_linux.zip") {
		t.Error("expected match — %OS expands to include linux")
	}
	if !fs.Match("myapp_Linux.zip") {
		t.Error("expected match — %OS expands to include Linux")
	}
	if !fs.Match("myapp_ubuntu.zip") {
		t.Error("expected match — %OS expands to include ubuntu")
	}
	// Should not match an OS that isn't a candidate on this platform
	if fs.Match("myapp_windows.zip") {
		t.Error("expected no match — windows not a candidate on this platform")
	}
}

func TestMatch_PctArchAndPctOS(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{{"%arch"}, {"%OS"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	// Both groups must match (AND across groups)
	if !fs.Match("myapp_x64_linux.zip") {
		t.Error("expected match — arch x64 + os linux both present")
	}
	if fs.Match("myapp_x64_windows.zip") {
		t.Error("expected no match — windows not a candidate on this platform")
	}
	if fs.Match("myapp_amd64_arm64.zip") {
		t.Error("expected no match — arm64 not a candidate on this platform")
	}
}

func TestMatch_PctArch_WithExclude(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	dlCfg := config.DownloadConfig{
		Keyword:        config.Keywords{{"%arch"}},
		ExcludeKeyword: config.Keywords{{"debug"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("myapp_x64_release.zip") {
		t.Error("expected match — arch matches, exclude not present")
	}
	if fs.Match("myapp_x64_debug.zip") {
		t.Error("expected no match — exclude keyword present")
	}
}

func TestMatch_PctOS_WithFiletype(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	dlCfg := config.DownloadConfig{
		Keyword:  config.Keywords{{"%OS"}},
		Filetype: config.Slice[string]{"zip"},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	if !fs.Match("myapp_linux.zip") {
		t.Error("expected match — os matches and filetype matches")
	}
	if fs.Match("myapp_linux.tar.gz") {
		t.Error("expected no match — filetype does not match")
	}
}

func TestMatch_PctArch_ExactTokenOnly(t *testing.T) {
	skipIfNotLinuxAmd64(t)
	// Partial usage like "x86_%arch" should NOT be expanded (kept verbatim)
	dlCfg := config.DownloadConfig{
		Keyword: config.Keywords{{"x86_%arch"}},
	}
	fs := NewFileSelector(dlCfg, true, newTestLogger(t))

	// The literal string "x86_%arch" is kept as-is since it's not exactly "%arch"
	if !fs.Match("x86_%arch_release.zip") {
		t.Error("expected match — literal token kept verbatim")
	}
	if fs.Match("x86_x64_release.zip") {
		t.Error("expected no match — %arch not expanded in partial context")
	}
}
