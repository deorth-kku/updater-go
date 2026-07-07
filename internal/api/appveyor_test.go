package api

import (
	"testing"
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
