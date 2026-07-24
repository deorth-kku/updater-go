package api

import (
	"io"
	"log/slog"
	"path"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

// slogDiscard returns a slog.Logger that discards all output.
func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestSourceforge_Latest_Filtering verifies keyword/filetype/index matching
// over the RSS feed (gap #15). Uses an in-memory mock RSS feed.
func TestSourceforge_Latest_Filtering(t *testing.T) {
	rss := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>7z2401-x64.exe</title>
      <pubDate>Mon, 01 Jan 2024 00:00:00 UT</pubDate>
    </item>
    <item>
      <title>7z2401-x86.exe</title>
      <pubDate>Tue, 02 Jan 2024 00:00:00 UT</pubDate>
    </item>
    <item>
      <title>7z2401-x64.msi</title>
      <pubDate>Wed, 03 Jan 2024 00:00:00 UT</pubDate>
    </item>
  </channel>
</rss>`
	mdl := newMockDownloader()
	mdl.On("/projects/sevenzip/rss", &HTTPResponse{StatusCode: 200, Body: []byte(rss)})

	api := NewSourceforgeAPI(
		config.BasicConfig{ProjectName: "sevenzip"},
		config.DownloadConfig{
			Keyword:  config.SimpleKeywords("x64"),
			Filetype: config.Slice[string]{"exe"},
			Index:    0,
		},
		mdl,
		slogDiscard(),
	)
	rel, err := api.Latest(t.Context())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	basenameIs(t, rel, "7z2401-x64.exe")
}

func basenameIs(t *testing.T, rel *Release, name string) {
	t.Helper()
	base := path.Base(rel.URL)
	if base != name {
		t.Errorf("basename = %q, want %q", base, name)
	}
}

// TestSourceforgeAPI_LatestByVersion_Match tests rollback version matching.
func TestSourceforgeAPI_LatestByVersion_Match(t *testing.T) {
	rss := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>7z2401-x64.exe</title>
      <pubDate>Wed, 03 Jan 2024 00:00:00 UT</pubDate>
    </item>
    <item>
      <title>7z2400-x64.exe</title>
      <pubDate>Tue, 02 Jan 2024 00:00:00 UT</pubDate>
    </item>
    <item>
      <title>7z2300-x64.exe</title>
      <pubDate>Mon, 01 Jan 2024 00:00:00 UT</pubDate>
    </item>
  </channel>
</rss>`
	mdl := newMockDownloader()
	mdl.On("/projects/sevenzip/rss", &HTTPResponse{StatusCode: 200, Body: []byte(rss)})

	api := NewSourceforgeAPI(
		config.BasicConfig{ProjectName: "sevenzip"},
		config.DownloadConfig{
			Keyword:  config.SimpleKeywords("x64"),
			Filetype: config.Slice[string]{"exe"},
		},
		mdl,
		slogDiscard(),
	)
	rel, err := api.LatestByVersion(t.Context(), "Tue, 02 Jan 2024 00:00:00 UT")
	if err != nil {
		t.Fatalf("LatestByVersion() error = %v", err)
	}
	if rel.Version != "Tue, 02 Jan 2024 00:00:00 UT" {
		t.Errorf("Version = %q, want %q", rel.Version, "Tue, 02 Jan 2024 00:00:00 UT")
	}
	basenameIs(t, rel, "7z2400-x64.exe")
}

// TestSourceforgeAPI_LatestByVersion_NotFound tests that an error is returned
// when the target version is not found.
func TestSourceforgeAPI_LatestByVersion_NotFound(t *testing.T) {
	rss := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>7z2401-x64.exe</title>
      <pubDate>Wed, 03 Jan 2024 00:00:00 UT</pubDate>
    </item>
  </channel>
</rss>`
	mdl := newMockDownloader()
	mdl.On("/projects/sevenzip/rss", &HTTPResponse{StatusCode: 200, Body: []byte(rss)})

	api := NewSourceforgeAPI(
		config.BasicConfig{ProjectName: "sevenzip"},
		config.DownloadConfig{
			Keyword:  config.SimpleKeywords("x64"),
			Filetype: config.Slice[string]{"exe"},
		},
		mdl,
		slogDiscard(),
	)
	rel, err := api.LatestByVersion(t.Context(), "Fri, 05 Jan 2024 00:00:00 UT")
	if err == nil {
		t.Fatalf("LatestByVersion() expected error, got %v", rel)
	}
}
