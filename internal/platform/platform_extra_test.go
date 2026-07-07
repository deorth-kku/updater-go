package platform

import (
	"runtime"
	"testing"
)

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
	input := []string{"%arch", "static", "%OS"}
	got := ExpandKeywords(input)
	if len(got) != 3 {
		t.Fatalf("ExpandKeywords() len = %d, want 3", len(got))
	}
	if got[0] == "%arch" {
		t.Errorf("ExpandKeywords()[0] not expanded, still %s", got[0])
	}
	if got[1] != "static" {
		t.Errorf("ExpandKeywords()[1] = %q, want %q", got[1], "static")
	}
	if got[2] == "%OS" {
		t.Errorf("ExpandKeywords()[2] not expanded, still %s", got[2])
	}
}

func TestExpandVariables_Tilde(t *testing.T) {
	// Ensure tilde is not affected
	if got := ExpandVariables("~"); got != "~" {
		t.Errorf("ExpandVariables(\"~\") = %q, want %q", got, "~")
	}
}

func TestArchName_RuntimeConsistency(t *testing.T) {
	// ArchName should match runtime.GOARCH for known values
	switch runtime.GOARCH {
	case "amd64":
		if ArchName() != "amd64" {
			t.Errorf("ArchName() = %q on amd64, want amd64", ArchName())
		}
	case "arm64":
		if ArchName() != "arm64" {
			t.Errorf("ArchName() = %q on arm64, want arm64", ArchName())
		}
	case "386":
		if ArchName() != "x86" {
			t.Errorf("ArchName() = %q on 386, want x86", ArchName())
		}
	case "arm":
		if ArchName() != "arm" {
			t.Errorf("ArchName() = %q on arm, want arm", ArchName())
		}
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
