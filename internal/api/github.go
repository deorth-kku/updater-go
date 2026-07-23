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
	noPre       bool
	downloader  Downloader
	logger      *slog.Logger
}

// NewGitHubAPI creates a new GitHub API adapter.
func NewGitHubAPI(cfg config.BasicConfig, dl Downloader, logger *slog.Logger) *GitHubAPI {
	return &GitHubAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		noPre:       false,
		downloader:  dl,
		logger:      logger,
	}
}

func (g *GitHubAPI) SetNoPreRelease(noPull bool) {
	g.noPre = noPull
}

// fetchAllReleases fetches the full list of releases from GitHub.
func (g *GitHubAPI) fetchAllReleases(ctx context.Context) ([]githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", g.accountName, g.projectName)

	g.logger.Debug("github query",
		"account", g.accountName,
		"project", g.projectName,
		"reason", "fetch all releases list",
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
	return releases, nil
}

// buildReleases builds *Release slices from raw githubRelease entries,
// applying the selectVersion logic (tag_name vs name) consistently.
func (g *GitHubAPI) buildReleases(releases []githubRelease) []*Release {
	// Mirror Python: if all release names are identical, this is a single
	// tag with multiple builds (e.g. pre-release + release), so use tag_name.
	// If names are unique, use the release name which includes dates/labels.
	names := make([]string, len(releases))
	for i, r := range releases {
		names[i] = r.Name
	}
	namesUnique := len(names) == uniqueStrings(names)

	result := make([]*Release, 0, len(releases))
	for i, rel := range releases {
		// When noPull is enabled, skip pre-releases.
		if g.noPre && rel.Prerelease {
			g.logger.Debug("github pre-release skipped (noPull)",
				"account", g.accountName,
				"project", g.projectName,
				"tag_name", rel.TagName,
				"name", rel.Name,
				"reason", "noPull mode excludes pre-releases",
				"result", "skip",
			)
			continue
		}
		v := selectVersion(rel, namesUnique, len(releases))
		g.logger.Debug("github list entry",
			"account", g.accountName,
			"project", g.projectName,
			"index", i,
			"tag_name", rel.TagName,
			"name", rel.Name,
			"prerelease", rel.Prerelease,
			"computed_version", v,
			"reason", "building release entry from list",
			"result", v,
		)
		result = append(result, g.buildRelease(rel, v))
	}
	return result
}

// List returns all releases from GitHub.
func (g *GitHubAPI) List(ctx context.Context) ([]*Release, error) {
	releases, err := g.fetchAllReleases(ctx)
	if err != nil {
		return nil, err
	}
	return g.buildReleases(releases), nil
}

// Latest returns the first release from List.
func (g *GitHubAPI) Latest(ctx context.Context) (*Release, error) {
	list, err := g.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		g.logger.Error("no github releases",
			"account", g.accountName,
			"project", g.projectName,
			"reason", "List returned empty",
			"result", "error",
		)
		return nil, fmt.Errorf("no releases found for %s/%s", g.accountName, g.projectName)
	}
	g.logger.Info("latest version detected",
		"account", g.accountName,
		"project", g.projectName,
		"version", list[0].Version,
		"reason", "took first entry from List",
		"result", list[0].Version,
	)
	return list[0], nil
}

// LatestByVersion finds a specific release by version string using List.
func (g *GitHubAPI) LatestByVersion(ctx context.Context, version string) (*Release, error) {
	list, err := g.List(ctx)
	if err != nil {
		return nil, err
	}

	for i, rel := range list {
		g.logger.Debug("github rollback check",
			"account", g.accountName,
			"project", g.projectName,
			"index", i,
			"computed_version", rel.Version,
			"target_version", version,
			"reason", "comparing computed version against target",
			"result", fmt.Sprintf("match=%v", rel.Version == version),
		)
		if rel.Version == version {
			g.logger.Info("rollback version found",
				"account", g.accountName,
				"project", g.projectName,
				"version", rel.Version,
				"reason", "target version matched during rollback scan",
				"result", rel.Version,
			)
			return rel, nil
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

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Name       string        `json:"name"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
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
