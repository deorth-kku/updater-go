package extractor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/deorth-kku/updater-go/internal/config"
	"github.com/deorth-kku/updater-go/internal/peversion"
	"github.com/mholt/archives"
)

func TestFindSfxOffset(t *testing.T) {
	fakeSfx := append(append([]byte(nil), sevenZipMagic...), []byte("7z payload data here")...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find the 7z signature")
	}
	if offset != 0 {
		t.Errorf("findSfxOffset() = %d, want 0", offset)
	}
}

func TestFindSfxOffset_ShortHeaderAtEnd(t *testing.T) {
	fakeSfx := append([]byte("stub"), sevenZipMagic...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find trailing 7z signature")
	}
	if offset != 4 {
		t.Errorf("findSfxOffset() = %d, want 4", offset)
	}
}

func TestFindSfxOffset_WithStub(t *testing.T) {
	// 7z signature embedded after an executable stub
	stub := []byte("MZ\x90\x00this is an executable stub of some length")
	magic := sevenZipMagic
	payload := []byte("0123456789ABCDEF")
	fakeSfx := append(append(stub, magic...), payload...)

	tmpDir := t.TempDir()
	sfxPath := tmpDir + "/test.exe"
	if err := os.WriteFile(sfxPath, fakeSfx, 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find the embedded 7z signature")
	}
	if offset != int64(len(stub)) {
		t.Errorf("findSfxOffset() = %d, want %d", offset, len(stub))
	}
}

