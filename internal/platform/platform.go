// Package platform provides OS/arch detection and variable expansion for
// download keywords (e.g. %arch → "amd64", %OS → "windows").
package platform

import (
	"runtime"
	"strings"
)

// ArchName returns the Go-style architecture name used in download keywords.
// Maps runtime.GOARCH to the values used in updater-config.
func ArchName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	case "arm":
		return "arm"
	default:
		return runtime.GOARCH
	}
}

// OSName returns the OS name used in download keywords.
// Maps runtime.GOOS to the values used in updater-config.
func OSName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

// ExpandVariables replaces %arch and %OS in the input string.
func ExpandVariables(s string) string {
	s = strings.ReplaceAll(s, "%arch", ArchName())
	s = strings.ReplaceAll(s, "%OS", OSName())
	return s
}

// ExpandKeywords expands %arch/%OS in all keywords.
func ExpandKeywords(keywords []string) []string {
	result := make([]string, len(keywords))
	for i, k := range keywords {
		result[i] = ExpandVariables(k)
	}
	return result
}
