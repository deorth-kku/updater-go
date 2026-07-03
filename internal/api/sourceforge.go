package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/deorth-kku/updater-go/internal/config"
)

// SourceforgeAPI implements API for SourceForge RSS feeds.
type SourceforgeAPI struct {
	projectName string
	downloader  Downloader
	version     string
}

// NewSourceforgeAPI creates a new SourceForge API adapter.
func NewSourceforgeAPI(cfg config.BasicConfig, dl Downloader) *SourceforgeAPI {
	return &SourceforgeAPI{
		projectName: cfg.ProjectName,
		downloader:  dl,
	}
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
	rssURL := fmt.Sprintf("https://sourceforge.net/projects/%s/rss?path=/", s.projectName)
	resp, err := s.downloader.Get(ctx, rssURL)
	if err != nil {
		return nil, fmt.Errorf("sourceforge rss: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return nil, fmt.Errorf("parse sourceforge rss: %w", err)
	}

	downloadPrefix := fmt.Sprintf("https://download.sourceforge.net/%s", s.projectName)

	for _, item := range feed.Channel.Item {
		// Parse date for sorting (most recent first in RSS)
		_, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", item.PubDate)
		if err != nil {
			// Try alternative format
			_, err = time.Parse("Mon, 02 Jan 2006 15:04:05 UT", item.PubDate)
			if err != nil {
				continue
			}
		}

		s.version = item.PubDate
		fileName := strings.TrimPrefix(item.Title, "/")
		dlURL := downloadPrefix + "/" + fileName

		return &Release{
			URL:     dlURL,
			Version: item.PubDate,
			Assets: []Asset{
				{URL: dlURL, Name: fileName},
			},
		}, nil
	}

	return nil, fmt.Errorf("no files found in sourceforge rss for %s", s.projectName)
}