func TestFindSfxOffset_NotASfx(t *testing.T) {
	// File without magic number
	tmpDir := t.TempDir()
	badPath := tmpDir + "/not_sfx.exe"
	if err := os.WriteFile(badPath, []byte("just some random data"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(badPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, ty := sfxMatcher.match(t.Context(), f)
	if ty != notSfx {
		t.Error("findSfxOffset() expected false for non-SFX file")
	}
}

func TestFindSfxOffset_FileNotFound(t *testing.T) {
	f, err := os.Open("/nonexistent/file.exe")
	if err == nil {
		f.Close()
		t.Fatal("expected error opening nonexistent file")
	}
}

func TestFindSfxOffsetPE_NotAPe(t *testing.T) {
	// Non-PE data should return notSfx (falls back to scanning in findSfxOffsetReaderAt).
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "not_pe.exe")
	if err := os.WriteFile(badPath, []byte("just some random data"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(badPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	off := findSfxOffsetPE(f)
	if off != 0 {
		t.Error("findSfxOffsetPE() expected notSfx for non-PE file")
	}
}

func TestIdentify_Sfx7z(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "payload.7z")
	writeSevenZGo(t, archivePath, map[string]string{"hello.txt": "hello\n"})

	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	sfxPath := filepath.Join(tmpDir, "payload.exe")
	stub := []byte("MZ\x90\x00stub")
	if err := os.WriteFile(sfxPath, append(stub, payload...), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	format, _, err := archives.Identify(t.Context(), filepath.Base(sfxPath), f)
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	if _, ok := format.(archives.Extractor); !ok {
		t.Fatal("Identify(.exe SFX) should return an archives.Extractor")
	}
}

const nvidia_testfile = "/tmp/610.88-desktop-win10-win11-64bit-international-dch-whql.exe"

func TestIdentify_Nvidia(t *testing.T) {
	checkFile(t, nvidia_testfile)
	f, err := os.Open(nvidia_testfile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find trailing 7z signature")
	}
	if offset != 1100324 {
		t.Errorf("findSfxOffset() = %d, want 1100324", offset)
	}
}

func TestExtractFile_Sfx7z(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "payload.7z")
	contents := map[string]string{"hello.txt": "hello\n", "sub/dir/file.txt": "content\n"}
	writeSevenZGo(t, archivePath, contents)

	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	sfxPath := filepath.Join(tmpDir, "payload.exe")
	stub := []byte("MZ\x90\x00stub")
	if err := os.WriteFile(sfxPath, append(stub, payload...), 0o644); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	cfg := config.DecompressConfig{}
	d, err := New(t.Context(), sfxPath, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	verifyExtracted(t, destDir, contents)
}

const (
	testZipSfx = "/tmp/ReShade_Setup_6.7.3_Addon.exe"
	test7zSfx  = "/tmp/git-sdk-installer-1.0.8-64.7z.exe"
)

func checkFile(t *testing.T, name string) {
	t.Helper()
	_, err := os.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		t.SkipNow()
	}
}
func TestFindSfxOffset_7z(t *testing.T) {
	// curl -L https://github.com/git-for-windows/build-extra/releases/download/git-sdk-1.0.8/git-sdk-installer-1.0.8-64.7z.exe >/tmp/git-sdk-installer-1.0.8-64.7z.exe
	checkFile(t, test7zSfx)
	f, err := os.Open(test7zSfx)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != sevenZipSfx {
		t.Fatal("findSfxOffset() expected to find trailing 7z signature")
	}
	if offset != 501745 {
		t.Errorf("findSfxOffset() = %d, want 501745", offset)
	}
}

const batfile = `@REM Set up the Git SDK

@REM determine root directory

@REM https://technet.microsoft.com/en-us/library/bb490909.aspx says:
@REM <percent>~dpI Expands <percent>I to a drive letter and path only.
@REM <percent>~fI Expands <percent>I to a fully qualified path name.
@FOR /F "delims=" %%D in ("%~dp0") do @set cwd=%%~fD

@CD "%cwd%"
@IF ERRORLEVEL 1 GOTO DIE

@REM set PATH
@set PATH=%cwd%\mini\mingw64\bin;%PATH%

@ECHO Cloning the Git for Windows SDK...
@git init
@IF ERRORLEVEL 1 GOTO DIE
@git config http.sslbackend schannel
@IF ERRORLEVEL 1 GOTO DIE
@git remote add origin https://github.com/git-for-windows/git-sdk-64
@IF ERRORLEVEL 1 GOTO DIE
@git fetch --depth 1 origin
@IF ERRORLEVEL 1 GOTO DIE
@git -c core.fscache=true checkout -t origin/main
@IF ERRORLEVEL 1 GOTO DIE

@REM Cleaning up temporary git.exe
@RMDIR /Q /S mini
@IF ERRORLEVEL 1 GOTO DIE

@REM Avoid overlapping address ranges
@IF 32 == 64 @(
	ECHO Auto-rebasing .dll files
	CALL autorebase.bat
)

@REM Before running a shell, let's prevent complaints about "permission denied"
@REM from MSYS2's /etc/post-install/01-devices.post
@MKDIR dev\shm 2> NUL
@MKDIR dev\mqueue 2> NUL

@START /B git-bash.exe
@EXIT /B 0

:DIE
@ECHO Installation of Git for Windows' SDK failed!
@PAUSE
@EXIT /B 1

`

func TestExtractFile_Sfx7zWithInstall(t *testing.T) {
	checkFile(t, test7zSfx)
	destDir := t.TempDir()
	cfg := config.DecompressConfig{}
	d, err := New(t.Context(), test7zSfx, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyExtracted(t, destDir, map[string]string{
		"setup-git-sdk.bat": batfile,
	})
}

type readerOnly struct {
	io.Reader
}

// TestExtractDirect_SfxZipWithInstall tests sfxMatcher.Extract() directly
// without going through the Decompressor layer.
func TestExtractDirect_SfxZipWithInstall(t *testing.T) {
	checkFile(t, testZipSfx)
	destDir := t.TempDir()

	f, err := os.Open(testZipSfx)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	handler := makeHandler(destDir, nil, slog.Default())
	err = sfxMatcher.Extract(t.Context(), readerOnly{f}, handler)
	if err == nil || err.Error() != "input type must be an io.ReaderAt and io.Seeker because of zip format constraints" {
		t.Error("unexpected error :", err)
	}

}

func TestFindSfxOffset_Zip(t *testing.T) {
	// curl https://reshade.me/downloads/ReShade_Setup_6.7.3_Addon.exe >/tmp/ReShade_Setup_6.7.3_Addon.exe
	checkFile(t, testZipSfx)
	f, err := os.Open(testZipSfx)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	offset, ty := sfxMatcher.match(t.Context(), f)
	if ty != zipSfx {
		t.Fatal("findSfxOffset() expected to find trailing zip signature")
	}
	if offset != 154624 {
		t.Errorf("findSfxOffset() = %d, want 154624", offset)
	}
}

func TestExtractFile_SfxZip(t *testing.T) {
	checkFile(t, testZipSfx)
	destDir := t.TempDir()
	cfg := config.DecompressConfig{}
	d, err := New(t.Context(), testZipSfx, cfg, false, "", slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()
	if err := d.Extract(t.Context(), destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	verifyJsonExtracted(t, destDir, []string{
		"ReShade32.json",
		"ReShade32_XR.json",
		"ReShade64.json",
		"ReShade64_XR.json",
	})
	filever, prodver, err := peversion.FileVersion(filepath.Join(destDir, "ReShade64.dll"))
	if err != nil {
		t.Fatalf("peversion error = %v", err)
	}
	fmt.Println(filever, prodver)
}

// verifyJsonExtracted checks that all expected JSON files exist in destDir and contain valid JSON.
func verifyJsonExtracted(t *testing.T, destDir string, jsonFiles []string) {
	t.Helper()
	for _, name := range jsonFiles {
		full := filepath.Join(destDir, name)
		content, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("%s: open: %v", name, err)
			continue
		}
		var v any
		if err := json.Unmarshal(content, &v); err != nil {
			t.Errorf("%s: invalid JSON: %v", name, err)
		}
	}
}
