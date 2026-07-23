package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deorth-kku/updater-go/internal/config"
)

// SourceforgeAPI implements API for SourceForge RSS feeds.
type SourceforgeAPI struct {
	projectName string
	dlCfg       config.DownloadConfig
	downloader  Downloader
	version     string
	rssURL      string // override for testing; empty uses the default endpoint
	logger      *slog.Logger
}

// NewSourceforgeAPI creates a new SourceForge API adapter.
func NewSourceforgeAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, dl Downloader, logger *slog.Logger) *SourceforgeAPI {
	return &SourceforgeAPI{
		projectName: cfg.ProjectName,
		dlCfg:       dlCfg,
		downloader:  dl,
		logger:      logger,
	}
}

// rssFeedURL returns the RSS endpoint, using a test override when set.
func (s *SourceforgeAPI) rssFeedURL() string {
	if s.rssURL != "" {
		return s.rssURL
	}
	return fmt.Sprintf("https://sourceforge.net/projects/%s/rss?path=/", s.projectName)
}

// RSS item from SourceForge RSS feed.
type rssItem struct {
	Title   string `xml:"title"`
	PubDate string `xml:"pubDate"`
}

type rssChannel struct {
	Item []rssItem `xml:"item"`
}

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

func (s *SourceforgeAPI) Latest(ctx context.Context) (*Release, error) {
	rssURL := s.rssFeedURL()
	s.logger.Debug("sourceforge query",
		"project", s.projectName,
		"reason", "fetch project RSS feed",
		"result", rssURL,
	)
	resp, err := s.downloader.Get(ctx, rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sourceforge rss: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("parse sourceforge rss: %w", err)
	}

	// Mirrors updater-rpc's SourceforgeApi.getDlUrl: iterate RSS items,
	// apply filename_check(keyword, no_keyword, filetype) to each file title,
	// and pick the `index`-th match (default 0). The matched item's pubDate
	// is the version.
	keyword := s.dlCfg.Keyword
	noKeyword := s.dlCfg.ExcludeKeyword
	filetype := s.dlCfg.Filetype
	// The reference defaults filetype to ["7z"] when unset (see
	// SourceforgeApi.getDlUrl's filetype="7z" default).
	if len(filetype) == 0 {
		filetype = config.StringOrSlice{"7z"}
	}
	index := s.dlCfg.Index

	downloadPrefix := fmt.Sprintf("https://download.sourceforge.net/%s", s.projectName)

	match := 0
	for _, item := range feed.Channel.Item {
		// Parse date for sorting (most recent first in RSS)
		_, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", item.PubDate)
		if err != nil {
			// Try alternative format
			_, err = time.Parse("Mon, 02 Jan 2006 15:04:05 UT", item.PubDate)
			if err != nil {
				s.logger.Debug("sourceforge item skipped",
					"project", s.projectName,
					"title", item.Title,
					"reason", "pub_date did not match known formats",
					"result", "skip",
				)
				continue
			}
		}

		fileName := strings.TrimPrefix(item.Title, "/")
		if !sourceforgeFilenameCheck(fileName, keyword, noKeyword, filetype) {
			s.logger.Debug("sourceforge item rejected",
				"project", s.projectName,
				"file", fileName,
				"reason", "did not match keyword/exclude/filetype filter",
				"result", "skip",
			)
			continue
		}

		if match == index {
			s.version = item.PubDate
			dlURL := downloadPrefix + "/" + fileName
			s.logger.Info("latest version detected",
				"project", s.projectName,
				"version", item.PubDate,
				"file", fileName,
				"reason", "matched sourceforge item at index",
				"result", item.PubDate,
			)
			return &Release{
				URL:     dlURL,
				Version: item.PubDate,
				Assets: []Asset{
					{URL: dlURL, Name: fileName},
				},
			}, nil
		}
		match++
	}

	s.logger.Error("no sourceforge file found",
		"project", s.projectName,
		"reason", "RSS feed contained no items matching the filter",
		"result", "error",
	)
	return nil, fmt.Errorf("no files found in sourceforge rss for %s", s.projectName)
}

// sourceforgeFilenameCheck mirrors updater-rpc's FatherApi.filename_check:
// every keyword must be a substring; no exclude keyword may be present; and
// the filename must end with one of the filetype extensions.
func sourceforgeFilenameCheck(filename string, keywords, noKeywords []string, filetypes []string) bool {
	nameLower := strings.ToLower(filename)
	for _, k := range keywords {
		if !strings.Contains(nameLower, strings.ToLower(k)) {
			return false
		}
	}
	for _, nk := range noKeywords {
		if strings.Contains(nameLower, strings.ToLower(nk)) {
			return false
		}
	}
	for _, ft := range filetypes {
		if strings.HasSuffix(nameLower, strings.ToLower(ft)) {
			return true
		}
	}
	return false
}
