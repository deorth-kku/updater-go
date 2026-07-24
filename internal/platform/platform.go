// Package platform provides OS/arch detection and variable expansion for
// download keywords (e.g. %arch → "amd64", %OS → "windows").
package platform

import (
	"runtime"
	"strings"

	"github.com/deorth-kku/updater-go/internal/config"
)

// ArchCandidates returns the full list of architecture candidate strings that
// a "%arch" keyword expands to. This mirrors updater-rpc's multi-alias
// behaviour (gap #27) where a single "%arch" token is replaced by every
// candidate architecture name the tool supports for the current platform.
func ArchCandidates() []string {
	switch runtime.GOARCH {
	case "amd64":
		return []string{"x86_64", "amd64", "x64", "linux-64", "linux64"}
	case "arm64":
		return []string{"arm64", "aarch64", "armv8"}
	case "386":
		return []string{"i386", "i686", "linux-32", "x86"}
	case "arm":
		return []string{"arm", "armv7", "armv7l"}
	default:
		return []string{runtime.GOARCH}
	}
}

// OSCandidates returns the full list of OS candidate strings that a "%OS"
// keyword expands to, mirroring updater-rpc (gap #27).
func OSCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"windows", "Windows", "win"}
	case "linux":
		return []string{"linux", "Linux", "ubuntu"}
	case "darwin":
		return []string{"darwin", "Darwin", "macos"}
	default:
		os := runtime.GOOS
		return []string{os, strings.Title(os)}
	}
}

// ExpandKeywords expands %arch/%OS keyword tokens. A keyword that is exactly
// "%arch" is replaced by every architecture candidate; a keyword that is
// exactly "%OS" is replaced by every OS candidate. Other keywords (including
// partial uses of the tokens, which the reference leaves unexpanded) are kept
// verbatim. This replicates updater-rpc's var_replace list substitution
// (gap #27).
func ExpandKeywords(keywords config.Keywords) config.Keywords {
	out := make(config.Keywords, len(keywords))
	for i, k := range keywords {
		switch {
		case isString(k, "%arch"):
			out[i] = ArchCandidates()
		case isString(k, "%OS"):
			out[i] = OSCandidates()
		default:
			out[i] = k
		}
	}
	return out
}

func isString(k []string, v string) bool {
	if len(k) != 1 {
		return false
	}
	return k[0] == v
}
