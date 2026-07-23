package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	downloader  Downloader
	logger      *slog.Logger
}

// NewAppveyorAPI creates a new AppVeyor API adapter.
func NewAppveyorAPI(cfg config.BasicConfig, dl Downloader, logger *slog.Logger) *AppveyorAPI {
	return &AppveyorAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		branch:      "",
		downloader:  dl,
		logger:      logger,
	}
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

// buildReleases fetches detail and artifacts for each non-PR build and
// returns a *Release for each one. Builds without artifacts older than 30
// days are skipped.
func (a *AppveyorAPI) buildReleases(history appveyorHistory) []*Release {
	baseURL := "https://ci.appveyor.com/api"
	var result []*Release

	for _, build := range history.Builds {
		if build.PullRequestID != "" {
			a.logger.Debug("appveyor build skipped",
				"account", a.accountName,
				"project", a.projectName,
				"version", build.Version,
				"reason", "build is PR-triggered, excluded",
				"result", "skip",
			)
			continue
		}

		version := build.Version
		buildURL := fmt.Sprintf("%s/projects/%s/%s/build/%s",
			baseURL, a.accountName, a.projectName, version)
		buildResp, err := a.downloader.Get(context.Background(), buildURL, nil)
		if err != nil {
			continue
		}

		var buildDetail appveyorBuildDetail
		if err := json.Unmarshal(buildResp.Body, &buildDetail); err != nil {
			continue
		}

		jobID := findJobID(buildDetail.Build.Jobs)
		if jobID == "" {
			a.logger.Debug("appveyor build skipped",
				"account", a.accountName,
				"project", a.projectName,
				"version", version,
				"reason", "no suitable job id found in build",
				"result", "skip",
			)
			continue
		}

		artifactsURL := fmt.Sprintf("%s/buildjobs/%s/artifacts", baseURL, jobID)
		artResp, err := a.downloader.Get(context.Background(), artifactsURL, nil)
		if err != nil {
			continue
		}

		var artifacts []AppveyorArtifact
		if err := json.Unmarshal(artResp.Body, &artifacts); err != nil {
			continue
		}

		if len(artifacts) == 0 {
			updated := buildDetail.Build.Updated
			if updated != "" {
				dt, err := time.Parse("2006-01-02T15:04:05", updated)
				if err == nil && time.Since(dt) > 30*24*time.Hour {
					a.logger.Debug("appveyor build skipped",
						"account", a.accountName,
						"project", a.projectName,
						"version", version,
						"reason", "no artifacts and build older than 30 days",
						"result", "skip",
					)
					continue
				}
			}
			a.logger.Debug("appveyor build skipped",
				"account", a.accountName,
				"project", a.projectName,
				"version", version,
				"reason", "no artifacts and no/old timestamp",
				"result", "skip",
			)
			continue
		}

		rel := &Release{
			Version:   version,
			Artifacts: artifacts,
			JobID:     jobID,
			BaseURL:   baseURL,
		}
		a.logger.Debug("appveyor build listed",
			"account", a.accountName,
			"project", a.projectName,
			"version", version,
			"job_id", jobID,
			"artifacts", len(artifacts),
			"reason", "added to list",
			"result", version,
		)
		result = append(result, rel)
	}
	return result
}

// List returns all builds from AppVeyor.
func (a *AppveyorAPI) List(ctx context.Context) ([]*Release, error) {
	history, err := a.fetchAllBuilds(ctx)
	if err != nil {
		return nil, err
	}
	return a.buildReleases(history), nil
}

// Latest returns the first build from List.
func (a *AppveyorAPI) Latest(ctx context.Context) (*Release, error) {
	list, err := a.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		a.logger.Error("no appveyor build found",
			"account", a.accountName,
			"project", a.projectName,
			"reason", "List returned empty",
			"result", "error",
		)
		return nil, fmt.Errorf("no suitable build found for %s/%s", a.accountName, a.projectName)
	}
	a.logger.Info("latest version detected",
		"account", a.accountName,
		"project", a.projectName,
		"version", list[0].Version,
		"job_id", list[0].JobID,
		"artifacts", len(list[0].Artifacts),
		"reason", "took first entry from List",
		"result", list[0].Version,
	)
	return list[0], nil
}

// LatestByVersion finds a specific build by version string using List.
func (a *AppveyorAPI) LatestByVersion(ctx context.Context, version string) (*Release, error) {
	list, err := a.List(ctx)
	if err != nil {
		return nil, err
	}

	for i, rel := range list {
		a.logger.Debug("appveyor rollback check",
			"account", a.accountName,
			"project", a.projectName,
			"index", i,
			"build_version", rel.Version,
			"target_version", version,
			"reason", "comparing computed version against target",
			"result", fmt.Sprintf("match=%v", rel.Version == version),
		)
		if rel.Version == version {
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

type appveyorHistory struct {
	Builds []struct {
		Version       string `json:"version"`
		PullRequestID string `json:"pullRequestId"`
	} `json:"builds"`
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
