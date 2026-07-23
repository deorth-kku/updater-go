package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"
	"time"

	"github.com/deorth-kku/updater-go/internal/config"
)

// AppveyorAPI implements API for AppVeyor CI builds.
type AppveyorAPI struct {
	accountName string
	projectName string
	branch      string
	noPull      bool
	downloader  Downloader
	logger      *slog.Logger
}

// NewAppveyorAPI creates a new AppVeyor API adapter.
func NewAppveyorAPI(cfg config.BasicConfig, dl Downloader, logger *slog.Logger) *AppveyorAPI {
	return &AppveyorAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		branch:      "",
		noPull:      false,
		downloader:  dl,
		logger:      logger,
	}
}

// SetNoPull sets whether to skip PR-triggered builds.
func (a *AppveyorAPI) SetNoPull(noPull bool) {
	a.noPull = noPull
}

// SetBranch sets the build branch for filtering.
func (a *AppveyorAPI) SetBranch(branch string) {
	a.branch = branch
}

// fetchAllBuilds fetches the build history from AppVeyor.
func (a *AppveyorAPI) fetchAllBuilds(ctx context.Context) (appveyorHistory, error) {
	baseURL := "https://ci.appveyor.com/api"
	branchParam := ""
	if a.branch != "" {
		branchParam = "&branch=" + a.branch
	}

	historyURL := fmt.Sprintf("%s/projects/%s/%s/history?recordsNumber=100%s",
		baseURL, a.accountName, a.projectName, branchParam)

	a.logger.Debug("appveyor query",
		"account", a.accountName,
		"project", a.projectName,
		"branch", a.branch,
		"reason", "fetch build history (optionally filtered by branch)",
		"result", historyURL,
	)

	resp, err := a.downloader.Get(ctx, historyURL, nil)
	if err != nil {
		return appveyorHistory{}, fmt.Errorf("appveyor history: %w", err)
	}

	var history appveyorHistory
	if err := json.Unmarshal(resp.Body, &history); err != nil {
		return appveyorHistory{}, fmt.Errorf("parse appveyor history: %w", err)
	}
	return history, nil
}

func (a *AppveyorAPI) filterBuilds(builds []appveyorBuild) iter.Seq2[int, appveyorBuild] {
	return func(yield func(int, appveyorBuild) bool) {
		for i, build := range builds {
			if a.noPull && build.PullRequestID != "" {
				a.logger.Debug("appveyor build skipped",
					"account", a.accountName,
					"project", a.projectName,
					"version", build.Version,
					"reason", "build is PR-triggered and no_pull is enabled, excluded",
					"result", "skip",
				)
				continue
			}
			if !yield(i, build) {
				return
			}
		}
	}
}

// buildReleases fetches detail and artifacts for each non-PR build and
// returns a *Release for each one. Builds without artifacts older than 30
// days are skipped.
func (a *AppveyorAPI) buildReleases(ctx context.Context, history appveyorHistory) []*Release {
	var result []*Release

	for _, build := range a.filterBuilds(history.Builds) {
		rel, err := a.fetchBuildDetail(ctx, build)
		if err != nil {
			a.logger.Debug("appveyor build skipped",
				"account", a.accountName,
				"project", a.projectName,
				"version", build.Version,
				"reason", fmt.Sprintf("fetchBuildDetail failed: %v", err),
				"result", "skip",
			)
			continue
		}

		a.logger.Debug("appveyor build listed",
			"account", a.accountName,
			"project", a.projectName,
			"version", rel.Version,
			"job_id", rel.JobID,
			"artifacts", len(rel.Artifacts),
			"reason", "added to list",
			"result", rel.Version,
		)
		result = append(result, rel)
	}
	return result
}

var errHttpFailed = errors.New("appveyor build detail http failed: ")

