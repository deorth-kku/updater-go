package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
}

// NewSimpleSpiderAPI creates a new SimpleSpider API adapter.
func NewSimpleSpiderAPI(cfg config.BasicConfig, dlCfg config.DownloadConfig, verCfg config.VersionConfig, dl Downloader) *SimpleSpiderAPI {
	headers := make(map[string]string)
	maps.Copy(headers, defaultHeaders)
	maps.Copy(headers, cfg.Headers)

	return &SimpleSpiderAPI{
		pageURL:    cfg.PageURL,
		dlCfg:      dlCfg,
		verCfg:     verCfg,
		headers:    headers,
		downloader: dl,
	}
}

var defaultHeaders = map[string]string{
	"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64; rv:23.0) Gecko/20100101 Firefox/23.0",
}

func (s *SimpleSpiderAPI) Latest(ctx context.Context) (*Release, error) {
	// If a direct URL is configured, use it without scraping
	if s.dlCfg.URL != "" {
		slog.Default().Info("simplespider using direct url",
			"step", "api.simplespider.latest",
			"page", s.pageURL,
			"reason", "download.url configured, skip scraping",
			"result", s.dlCfg.URL,
		)
		return s.buildFromDirectURL(ctx, s.dlCfg.URL)
	}

	// Fetch the page
	page, err := s.fetchPage(ctx)
	if err != nil {
		return nil, err
	}

	// Extract download URL from regexes
	dlURL, err := s.extractURLFromPage(page)
	if err != nil {
		return nil, err
	}
	slog.Default().Debug("simplespider url extracted",
		"step", "api.simplespider.latest",
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
	slog.Default().Info("latest version detected",
		"step", "api.simplespider.latest",
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

func (s *SimpleSpiderAPI) buildFromDirectURL(ctx context.Context, dlURL string) (*Release, error) {
	if s.dlCfg.TryRedirect {
		redirURL, err := followRedirect(ctx, dlURL)
		if err == nil {
			dlURL = redirURL
		}
	}

	version := "unknown"
	if s.verCfg.Regex != "" {
		re, err := regexp.Compile(s.verCfg.Regex)
		if err == nil {
			if matches := re.FindStringSubmatch(extractFilename(dlURL)); len(matches) > 1 {
				version = matches[1]
			}
		}
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
	var bodyReader io.Reader

	// If Data is configured, use POST with JSON body
	if len(s.dlCfg.Data) > 0 {
		jsonBody, err := json.Marshal(s.dlCfg.Data)
		if err != nil {
			return "", fmt.Errorf("marshal data: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	var req *http.Request
	var reqErr error
	if bodyReader != nil {
		req, reqErr = http.NewRequestWithContext(ctx, http.MethodPost, s.pageURL, bodyReader)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, reqErr = http.NewRequestWithContext(ctx, http.MethodGet, s.pageURL, nil)
		if reqErr != nil {
			return "", reqErr
		}
	}

	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("simplespider fetch page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *SimpleSpiderAPI) extractURLFromPage(page string) (string, error) {
	if len(s.dlCfg.Regexes) == 0 {
		return "", fmt.Errorf("no regexes configured for simplespider")
	}

	currentURL := s.pageURL
	for i, pattern := range s.dlCfg.Regexes {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("compile regex %d: %w", i, err)
		}

		// Apply regex to the page HTML, not the URL
		matches := re.FindStringSubmatch(page)
		if len(matches) < 2 {
			return "", fmt.Errorf("regex %d did not match: %s", i, pattern)
		}

		rawURL := unescapeHTML(matches[1])

		// Resolve relative URLs
		if strings.HasPrefix(rawURL, "/") {
			site := siteName(currentURL)
			rawURL = site + rawURL
		} else if !strings.HasPrefix(rawURL, "http") {
			rawURL = joinURL(currentURL, rawURL)
		}

		currentURL = rawURL
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
	if s.verCfg.FromPage && s.verCfg.Regex != "" {
		re, err := regexp.Compile(s.verCfg.Regex)
		if err != nil {
			return "", fmt.Errorf("compile version regex: %w", err)
		}
		if matches := re.FindStringSubmatch(page); len(matches) > 1 {
			return matches[1], nil
		}
	}

	if s.verCfg.Regex != "" {
		re, err := regexp.Compile(s.verCfg.Regex)
		if err != nil {
			return "", fmt.Errorf("compile version regex: %w", err)
		}
		fileName := extractFilename(dlURL)
		if matches := re.FindStringSubmatch(fileName); len(matches) > 1 {
			return matches[1], nil
		}
	}

	return extractFilename(dlURL), nil
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
