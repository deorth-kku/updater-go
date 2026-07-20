package peversion

import (
	"os"
	"path/filepath"
	"testing"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "fixture.exe")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	return p
}

func TestFileVersion(t *testing.T) {
	p := fixturePath(t)
	fv, pv, err := FileVersion(p)
	if err != nil {
		t.Fatalf("FileVersion: %v", err)
	}
	want := Version{1, 2, 3, 4}
	if fv != want {
		t.Fatalf("FileVersion = %v, want %v", fv, want)
	}
	if got := fv.String(); got != "1.2.3.4" {
		t.Fatalf("String() = %q, want %q", got, "1.2.3.4")
	}
	// ProductVersion must also be parsed (mirrors Python's use of both
	// FileVersionMS/LS and ProductVersionMS/LS).
	if pv == (Version{}) {
		t.Fatalf("ProductVersion not parsed: got zero value")
	}
}

func TestFileVersionFromReader(t *testing.T) {
	p := fixturePath(t)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	fv, pv, err := FileVersionFromReader(&readerAt{b})
	if err != nil {
		t.Fatalf("FileVersionFromReader: %v", err)
	}
	if fv != (Version{1, 2, 3, 4}) {
		t.Fatalf("FileVersionFromReader = %v, want {1 2 3 4}", fv)
	}
	if pv == (Version{}) {
		t.Fatalf("FileVersionFromReader ProductVersion not parsed: got zero value")
	}
}

func TestConvertVersion(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"1.2.3.4", []int{1, 2, 3, 4}},
		{"v1.2.3-beta", []int{1, 2, 3, 0}},
		{"Release 0.73", []int{0, 73}},
		{"", []int{0}},
		{"2021.9.3", []int{2021, 9, 3}},
	}
	for _, c := range cases {
		got := ConvertVersion(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("ConvertVersion(%q) = %v, want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("ConvertVersion(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

func TestNeedsUpdate(t *testing.T) {
	installed := Version{1, 2, 3, 4}
	// Newer remote version -> update needed.
	if !NeedsUpdate("1.2.3.5", installed, installed) {
		t.Fatal("expected update for 1.2.3.5 > 1.2.3.4")
	}
	if !NeedsUpdate("2.0.0.0", installed, installed) {
		t.Fatal("expected update for 2.0.0.0 > 1.2.3.4")
	}
	// Equal remote version -> no update.
	if NeedsUpdate("1.2.3.4", installed, installed) {
		t.Fatal("did not expect update for equal version")
	}
	// Older remote version -> no update.
	if NeedsUpdate("1.2.3.0", installed, installed) {
		t.Fatal("did not expect update for older version")
	}
	// If FileVersion is 0.0.0.0 but ProductVersion is current, update is
	// still needed because remote is newer than BOTH (matches updater-rpc's
	// not(A or B) semantics: update only when remote > fileVer AND remote >
	// prodVer).
	if !NeedsUpdate("1.2.3.5", Version{0, 0, 0, 0}, installed) {
		t.Fatal("expected update when ProductVersion is current but FileVersion is 0")
	}
	// Prefix-equal / longer remote must NOT trigger an update (mirrors
	// Python's version_compare, which treats a prefix-equal old version as
	// "not smaller", so the remote is not strictly greater).
	if NeedsUpdate("1.2.3.4.5", installed, installed) {
		t.Fatal("did not expect update for remote that is a prefix-extension of installed")
	}
	if NeedsUpdate("1.2.3", installed, installed) {
		t.Fatal("did not expect update for remote that is a prefix of installed")
	}
}

// readerAt adapts a byte slice to io.ReaderAt.
type readerAt struct{ b []byte }

func (r *readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.b)) {
		return 0, os.ErrClosed
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, os.ErrClosed
	}
	return n, nil
}
