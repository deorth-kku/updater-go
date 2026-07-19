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
}

// NewAppveyorAPI creates a new AppVeyor API adapter.
func NewAppveyorAPI(cfg config.BasicConfig, dl Downloader) *AppveyorAPI {
	return &AppveyorAPI{
		accountName: cfg.AccountName,
		projectName: cfg.ProjectName,
		branch:      "",
		downloader:  dl,
	}
}

// SetBranch sets the build branch for filtering.
func (a *AppveyorAPI) SetBranch(branch string) {
	a.branch = branch
}

func (a *AppveyorAPI) Latest(ctx context.Context) (*Release, error) {
	baseURL := "https://ci.appveyor.com/api"
	branchParam := ""
	if a.branch != "" {
		branchParam = "&branch=" + a.branch
	}

	historyURL := fmt.Sprintf("%s/projects/%s/%s/history?recordsNumber=100%s",
		baseURL, a.accountName, a.projectName, branchParam)

	slog.Default().Debug("appveyor query",
		"step", "api.appveyor.latest",
		"account", a.accountName,
		"project", a.projectName,
		"branch", a.branch,
		"reason", "fetch build history (optionally filtered by branch)",
		"result", historyURL,
	)

	resp, err := a.downloader.Get(ctx, historyURL)
	if err != nil {
		return nil, fmt.Errorf("appveyor history: %w", err)
	}

	var history appveyorHistory
	if err := json.Unmarshal(resp.Body, &history); err != nil {
		return nil, fmt.Errorf("parse appveyor history: %w", err)
	}

	for _, build := range history.Builds {
		// Skip PR-triggered builds when no_pull is enabled
		if build.PullRequestID != "" {
			slog.Default().Debug("appveyor build skipped",
				"step", "api.appveyor.latest",
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
		buildResp, err := a.downloader.Get(ctx, buildURL)
		if err != nil {
			continue
		}

		var buildDetail appveyorBuildDetail
		if err := json.Unmarshal(buildResp.Body, &buildDetail); err != nil {
			continue
		}

		jobID := findJobID(buildDetail.Build.Jobs)
		if jobID == "" {
			slog.Default().Debug("appveyor build skipped",
				"step", "api.appveyor.latest",
				"account", a.accountName,
				"project", a.projectName,
				"version", version,
				"reason", "no suitable job id found in build",
				"result", "skip",
			)
			continue
		}

		artifactsURL := fmt.Sprintf("%s/buildjobs/%s/artifacts", baseURL, jobID)
		artResp, err := a.downloader.Get(ctx, artifactsURL)
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
					slog.Default().Debug("appveyor build skipped",
						"step", "api.appveyor.latest",
						"account", a.accountName,
						"project", a.projectName,
						"version", version,
						"reason", "no artifacts and build older than 30 days",
						"result", "skip",
					)
					continue
				}
			}
			slog.Default().Debug("appveyor build skipped",
				"step", "api.appveyor.latest",
				"account", a.accountName,
				"project", a.projectName,
				"version", version,
				"reason", "no artifacts and no/old timestamp",
				"result", "skip",
			)
			continue
		}

		slog.Default().Info("latest version detected",
			"step", "api.appveyor.latest",
			"account", a.accountName,
			"project", a.projectName,
			"version", version,
			"job_id", jobID,
			"artifacts", len(artifacts),
			"reason", "found build with artifacts",
			"result", version,
		)
		return &Release{
			Version:   version,
			Artifacts: artifacts,
			JobID:     jobID,
			BaseURL:   baseURL,
		}, nil
	}

	slog.Default().Error("no appveyor build found",
		"step", "api.appveyor.latest",
		"account", a.accountName,
		"project", a.projectName,
		"reason", "no build satisfied artifact/age criteria",
		"result", "error",
	)
	return nil, fmt.Errorf("no suitable build found for %s/%s", a.accountName, a.projectName)
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
