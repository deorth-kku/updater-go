package api

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

// slogDiscard returns a slog.Logger that discards all output.
func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestSourceforge_FilenameCheck verifies the filename_check filter (gap #15).
func TestSourceforge_FilenameCheck(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		keywords  []string
		noKeyword []string
		filetypes []string
		want      bool
	}{
		{"match exe", "7z2400-x64.exe", []string{"x64"}, nil, []string{"exe"}, true},
		{"missing keyword", "7z2400.exe", []string{"x64"}, nil, []string{"exe"}, false},
		{"exclude keyword", "7z2400-beta.exe", []string{"x64"}, []string{"beta"}, []string{"exe"}, false},
		{"filetype mismatch default7z", "7z2400-x64.exe", []string{"x64"}, nil, []string{"7z"}, false},
		{"no keyword filetype only", "7z2400.7z", nil, nil, []string{"7z"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceforgeFilenameCheck(tt.filename, tt.keywords, tt.noKeyword, tt.filetypes)
			if got != tt.want {
				t.Errorf("sourceforgeFilenameCheck(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
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

	api := &SourceforgeAPI{
		projectName: "sevenzip",
		rssURL:      "https://sourceforge.net/projects/sevenzip/rss?path=/",
		dlCfg: config.DownloadConfig{
			Keyword:  config.StringOrSlice{"x64"},
			Filetype: config.StringOrSlice{"exe"},
			Index:    0,
		},
		downloader: mdl,
		logger:     slogDiscard(),
	}
	rel, err := api.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Assets[0].Name != "7z2401-x64.exe" {
		t.Errorf("Asset.Name = %q, want %q", rel.Assets[0].Name, "7z2401-x64.exe")
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

	api := &SourceforgeAPI{
		projectName: "sevenzip",
		rssURL:      "https://sourceforge.net/projects/sevenzip/rss?path=/",
		dlCfg: config.DownloadConfig{
			Keyword:  config.StringOrSlice{"x64"},
			Filetype: config.StringOrSlice{"exe"},
		},
		downloader: mdl,
		logger:     slogDiscard(),
	}
	rel, err := api.LatestByVersion(context.Background(), "Tue, 02 Jan 2024 00:00:00 UT")
	if err != nil {
		t.Fatalf("LatestByVersion() error = %v", err)
	}
	if rel.Version != "Tue, 02 Jan 2024 00:00:00 UT" {
		t.Errorf("Version = %q, want %q", rel.Version, "Tue, 02 Jan 2024 00:00:00 UT")
	}
	if rel.Assets[0].Name != "7z2400-x64.exe" {
		t.Errorf("Asset.Name = %q, want %q", rel.Assets[0].Name, "7z2400-x64.exe")
	}
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

	api := &SourceforgeAPI{
		projectName: "sevenzip",
		rssURL:      "https://sourceforge.net/projects/sevenzip/rss?path=/",
		dlCfg: config.DownloadConfig{
			Keyword:  config.StringOrSlice{"x64"},
			Filetype: config.StringOrSlice{"exe"},
		},
		downloader: mdl,
		logger:     slogDiscard(),
	}
	rel, err := api.LatestByVersion(context.Background(), "Fri, 05 Jan 2024 00:00:00 UT")
	if err == nil {
		t.Fatalf("LatestByVersion() expected error, got %v", rel)
	}
}
