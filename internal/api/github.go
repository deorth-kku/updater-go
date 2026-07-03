package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// GitHubAPI implements API for GitHub Releases.
type GitHubAPI struct {
	accountName string
	projectName string
	downloader  Downloader
}

// NewGitHubAPI creates a new GitHub API adapter.
func NewGitHubAPI(cfg config.BasicConfig, dl Downloader) *GitHubAPI {
	return &GitHubAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		downloader:  dl,
	}
}

func (g *GitHubAPI) Latest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", g.accountName, g.projectName)
	resp, err := g.downloader.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("github releases: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github releases returned status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var releases []githubRelease
	if err := unmarshalJSON(resp.Body, &releases); err != nil {
		return nil, fmt.Errorf("parse github releases: %w", err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for %s/%s", g.accountName, g.projectName)
	}

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
