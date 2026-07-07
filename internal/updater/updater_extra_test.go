package updater

import (
	"testing"

	"github.com/deorth-kku/updater-go/internal/api"
)

func TestAssetNames_NoDuplicates(t *testing.T) {
	assets := []api.Asset{
		{Name: "file1.zip"},
		{Name: "file2.zip"},
		{Name: "file3.zip"},
	}
	got := assetNames(assets)
	if len(got) != 3 {
		t.Fatalf("assetNames() len = %d, want 3", len(got))
	}
	if got[0] != "file1.zip" || got[1] != "file2.zip" || got[2] != "file3.zip" {
		t.Errorf("assetNames() = %v, want [file1.zip, file2.zip, file3.zip]", got)
	}
}

func TestReplaceVars_Extras(t *testing.T) {
	tests := []struct {
		testName string
		input    string
		path     string
		varName  string
		dlFile   string
		version  string
		expected string
	}{
		{
			testName: "all variables",
			input:    "%PATH/%NAME",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "/opt/app/myapp",
		},
		{
			testName: "version variable",
			input:    "app-%VER",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "2.0.0",
			expected: "app-2.0.0",
		},
		{
			testName: "dl filename variable",
			input:    "%DL_FILENAME",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "download.zip",
			version:  "1.0.0",
			expected: "download.zip",
		},
		{
			testName: "no variables",
			input:    "static string",
			path:     "/opt/app",
			varName:  "myapp",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "static string",
		},
		{
			testName: "multiple occurrences",
			input:    "%NAME-%NAME",
			path:     "/opt/app",
			varName:  "test",
			dlFile:   "app.zip",
			version:  "1.0.0",
			expected: "test-test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			got := replaceVars(tt.input, tt.path, tt.varName, tt.dlFile, tt.version)
			if got != tt.expected {
				t.Errorf("replaceVars() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAssetNames_Empty(t *testing.T) {
	got := assetNames(nil)
	if len(got) != 0 {
		t.Errorf("assetNames(nil) len = %d, want 0", len(got))
	}
}

func TestArtifactNames(t *testing.T) {
	artifacts := []api.AppveyorArtifact{
		{FileName: "artifact1.zip"},
		{FileName: "artifact2.zip"},
	}
	got := artifactNames(artifacts)
	if len(got) != 2 {
		t.Fatalf("artifactNames() len = %d, want 2", len(got))
	}
	if got[0] != "artifact1.zip" || got[1] != "artifact2.zip" {
		t.Errorf("artifactNames() = %v, want [artifact1.zip, artifact2.zip]", got)
	}
}

func TestArtifactNames_Empty(t *testing.T) {
	got := artifactNames(nil)
	if len(got) != 0 {
		t.Errorf("artifactNames(nil) len = %d, want 0", len(got))
	}
}