// fetchBuildDetail fetches detail and artifacts for a single build.
// Returns a *Release or an error if the build has no artifacts.
func (a *AppveyorAPI) fetchBuildDetail(ctx context.Context, build appveyorBuild) (*Release, error) {
	baseURL := "https://ci.appveyor.com/api"
	version := build.Version

	buildURL := fmt.Sprintf("%s/projects/%s/%s/build/%s",
		baseURL, a.accountName, a.projectName, version)
	buildResp, err := a.downloader.Get(ctx, buildURL, nil)
	if err != nil {
		return nil, errors.Join(errHttpFailed, err)
	}

	var buildDetail appveyorBuildDetail
	if err := json.Unmarshal(buildResp.Body, &buildDetail); err != nil {
		return nil, fmt.Errorf("parse appveyor build detail: %w", err)
	}

	jobID := findJobID(buildDetail.Build.Jobs)
	if jobID == "" {
		return nil, fmt.Errorf("no suitable job id found for build %s", version)
	}

	artifactsURL := fmt.Sprintf("%s/buildjobs/%s/artifacts", baseURL, jobID)
	artResp, err := a.downloader.Get(ctx, artifactsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("appveyor artifacts: %w", err)
	}

	var artifacts []AppveyorArtifact
	if err := json.Unmarshal(artResp.Body, &artifacts); err != nil {
		return nil, fmt.Errorf("parse appveyor artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		updated := buildDetail.Build.Updated
		if updated != "" {
			dt, err := time.Parse("2006-01-02T15:04:05", updated)
			if err == nil && time.Since(dt) > 30*24*time.Hour {
				return nil, fmt.Errorf("build %s has no artifacts and is older than 30 days", version)
			}
		}
		return nil, fmt.Errorf("build %s has no artifacts", version)
	}

	return &Release{
		Version:   version,
		Artifacts: artifacts,
		JobID:     jobID,
		BaseURL:   baseURL,
	}, nil
}

// List returns all builds from AppVeyor.
func (a *AppveyorAPI) List(ctx context.Context) ([]*Release, error) {
	history, err := a.fetchAllBuilds(ctx)
	if err != nil {
		return nil, err
	}
	return a.buildReleases(ctx, history), nil
}

// Latest returns the first non-PR build with artifacts.
func (a *AppveyorAPI) Latest(ctx context.Context) (*Release, error) {
	history, err := a.fetchAllBuilds(ctx)
	if err != nil {
		return nil, err
	}

	for _, build := range a.filterBuilds(history.Builds) {
		rel, err := a.fetchBuildDetail(ctx, build)
		if errors.Is(err, errHttpFailed) {
			return nil, err
		} else if err != nil {
			a.logger.Debug("appveyor build skipped",
				"account", a.accountName,
				"project", a.projectName,
				"version", build.Version,
				"reason", err,
				"result", "skip",
			)
			continue
		}
		a.logger.Info("latest version detected",
			"account", a.accountName,
			"project", a.projectName,
			"version", rel.Version,
			"job_id", rel.JobID,
			"artifacts", len(rel.Artifacts),
			"reason", "first non-PR build with artifacts found",
			"result", rel.Version,
		)
		return rel, nil
	}

	a.logger.Error("no appveyor build found",
		"account", a.accountName,
		"project", a.projectName,
		"reason", "no build with artifacts found after filtering",
		"result", "error",
	)
	return nil, fmt.Errorf("no suitable build found for %s/%s", a.accountName, a.projectName)
}

// LatestByVersion finds a specific build by version string.
// Only fetches artifacts for the matching build.
func (a *AppveyorAPI) LatestByVersion(ctx context.Context, version string) (*Release, error) {
	history, err := a.fetchAllBuilds(ctx)
	if err != nil {
		return nil, err
	}

	for i, build := range a.filterBuilds(history.Builds) {
		a.logger.Debug("appveyor rollback check",
			"account", a.accountName,
			"project", a.projectName,
			"index", i,
			"build_version", build.Version,
			"target_version", version,
			"reason", "comparing computed version against target",
			"result", fmt.Sprintf("match=%v", build.Version == version),
		)

		if build.Version == version {
			rel, err := a.fetchBuildDetail(ctx, build)
			if err != nil {
				return nil, err
			}
			a.logger.Info("rollback version detected",
				"account", a.accountName,
				"project", a.projectName,
				"version", rel.Version,
				"job_id", rel.JobID,
				"artifacts", len(rel.Artifacts),
				"reason", "target version matched during rollback scan",
				"result", rel.Version,
			)
			return rel, nil
		}
	}

	a.logger.Error("rollback version not found",
		"account", a.accountName,
		"project", a.projectName,
		"target_version", version,
		"reason", "no build matched the target version string",
		"result", "error",
	)
	return nil, fmt.Errorf("version %q not found in appveyor history for %s/%s", version, a.accountName, a.projectName)
}

// findJobID selects the best job ID from a build's job list.
// Prefers jobs with "release" in the name when multiple jobs exist.
func findJobID(jobs []appveyorJob) string {
	if len(jobs) > 1 {
		for _, job := range jobs {
			if strings.Contains(strings.ToLower(job.Name), "release") {
				return job.ID
			}
		}
	}
	if len(jobs) == 1 {
		return jobs[0].ID
	}
	return ""
}

// --- AppVeyor API types ---

type appveyorBuild struct {
	Version       string `json:"version"`
	PullRequestID string `json:"pullRequestId"`
}

type appveyorHistory struct {
	Builds []appveyorBuild `json:"builds"`
}

type appveyorBuildDetail struct {
	Build struct {
		Jobs    []appveyorJob `json:"jobs"`
		Updated string        `json:"updated"`
	} `json:"build"`
}

type appveyorJob struct {
	Name string `json:"name"`
	ID   string `json:"jobId"`
}

type AppveyorArtifact struct {
	FileName string `json:"fileName"`
}
