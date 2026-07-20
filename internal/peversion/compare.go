package peversion

import "regexp"

var nonVersionChars = regexp.MustCompile(`[^0-9.\-]`)

// ConvertVersion mirrors updater-rpc's version_convert: it strips everything
// that is not a digit, dot or dash, replaces dashes with dots, and splits on
// dots into a slice of integers. Non-numeric segments become 0.
//
//	e.g. "v1.2.3-beta" -> []int{1, 2, 3}
//	     "Release 0.73" -> []int{0, 73}
func ConvertVersion(s string) []int {
	s = nonVersionChars.ReplaceAllString(s, "")
	s = replaceAll(s, "-", ".")
	parts := splitDots(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			out = append(out, 0)
			continue
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}

// Ints returns the 4 components as a plain []int for comparison.
func (v Version) Ints() []int {
	return []int{int(v[0]), int(v[1]), int(v[2]), int(v[3])}
}

// versionGreater reports whether a is lexicographically strictly greater than
// b, comparing component by component up to the shorter length. It is the
// inverse of Python's version_compare: that function returns True (i.e.
// "not greater") when, component by component, a never exceeds b. The critical
// match is prefix equality: Python's version_compare returns True (a is NOT
// greater) when a is a prefix of b or equals it, so here we return false on
// prefix equality rather than treating the longer slice as greater.
func versionGreater(a, b []int) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	// Prefix equal: strictly greater only if a has an extra non-equal
	// component... but per Python's version_compare, a prefix-equal a is NOT
	// greater than b, so we always return false here.
	return false
}

// NeedsUpdate decides whether the remote version requires an update, using the
// exact semantics of updater-rpc: an update happens only when the remote
// version is strictly greater than BOTH the installed FileVersion and the
// ProductVersion. If either installed version already meets or exceeds the
// remote one, no update is needed.
func NeedsUpdate(remote string, fileVer, prodVer Version) bool {
	r := ConvertVersion(remote)
	return versionGreater(r, fileVer.Ints()) && versionGreater(r, prodVer.Ints())
}

// tiny stdlib reimplementations to avoid importing strings in a hot path.
func replaceAll(s, old, new string) string {
	if old == "" {
		return s
	}
	out := ""
	start := 0
	for i := 0; i+len(old) <= len(s); {
		if s[i:i+len(old)] == old {
			out += s[start:i] + new
			i += len(old)
			start = i
		} else {
			i++
		}
	}
	out += s[start:]
	return out
}

func splitDots(s string) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
