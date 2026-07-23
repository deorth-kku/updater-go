package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// SimpleSpiderAPI implements API for regex-based web scraping (e.g. go.dev/dl).
type SimpleSpiderAPI struct {
	pageURL    string
	dlCfg      config.DownloadConfig
	verCfg     config.VersionConfig
	headers    map[string]string
	downloader Downloader
	logger     *slog.Logger
}

// NewSimpleSpiderAPI creates a new SimpleSpider API adapter.
func NewSimpleSpiderAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, dl Downloader, logger *slog.Logger) *SimpleSpiderAPI {
	headers := make(map[string]string)
	maps.Copy(headers, defaultHeaders)
	maps.Copy(headers, cfg.Headers)

	return &SimpleSpiderAPI{
		pageURL:    cfg.PageURL,
		dlCfg:      dlCfg,
		verCfg:     verCfg,
		headers:    headers,
		downloader: dl,
		logger:     logger,
	}
}

var defaultHeaders = map[string]string{
	"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:23.0) Gecko/20100101 Firefox/23.0",
}

func (s *SimpleSpiderAPI) Latest(ctx context.Context) (*Release, error) {
	// If a direct URL is configured, use it without scraping
	if s.dlCfg.URL != "" {
		s.logger.Info("simplespider using direct url",
			"page", s.pageURL,
			"reason", "download.url configured, skip scraping",
			"result", s.dlCfg.URL,
		)
		// Even with a direct URL, fetch the page if version extraction
		// needs to match against page content (from_page=true).
		var err error
		var page string
		if s.verCfg.FromPage {
			page, err = s.fetchPage(ctx)
			if err != nil {
				return nil, err
			}
		}
		return s.buildFromDirectURL(ctx, s.dlCfg.URL, page)
	}

	// Fetch the page
	page, err := s.fetchPage(ctx)
	if err != nil {
		return nil, err
	}

	// Extract download URL from regexes
	dlURL, err := s.extractURLFromPage(ctx, page)
	if err != nil {
		return nil, err
	}
	s.logger.Debug("simplespider url extracted",
		"page", s.pageURL,
		"url", dlURL,
		"reason", "regexes matched a download url on the page",
		"result", dlURL,
	)

	// Extract version
	version, err := s.extractVersion(dlURL, page)
	if err != nil {
		return nil, err
	}
	s.logger.Info("latest version detected",
		"page", s.pageURL,
		"version", version,
		"reason", "version extracted from url or page",
		"result", version,
	)

	fileName := extractFilename(dlURL)
	if s.dlCfg.FilenameOverride != "" {
		name := s.dlCfg.FilenameOverride
		if s.dlCfg.AddVersionToFilename {
			name = strings.ReplaceAll(name, "{version}", version)
		}
		fileName = name
	}

	return &Release{
		URL:     dlURL,
		Version: version,
		Assets: []Asset{
			{URL: dlURL, Name: fileName},
		},
	}, nil
}

// LatestByVersion is not supported for SimpleSpider (no historical version list).
func (s *SimpleSpiderAPI) LatestByVersion(_ context.Context, _ string) (*Release, error) {
	return nil, fmt.Errorf("rollback not supported for simplespider backend")
}

func (s *SimpleSpiderAPI) buildFromDirectURL(ctx context.Context, dlURL string, page string) (*Release, error) {
	// Mirrors updater-rpc simplespider.getDlUrl: when download.data is
	// configured, POST it to the direct URL and use the 3xx Location header.
	// Note: try_redirect (HEAD-follow) is NOT applied to a direct URL in the
	// Python reference — only to the regex-chain branch.
	if len(s.dlCfg.Data) > 0 {
		located, err := postFormAndFollow(ctx, dlURL, s.dlCfg.Data)
		if err == nil {
			dlURL = located
		}
	}

	version, err := s.extractVersion(dlURL, page)
	if err != nil {
		return nil, err
	}

	fileName := extractFilename(dlURL)
	if s.dlCfg.FilenameOverride != "" {
		name := s.dlCfg.FilenameOverride
		if s.dlCfg.AddVersionToFilename {
			name = strings.ReplaceAll(name, "{version}", version)
		}
		fileName = name
	}

	return &Release{
		URL:     dlURL,
		Version: version,
		Assets: []Asset{
			{URL: dlURL, Name: fileName},
		},
	}, nil
}

func (s *SimpleSpiderAPI) fetchPage(ctx context.Context) (string, error) {
	// If Data is configured, use POST with JSON body
	if len(s.dlCfg.Data) > 0 {
		jsonBody, err := json.Marshal(s.dlCfg.Data)
		if err != nil {
			return "", fmt.Errorf("marshal data: %w", err)
		}
		resp, err := s.downloader.Post(ctx, s.pageURL, jsonBody, s.headers)
		if err != nil {
			return "", fmt.Errorf("simplespider fetch page: %w", err)
		}
		return string(resp.Body), nil
	}

	resp, err := s.downloader.Get(ctx, s.pageURL, s.headers)
	if err != nil {
		return "", fmt.Errorf("simplespider fetch page: %w", err)
	}
	return string(resp.Body), nil
}

