package platform

import (
	"runtime"
	"testing"
)

func TestExpandVariables(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"%arch", ArchName()},
		{"%OS", OSName()},
		{"%arch-%OS", ArchName() + "-" + OSName()},
		{"no_vars", "no_vars"},
		{"%arch%arch", ArchName() + ArchName()},
	}
	for _, tt := range tests {
		if got := ExpandVariables(tt.input); got != tt.want {
			t.Errorf("ExpandVariables(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestArchName(t *testing.T) {
	// Just verify it returns something reasonable
	got := ArchName()
	if got == "" {
		t.Error("ArchName() returned empty string")
	}
	// On this system it should match runtime.GOARCH
	if runtime.GOARCH == "amd64" && got != "amd64" {
		t.Errorf("ArchName() = %q on amd64, want %q", got, "amd64")
	}
}

func TestOSName(t *testing.T) {
	got := OSName()
	if got == "" {
		t.Error("OSName() returned empty string")
	}
}

func TestExpandKeywords(t *testing.T) {
	input := []string{"%arch", "release", "%OS"}
	got := ExpandKeywords(input)
	want := []string{ArchName(), "release", OSName()}
	if len(got) != len(want) {
		t.Fatalf("ExpandKeywords() len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ExpandKeywords()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
