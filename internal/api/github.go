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

// LatestByVersion finds a specific release by version string. It iterates
// through all releases, applying the same selectVersion logic (tag_name vs
// name) used by Latest, and returns the first match.
func (g *GitHubAPI) LatestByVersion(ctx context.Context, version string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", g.accountName, g.projectName)

	g.logger.Debug("github query (rollback)",
		"account", g.accountName,
		"project", g.projectName,
		"target_version", version,
		"reason", "fetch all releases to find target version",
		"result", url,
	)

	resp, err := g.downloader.Get(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("github releases: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github releases returned status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var releases []githubRelease
	if err := json.Unmarshal(resp.Body, &releases); err != nil {
		return nil, fmt.Errorf("parse github releases: %w", err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for %s/%s", g.accountName, g.projectName)
	}

	// Mirror Python: compute namesUnique from the full list, then apply
	// selectVersion consistently to each release — same logic as Latest.
	names := make([]string, len(releases))
	for i, r := range releases {
		names[i] = r.Name
	}
	namesUnique := len(names) == uniqueStrings(names)

	for i, rel := range releases {
		v := selectVersion(rel, namesUnique, len(releases))
		g.logger.Debug("github rollback check",
			"account", g.accountName,
			"project", g.projectName,
			"index", i,
			"tag_name", rel.TagName,
			"name", rel.Name,
			"computed_version", v,
			"target_version", version,
			"reason", "comparing computed version against target",
			"result", fmt.Sprintf("match=%v", v == version),
		)
		if v == version {
			g.logger.Info("rollback version found",
				"account", g.accountName,
				"project", g.projectName,
				"version", v,
				"tag_name", rel.TagName,
				"name", rel.Name,
				"reason", "target version matched during rollback scan",
				"result", v,
			)
			return g.buildRelease(rel, v), nil
		}
	}

	g.logger.Error("rollback version not found",
		"account", g.accountName,
		"project", g.projectName,
		"target_version", version,
		"reason", "no release matched the target version string",
		"result", "error",
	)
	return nil, fmt.Errorf("version %q not found in %s/%s releases", version, g.accountName, g.projectName)
}

func (g *GitHubAPI) Latest(ctx context.Context) (*Release, error) {
	var url string
	if g.noPull {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", g.accountName, g.projectName)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", g.accountName, g.projectName)
	}

	g.logger.Debug("github query",
		"account", g.accountName,
		"project", g.projectName,
		"no_pull", g.noPull,
		"reason", "no_pull selects /releases/latest endpoint, otherwise /releases list",
		"result", url,
	)

	resp, err := g.downloader.Get(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("github releases: %w", err)
	}
	if resp.StatusCode != 200 {
		g.logger.Error("github query failed",
			"account", g.accountName,
			"project", g.projectName,
			"status", resp.StatusCode,
			"reason", "github API returned non-200 status",
			"result", "error",
		)
		return nil, fmt.Errorf("github releases returned status %d: %s", resp.StatusCode, string(resp.Body))
	}

	if g.noPull {
		// Single release response from /releases/latest
		var rel githubRelease
		if err := json.Unmarshal(resp.Body, &rel); err != nil {
			return nil, fmt.Errorf("parse github release: %w", err)
		}
		// Mirror Python: for a single release, use name if non-empty,
		// otherwise fall back to tag_name.
		version := rel.Name
		if version == "" {
			version = rel.TagName
		}
		g.logger.Info("latest version detected",
			"account", g.accountName,
			"project", g.projectName,
			"version", version,
			"reason", "parsed single /releases/latest response",
			"result", version,
		)
		return g.buildRelease(rel, version), nil
	}

	var releases []githubRelease
	if err := json.Unmarshal(resp.Body, &releases); err != nil {
		return nil, fmt.Errorf("parse github releases: %w", err)
	}
	if len(releases) == 0 {
		g.logger.Error("no github releases",
			"account", g.accountName,
			"project", g.projectName,
			"reason", "releases list is empty",
			"result", "error",
		)
		return nil, fmt.Errorf("no releases found for %s/%s", g.accountName, g.projectName)
	}

	// Mirror Python: if all release names are identical, this is a single
	// tag with multiple builds (e.g. pre-release + release), so use tag_name.
	// If names are unique, use the release name which includes dates/labels.
	names := make([]string, len(releases))
	for i, r := range releases {
		names[i] = r.Name
	}
	namesUnique := len(names) == uniqueStrings(names)
	version := selectVersion(releases[0], namesUnique, len(releases))
	g.logger.Info("latest version detected",
		"account", g.accountName,
		"project", g.projectName,
		"version", version,
		"reason", "took first entry from releases list (names_unique="+fmt.Sprint(namesUnique)+")",
		"result", version,
	)
	return g.buildRelease(releases[0], version), nil
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

// selectVersion mirrors Python's logic: if there are multiple releases and
// their names are not all identical (i.e. at least two distinct names exist),
// use the release name as the version. Otherwise fall back to tag_name.
// Single releases always use tag_name (stable identifier).
// This handles the case where the same tag is rebuilt multiple times —
// all releases share the same name, so we use tag_name to avoid false
// version drift (e.g. "v0.38.0 - June 3rd, 2026" vs stored "v0.38.0").
func selectVersion(rel githubRelease, namesUnique bool, totalReleases int) string {
	if totalReleases > 1 && namesUnique && len(rel.Name) > 0 {
		return rel.Name
	}
	return rel.TagName
}

// uniqueStrings returns the number of distinct strings in s.
func uniqueStrings(s []string) int {
	seen := make(map[string]struct{}, len(s))
	for _, v := range s {
		seen[v] = struct{}{}
	}
	return len(seen)
}

func (g *GitHubAPI) buildRelease(rel githubRelease, version string) *Release {
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
