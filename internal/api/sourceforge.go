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

// fetchAllItems fetches the RSS feed and returns all items.
func (s *SourceforgeAPI) fetchAllItems(ctx context.Context) ([]rssItem, error) {
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
	return feed.Channel.Item, nil
}

// buildReleases filters RSS items by filename criteria and builds Release
// entries for all matches.
func (s *SourceforgeAPI) buildReleases(items []rssItem) []*Release {
	keyword := s.dlCfg.Keyword
	noKeyword := s.dlCfg.ExcludeKeyword
	filetype := s.dlCfg.Filetype
	if len(filetype) == 0 {
		filetype = config.StringOrSlice{"7z"}
	}
	downloadPrefix := fmt.Sprintf("https://download.sourceforge.net/%s", s.projectName)

	var result []*Release
	matchIdx := 0
	for _, item := range items {
		// Parse date for sorting (most recent first in RSS)
		_, err := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", item.PubDate)
		if err != nil {
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

		dlURL := downloadPrefix + "/" + fileName
		rel := &Release{
			URL:     dlURL,
			Version: item.PubDate,
			Assets: []Asset{
				{URL: dlURL, Name: fileName},
			},
		}
		s.logger.Debug("sourceforge item listed",
			"project", s.projectName,
			"file", fileName,
			"version", item.PubDate,
			"reason", "added to list",
			"result", item.PubDate,
		)
		result = append(result, rel)
		matchIdx++
	}
	return result
}

// List returns all matching releases from Sourceforge RSS.
func (s *SourceforgeAPI) List(ctx context.Context) ([]*Release, error) {
	items, err := s.fetchAllItems(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildReleases(items), nil
}

// Latest returns the release at the configured index from List.
func (s *SourceforgeAPI) Latest(ctx context.Context) (*Release, error) {
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	downloadPrefix := fmt.Sprintf("https://download.sourceforge.net/%s", s.projectName)
	index := s.dlCfg.Index

	match := 0
	for _, item := range list {
		if match == index {
			s.version = item.Version
			dlURL := downloadPrefix + "/" + item.Assets[0].Name
			s.logger.Info("latest version detected",
				"project", s.projectName,
				"version", item.Version,
				"file", item.Assets[0].Name,
				"reason", "matched sourceforge item at index",
				"result", item.Version,
			)
			return &Release{
				URL:     dlURL,
				Version: item.Version,
				Assets: []Asset{
					{URL: dlURL, Name: item.Assets[0].Name},
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

// LatestByVersion finds a specific release by version (pubDate) string using List.
func (s *SourceforgeAPI) LatestByVersion(ctx context.Context, version string) (*Release, error) {
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	for i, rel := range list {
		s.logger.Debug("sourceforge rollback check",
			"project", s.projectName,
			"index", i,
			"item_version", rel.Version,
			"target_version", version,
			"reason", "comparing computed version against target",
			"result", fmt.Sprintf("match=%v", rel.Version == version),
		)
		if rel.Version == version {
			s.version = rel.Version
			dlURL := fmt.Sprintf("https://download.sourceforge.net/%s/%s", s.projectName, rel.Assets[0].Name)
			s.logger.Info("rollback version found",
				"project", s.projectName,
				"version", rel.Version,
				"file", rel.Assets[0].Name,
				"reason", "target version matched during rollback scan",
				"result", rel.Version,
			)
			return &Release{
				URL:     dlURL,
				Version: rel.Version,
				Assets: []Asset{
					{URL: dlURL, Name: rel.Assets[0].Name},
				},
			}, nil
		}

		s.logger.Debug("sourceforge rollback version mismatch",
			"project", s.projectName,
			"item_version", rel.Version,
			"target_version", version,
			"reason", "item pubDate does not match target version",
			"result", "skip",
		)
	}

	s.logger.Error("rollback version not found",
		"project", s.projectName,
		"target_version", version,
		"reason", "no RSS item matched the target version string",
		"result", "error",
	)
	return nil, fmt.Errorf("version %q not found in sourceforge RSS for %s", version, s.projectName)
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
