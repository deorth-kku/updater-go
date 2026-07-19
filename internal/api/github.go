package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// GitHubAPI implements API for GitHub Releases.
type GitHubAPI struct {
	accountName string
	projectName string
	noPull      bool
	downloader  Downloader
	logger      *slog.Logger
}

// NewGitHubAPI creates a new GitHub API adapter.
func NewGitHubAPI(cfg config.BasicConfig, dl Downloader, logger *slog.Logger) *GitHubAPI {
	return &GitHubAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		noPull:      false,
		downloader:  dl,
		logger:      logger,
	}
}

// SetNoPull enables no-pull mode (uses /releases/latest instead of /releases).
func (g *GitHubAPI) SetNoPull(noPull bool) {
	g.noPull = noPull
}

func (g *GitHubAPI) Latest(ctx context.Context) (*Release, error) {
	var url string
	if g.noPull {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", g.accountName, g.projectName)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", g.accountName, g.projectName)
	}

	g.logger.Debug("github query",
		"step", "api.github.latest",
		"account", g.accountName,
		"project", g.projectName,
		"no_pull", g.noPull,
		"reason", "no_pull selects /releases/latest endpoint, otherwise /releases list",
		"result", url,
	)

	resp, err := g.downloader.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("github releases: %w", err)
	}
	if resp.StatusCode != 200 {
		g.logger.Error("github query failed",
			"step", "api.github.latest",
			"account", g.accountName,
			"project", g.projectName,
			"status", resp.StatusCode,
			"reason", "github API returned non-200 status",
			"result", "error",
		)
		return nil, fmt.Errorf("github releases returned status %d: %s", resp.StatusCode, string(resp.Body))
	}

	if g.noPull {
		// Single release response
		var rel githubRelease
		if err := json.Unmarshal(resp.Body, &rel); err != nil {
			return nil, fmt.Errorf("parse github release: %w", err)
		}
		g.logger.Info("latest version detected",
			"step", "api.github.latest",
			"account", g.accountName,
			"project", g.projectName,
			"version", rel.TagName,
			"reason", "parsed single /releases/latest response",
			"result", rel.TagName,
		)
		return g.buildRelease(rel), nil
	}

	var releases []githubRelease
	if err := json.Unmarshal(resp.Body, &releases); err != nil {
		return nil, fmt.Errorf("parse github releases: %w", err)
	}
	if len(releases) == 0 {
		g.logger.Error("no github releases",
			"step", "api.github.latest",
			"account", g.accountName,
			"project", g.projectName,
			"reason", "releases list is empty",
			"result", "error",
		)
		return nil, fmt.Errorf("no releases found for %s/%s", g.accountName, g.projectName)
	}

	g.logger.Info("latest version detected",
		"step", "api.github.latest",
		"account", g.accountName,
		"project", g.projectName,
		"version", releases[0].TagName,
		"reason", "took first entry from releases list",
		"result", releases[0].TagName,
	)
	return g.buildRelease(releases[0]), nil
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (g *GitHubAPI) buildRelease(rel githubRelease) *Release {
	version := rel.TagName

	r := &Release{Version: version}
	for _, a := range rel.Assets {
		r.Assets = append(r.Assets, Asset{
			URL:  a.BrowserDownloadURL,
			Name: a.Name,
		})
	}
	return r
}

// FilterAssets filters release assets by keywords, exclude keywords, and filetype.
func FilterAssets(assets []Asset, keywords []string, excludeKeywords []string, filetype string) []Asset {
	var result []Asset
	for _, a := range assets {
		if matchesAsset(a.Name, keywords, excludeKeywords, filetype) {
			result = append(result, a)
		}
	}
	return result
}

// matchesAsset checks if a filename matches the keyword/exclude/filetype criteria.
func matchesAsset(name string, keywords, excludeKeywords []string, filetype string) bool {
	if filetype != "" {
		ext := "." + strings.TrimPrefix(filetype, ".")
		if !strings.HasSuffix(strings.ToLower(name), ext) {
			return false
		}
	}
	for _, ek := range excludeKeywords {
		if strings.Contains(strings.ToLower(name), strings.ToLower(ek)) {
			return false
		}
	}
	for _, k := range keywords {
		if !strings.Contains(strings.ToLower(name), strings.ToLower(k)) {
			return false
		}
	}
	return true
}
