package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
)

func TestFindJobID(t *testing.T) {
	tests := []struct {
		name string
		jobs []appveyorJob
		want string
	}{
		{
			name: "single job",
			jobs: []appveyorJob{{Name: "build", ID: "job-1"}},
			want: "job-1",
		},
		{
			name: "multiple jobs with release",
			jobs: []appveyorJob{
				{Name: "build", ID: "job-1"},
				{Name: "release", ID: "job-2"},
			},
			want: "job-2",
		},
		{
			name: "multiple jobs without release",
			jobs: []appveyorJob{
				{Name: "build", ID: "job-1"},
				{Name: "test", ID: "job-2"},
			},
			want: "",
		},
		{
			name: "empty jobs",
			jobs: []appveyorJob{},
			want: "",
		},
		{
			name: "case insensitive release",
			jobs: []appveyorJob{
				{Name: "build", ID: "job-1"},
				{Name: "Release", ID: "job-2"},
			},
			want: "job-2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findJobID(tt.jobs)
			if got != tt.want {
				t.Errorf("findJobID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAppveyorAPI_LatestByVersion_Match tests rollback version matching.
func TestAppveyorAPI_LatestByVersion_Match(t *testing.T) {
	history := appveyorHistory{
		Builds: []appveyorBuild{
			{Version: "1.0.0", PullRequestID: ""},
			{Version: "1.0.1", PullRequestID: ""},
			{Version: "1.0.2", PullRequestID: ""},
		},
	}
	buildDetail := appveyorBuildDetail{
		Build: struct {
			Jobs    []appveyorJob `json:"jobs"`
			Updated string        `json:"updated"`
		}{
			Jobs: []appveyorJob{{Name: "release", ID: "job-123"}},
		},
	}
	artifacts := []AppveyorArtifact{{FileName: "rpcs3-win64-vulkan.zip"}}

	mdl := newMockDownloader()
	hBody, _ := json.Marshal(history)
	bBody, _ := json.Marshal(buildDetail)
	aBody, _ := json.Marshal(artifacts)
	mdl.On("/api/projects/blueskythlikesclouds/mikumikulibrary/history", &HTTPResponse{StatusCode: 200, Body: hBody})
	mdl.On("/api/projects/blueskythlikesclouds/mikumikulibrary/build/1.0.1", &HTTPResponse{StatusCode: 200, Body: bBody})
	mdl.On("/api/buildjobs/job-123/artifacts", &HTTPResponse{StatusCode: 200, Body: aBody})

	api := NewAppveyorAPI(config.BasicConfig{
		AccountName: "blueskythlikesclouds",
		ProjectName: "mikumikulibrary",
	}, mdl, slog.Default())

	rel, err := api.LatestByVersion(context.Background(), "1.0.1")
	if err != nil {
		t.Fatalf("LatestByVersion() error = %v", err)
	}
	if rel.Version != "1.0.1" {
		t.Errorf("Version = %q, want %q", rel.Version, "1.0.1")
	}
	if rel.JobID != "job-123" {
		t.Errorf("JobID = %q, want %q", rel.JobID, "job-123")
	}
	if len(rel.Artifacts) != 1 || rel.Artifacts[0].FileName != "rpcs3-win64-vulkan.zip" {
		t.Errorf("Artifacts = %v, want [rpcs3-win64-vulkan.zip]", rel.Artifacts)
	}
}

// TestAppveyorAPI_LatestByVersion_NotFound tests that an error is returned
// when the target version is not found.
func TestAppveyorAPI_LatestByVersion_NotFound(t *testing.T) {
	history := appveyorHistory{
		Builds: []appveyorBuild{
			{Version: "1.0.0", PullRequestID: ""},
			{Version: "1.0.1", PullRequestID: ""},
		},
	}

	mdl := newMockDownloader()
	hBody, _ := json.Marshal(history)
	mdl.On("/api/projects/blueskythlikesclouds/mikumikulibrary/history", &HTTPResponse{StatusCode: 200, Body: hBody})

	api := NewAppveyorAPI(config.BasicConfig{
		AccountName: "blueskythlikesclouds",
		ProjectName: "mikumikulibrary",
	}, mdl, slog.Default())

	rel, err := api.LatestByVersion(context.Background(), "9.9.9")
	if err == nil {
		t.Fatalf("LatestByVersion() expected error, got %v", rel)
	}
}
