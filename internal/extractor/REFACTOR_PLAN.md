# Refactor Plan: extractor package → `github.com/mholt/archives`

## Goal
Replace the hand-written per-format extractors (`zip.go`, `targz.go`, `tarxz.go`,
`sevenz.go`, `sfx.go`) with the unified `github.com/mholt/archives` library, which
auto-detects archive/compression formats (zip, tar, tar.gz, tar.xz, 7z, rar, gz,
bz2, zst, …) and even self-extracting `.exe` (SFX) archives via stream peeking.

## Current architecture (to change)
- `extractor.go`: `Decompressor` + `Extractor` interface + `newExtractor(ext, src)` dispatch.
- `zip.go` / `targz.go` / `tarxz.go` / `sevenz.go`: per-format `Extractor` impls.
- `sfx.go`: magic-number scan to detect 7z SFX inside `.exe`.
- `utils.go`: `detectExt`, `extractTar`, `extract7zraw`, `safePath`, `copyFile`,
  `copyDir`, `moveDirContents`, `cleanInstall`, `skipper` types.
- `file_selector.go`: unchanged (kept).

## New architecture

### `extractor.go` (rewrite core)
- Drop the `Extractor` interface and `newExtractor` dispatch.
- Add an internal `extractJob{ src, dest string; skip skipper }` with:
  - `run()`: `os.Open(src)` → `archives.Identify(ctx, filepath.Base(src), f)`.
    - `archives.NoMatch` (or non-`archives.Extractor`) → `copyFile(src, dest/base)` (preserves non-archive copy behavior for `.exe`/`.apk`/`.dmg`).
    - else call `format.(archives.Extractor).Extract(ctx, f, handler)`.
  - `handler(ctx, fi archives.FileInfo) error`: apply `skip.shouldSkipFile(fi.NameInArchive)`; `safePath` check; `MkdirAll` for dirs; open `fi.Open()`, copy to `filepath.Join(dest, fi.NameInArchive)` with `fi.Mode()`.
- `Decompressor.Extract(srcPath, destDir)` keeps the **same public signature**:
  - `Skip.Bool()` → return nil.
  - `CleanInstall` → `cleanInstall(destDir)`.
  - build `job` with `skip = excludeSkipper(cfg.ExcludeFileType)`.
  - `SingleDir.Bool()` → `extractWithSingleDir(job, cfg.SingleDir, destDir)` (refactored to take `*extractJob` + prefix instead of `Extractor`).
  - else `job.run()`.

### `utils.go` (trim)
- **Keep**: `skipper` interface, `excludeSkipper`, `prefixSkipper`, `mergeSkipper`,
  `safePath`, `copyFile`, `copyDir`, `moveDirContents`, `cleanInstall`.
- **Delete**: `detectExt`, `extractTar`, `extract7zraw` (replaced by `archives`).

### Delete files
- `zip.go`, `targz.go`, `tarxz.go`, `sevenz.go`, `sfx.go` (all logic now in `archives`).

### `file_selector.go`
- Unchanged.

## Caller changes (`internal/updater/updater.go`)
- `isArchive(ext)` (line ~302): broaden the lexical check to include the formats
  `archives` supports and `.exe` (SFX). Add: `.7z`, `.rar`, `.tar`, `.gz`, `.bz2`,
  `.zst`, `.tzst`, `.tbz`, `.tbz2`, `.tar.zst`, `.tar.bz2`, `.lz4`, `.sz`, `.br`,
  `.z`, `.lz`, `.tar.lz4`, `.tar.lz`, and `.exe`. (Keep `.zip`, `.tar.gz`, `.tgz`,
  `.tar.xz`, `.txz`.) This preserves the existing `extDest` stripping + delete-archive logic.
- No change to the `extractor.New(...).Extract(...)` call site.

## Dependencies (`go.mod`)
- Add `github.com/mholt/archives` (v0.1.5).
- Remove `github.com/bodgit/sevenzip` and `github.com/ulikunitz/xz` (no longer used).
- Run `go mod tidy`.

## Tests to rewrite (`internal/extractor/*_test.go`)
- `extractor_test.go`:
  - Keep helpers `writeZipGo`, `writeTarGzGo`, `writeTarXzGo`, `writeSevenZGo`, `writeSfxGo`, `verifyExtracted`.
  - Rewrite `TestZipExtractor_Extract` / `TarGz` / `TarXz` / `SevenZ` / `SkipFilter` / `EvilPath` to drive extraction through `extractJob` (or `Decompressor`) instead of the removed `new*Extractor` constructors.
  - `TestSfxExtractor_Extract` / `NotASfx`: drive through `extractJob`/`Decompressor` (SFX now auto-detected by `Identify`).
- `extractor_extra_test.go`:
  - Remove `TestNewExtractor_*` (no more `newExtractor`). Replace with a small
    `TestIdentify_*` suite asserting `archives.Identify` detects `.zip`, `.tar.gz`,
    `.tar.xz`, `.7z`, and a non-SFX `.exe` → `NoMatch`.
- `sfx_test.go`:
  - Remove `TestSfxExtracter_ReadAt*` (no more `newSfxExtracter`/`ReadAt`). Replace
    with end-to-end SFX extraction tests via `Decompressor.Extract`.
- `utils_test.go`:
  - Remove `TestDetectExt` (no more `detectExt`).
  - Keep `TestSafePath`, `TestCopyFile`, `TestCopyDir` (if present), `TestMoveDirContents`, `TestCleanInstall`.

## Verification
- `go build ./...`
- `go test ./internal/extractor/... ./internal/updater/...`
- `go vet ./...`

## Risks / notes
- `archives.Extract` for `Zip`/`SevenZip` requires `io.ReaderAt`+`io.Seeker`; `os.File` satisfies both — fine.
- `archives` handles path-traversal sanitization, but we keep an explicit `safePath` guard for defense-in-depth (existing evil-path tests must still pass).
- Symlinks inside archives: `archives` exposes `LinkTarget`; we ignore it for now (write as regular file / skip) — acceptable for this use case.
- SFX detection previously used a 2 MB scan + magic offset; `archives.Identify` peeks only the header, which is sufficient since the 7z signature sits at offset 0 of the payload (right after the stub). Verified by the rewritten SFX tests.