// fetchURL performs a GET (with the configured headers) on an arbitrary URL
// and returns the response body as a string. Used to fetch the content of
// each resolved URL in a multi-level simplespider chain.
func (s *SimpleSpiderAPI) fetchURL(ctx context.Context, rawURL string) (string, error) {
	resp, err := s.downloader.Get(ctx, rawURL, s.headers)
	if err != nil {
		return "", fmt.Errorf("simplespider fetch %s: %w", rawURL, err)
	}
	return string(resp.Body), nil
}

func (s *SimpleSpiderAPI) extractURLFromPage(ctx context.Context, page string) (string, error) {
	if len(s.dlCfg.Regexes) == 0 {
		return "", fmt.Errorf("no regexes configured for simplespider")
	}

	// Per-level index selection, mirroring updater-rpc's indexes[lv].
	// Missing entries default to 0 (first match).
	indexes := s.dlCfg.Indexes
	if len(indexes) == 0 {
		indexes = make([]int, len(s.dlCfg.Regexes))
	}

	currentURL := s.pageURL
	// Level 0 applies its regex to the fetched page; each subsequent level
	// applies its regex to the (fetched) content of the previously resolved
	// URL, matching updater-rpc's page_regex_url behavior.
	source := page
	for i, pattern := range s.dlCfg.Regexes {
		idx := 0
		if i < len(indexes) {
			idx = indexes[i]
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("compile regex %d: %w", i, err)
		}

		// Apply regex to the level's source HTML, mirroring re.findall(...)[index].
		matches := re.FindAllStringSubmatch(source, -1)
		if len(matches) == 0 || idx >= len(matches) {
			return "", fmt.Errorf("regex %d did not match (index %d): %s", i, idx, pattern)
		}
		grp := matches[idx]
		rawURL := grp[0]
		if len(grp) > 1 {
			rawURL = grp[1]
		}

		rawURL = unescapeHTML(rawURL)

		// Resolve relative URLs
		if strings.HasPrefix(rawURL, "/") {
			site := siteName(currentURL)
			rawURL = site + rawURL
		} else if !strings.HasPrefix(rawURL, "http") {
			rawURL = joinURL(currentURL, rawURL)
		}

		currentURL = rawURL
		if i < len(s.dlCfg.Regexes)-1 {
			fetched, ferr := s.fetchURL(ctx, currentURL)
			if ferr != nil {
				return "", fmt.Errorf("fetch level %d url %s: %w", i+1, currentURL, ferr)
			}
			source = fetched
		}
	}

	dlURL := currentURL

	// Follow redirect if configured
	if s.dlCfg.TryRedirect {
		redirURL, err := followRedirect(context.Background(), dlURL)
		if err == nil {
			dlURL = redirURL
		}
	}

	return dlURL, nil
}

func (s *SimpleSpiderAPI) extractVersion(dlURL, page string) (string, error) {
	// Mirrors updater-rpc's getVersion(regex, from_page, index): when
	// from_page is set the regex is applied to the page text, otherwise to
	// the download filename. version.index selects which regex match to use.
	if s.verCfg.Regex == "" {
		return "", fmt.Errorf("no version regex configured")
	}
	source := extractFilename(dlURL)
	if s.verCfg.FromPage {
		source = page
	}
	re, err := regexp.Compile(s.verCfg.Regex)
	if err != nil {
		return "", fmt.Errorf("compile version regex: %w", err)
	}
	matches := re.FindAllStringSubmatch(source, -1)
	idx := s.verCfg.Index
	if idx < len(matches) {
		grp := matches[idx]
		if len(grp) > 1 && grp[1] != "" {
			return grp[1], nil
		}
		return grp[0], nil
	}
	return "", fmt.Errorf("cannot find version at position %d", idx)
}

// --- helpers ---

func followRedirect(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 302 || resp.StatusCode == 303 {
		return resp.Header.Get("Location"), nil
	}
	return rawURL, nil
}

// postFormAndFollow POSTs form-encoded data to rawURL and, if the server
// responds with a 302/303, returns the Location header. This mirrors
// updater-rpc's simplespider.getDlUrl behaviour for direct URLs with data.
func postFormAndFollow(ctx context.Context, rawURL string, data map[string]any) (string, error) {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, fmt.Sprintf("%v", v))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 302 || resp.StatusCode == 303 {
		return resp.Header.Get("Location"), nil
	}
	return "", fmt.Errorf("download.data POST returned status %d (no redirect)", resp.StatusCode)
}

func extractFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	path := u.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	return path
}

func siteName(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func joinURL(base, rel string) string {
	if base == "" {
		return rel
	}
	u, err := url.Parse(base)
	if err != nil {
		return rel
	}
	if strings.HasPrefix(rel, "/") {
		// Absolute path: replace the base path entirely
		u.Path = rel
	} else {
		u.Path = strings.TrimRight(u.Path, "/") + "/" + rel
	}
	return u.String()
}

func unescapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}
