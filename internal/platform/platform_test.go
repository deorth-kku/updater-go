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
	// ArchName is the primary candidate from ArchCandidates.
	if got != ArchCandidates()[0] {
		t.Errorf("ArchName() = %q, want primary candidate %q", got, ArchCandidates()[0])
	}
}

func TestOSName(t *testing.T) {
	got := OSName()
	if got == "" {
		t.Error("OSName() returned empty string")
	}
}

func TestExpandKeywords(t *testing.T) {
	// An exact "%arch" token expands to the full candidate list; "%OS" to the
	// OS candidate list; others are kept verbatim (gap #27).
	input := []string{"%arch", "release", "%OS"}
	got := ExpandKeywords(input)
	want := append([]string{}, ArchCandidates()...)
	want = append(want, "release")
	want = append(want, OSCandidates()...)
	if len(got) != len(want) {
		t.Fatalf("ExpandKeywords() len = %d, want %d (%v)", len(got), len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ExpandKeywords()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
func TestExpandVariables_Empty(t *testing.T) {
	if got := ExpandVariables(""); got != "" {
		t.Errorf("ExpandVariables(\"\") = %q, want empty", got)
	}
}

func TestExpandVariables_NoMatch(t *testing.T) {
	tests := []string{
		"hello world",
		"%unknown",
		"just text",
	}
	for _, input := range tests {
		if got := ExpandVariables(input); got != input {
			t.Errorf("ExpandVariables(%q) = %q, want %q", input, got, input)
		}
	}
}

func TestArchName_AllVariants(t *testing.T) {
	// Test that ArchName returns recognized values
	got := ArchName()
	// On Linux, we might get other arches too, so just check it's not empty
	if got == "" {
		t.Error("ArchName() returned empty string")
	}
}

func TestOSName_AllVariants(t *testing.T) {
	got := OSName()
	if got == "" {
		t.Error("OSName() returned empty string")
	}
}

func TestExpandKeywords_Empty(t *testing.T) {
	got := ExpandKeywords(nil)
	if got == nil {
		t.Error("ExpandKeywords(nil) should return empty slice, not nil")
	}
	if len(got) != 0 {
		t.Errorf("ExpandKeywords(nil) len = %d, want 0", len(got))
	}
}

func TestExpandKeywords_NoExpansion(t *testing.T) {
	input := []string{"hello", "world"}
	got := ExpandKeywords(input)
	if len(got) != 2 {
		t.Fatalf("ExpandKeywords() len = %d, want 2", len(got))
	}
	if got[0] != "hello" || got[1] != "world" {
		t.Errorf("ExpandKeywords() = %v, want [hello, world]", got)
	}
}

func TestExpandKeywords_Mixed(t *testing.T) {
	// Exact "%arch"/"%OS" tokens expand to candidate lists; "static" is kept.
	input := []string{"%arch", "static", "%OS"}
	got := ExpandKeywords(input)
	want := append([]string{}, ArchCandidates()...)
	want = append(want, "static")
	want = append(want, OSCandidates()...)
	if len(got) != len(want) {
		t.Fatalf("ExpandKeywords() len = %d, want %d", len(got), len(want))
	}
	if got[len(ArchCandidates())] != "static" {
		t.Errorf("ExpandKeywords()[%d] = %q, want %q", len(ArchCandidates()), got[len(ArchCandidates())], "static")
	}
}

func TestArchCandidates_NonEmpty(t *testing.T) {
	if len(ArchCandidates()) == 0 {
		t.Error("ArchCandidates() returned empty list")
	}
	if ArchCandidates()[0] != ArchName() {
		t.Errorf("ArchCandidates()[0] = %q, want primary %q", ArchCandidates()[0], ArchName())
	}
}

func TestOSCandidates_NonEmpty(t *testing.T) {
	if len(OSCandidates()) == 0 {
		t.Error("OSCandidates() returned empty list")
	}
	// Linux must include "ubuntu" per updater-rpc (gap #27).
	if runtime.GOOS == "linux" {
		found := false
		for _, c := range OSCandidates() {
			if c == "ubuntu" {
				found = true
			}
		}
		if !found {
			t.Errorf("OSCandidates() = %v, want to include ubuntu", OSCandidates())
		}
	}
}

func TestExpandVariables_Tilde(t *testing.T) {
	// Ensure tilde is not affected
	if got := ExpandVariables("~"); got != "~" {
		t.Errorf("ExpandVariables(\"~\") = %q, want %q", got, "~")
	}
}

func TestArchName_RuntimeConsistency(t *testing.T) {
	// ArchName's primary candidate must be the first entry of ArchCandidates.
	if ArchName() != ArchCandidates()[0] {
		t.Errorf("ArchName() = %q, want primary candidate %q", ArchName(), ArchCandidates()[0])
	}
}

func TestOSName_RuntimeConsistency(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		if OSName() != "windows" {
			t.Errorf("OSName() = %q on windows, want windows", OSName())
		}
	case "linux":
		if OSName() != "linux" {
			t.Errorf("OSName() = %q on linux, want linux", OSName())
		}
	case "darwin":
		if OSName() != "macos" {
			t.Errorf("OSName() = %q on darwin, want macos", OSName())
		}
	}
}
