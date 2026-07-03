# updater-rpc Go Rewrite — Design Plan

## Summary

Rewrite the Python `updater-rpc` (at ../updater-rpc) tool in Go. Detect latest versions via 5 API backends (GitHub Releases, AppVeyor CI, SourceForge RSS, generic web scraping, generic JSON API), download via aria2 RPC (using your `aria2rpc-go` client), extract with pure Go libraries, manage process restarts — all driven by the existing JSON config schema from `updater-config`.

**Design goals:**
- Clean interface boundaries; each layer testable in isolation
- Parallel project updates via goroutine worker pool
- Zero CGO; cross-compilable
- 100% backward-compatible with existing `config.json` + per-project configs from `updater-config` (at ../updater-config)

---

## Module Layout

```
updater-rpc/
├── cmd/updater/main.go              # CLI entry point
├── internal/
│   ├── config/
│   │   ├── config.go                # Main config: loading, defaults, legacy migration
│   │   └── project.go               # Per-project config structs (matching updater-config schema)
│   ├── api/
│   │   ├── api.go                   # API interface + NewAPI factory (dispatches on basic.api_type)
│   │   ├── github.go                # GitHub Releases API
│   │   ├── appveyor.go              # AppVeyor CI API
│   │   ├── sourceforge.go           # SourceForge RSS API
│   │   ├── simple.go                # SimpleSpider: regex-based web scraping
│   │   └── apijson.go               # ApiJson: path-based JSON navigation
│   ├── downloader/
│   │   ├── downloader.go            # Download coordination, progress, retry
│   │   └── aria2.go                 # Thin wrapper around aria2rpc.Client
│   ├── extractor/
│   │   ├── extractor.go             # Format dispatch + core extraction logic
│   │   ├── zip.go                   # ZIP via std library
│   │   ├── tar.go                   # TAR/GZ via std library
│   │   ├── sevenz.go                # 7z via github.com/bodgit/sevenzip
│   │   └── tarxz.go                 # TAR/XZ via stdlib + ulikunitz/xz
│   │   └── file_selector.go         # Include/exclude keyword filtering
│   ├── updater/
│   │   └── updater.go               # Core update orchestration
│   ├── process/
│   │   └── process.go               # Process stop/start/restart via os/exec
│   ├── metadata/
│   │   └── metadata.go              # Remote metadata repo management
│   └── platform/
│       └── platform.go              # OS/arch detection (%arch, %OS variable expansion)
├── config.json                      # User's main config
├── config/                          # Local per-project configs (optional override)
├── go.mod
└── go.sum
```

---

## Aria2 Architecture (Primary Design)

**Primary mode**: Connect to an existing aria2 RPC server (local or remote).
**Fallback**: Start a local aria2c subprocess if no RPC endpoint is reachable.

### Remote Directory Mapping

When `aria2.ip` is not `127.0.0.1` / `localhost`:
- `aria2.remote-dir`: the download directory on the remote aria2 server (e.g. `/mnt/aria2/downloads`)
- `aria2.local-dir`: the local mount point (NFS/SMB) where the same files are accessible (e.g. `/mnt/nfs/aria2/downloads`)

**Download flow:**
1. Tell aria2 to download to `remote_dir/project_name`
2. Poll `tellStatus` until download completes (status == "complete")
3. Read the finished file from `local_dir/project_name/filename` (local mount)
4. Extract from local path

**Rationale**: Reuse downloaded files across multiple systems. When the file already exists on the remote filesystem (no `.aria2` resume file), aria2 reports download complete immediately — so the same download is "shared" across machines without re-downloading.

### Config Fields

```go
type Aria2Config struct {
    IP            string  `json:"ip"`             // "127.0.0.1" or remote host
    RPCListenPort string  `json:"rpc-listen-port"` // "6800"
    RPCSecret     string  `json:"rpc-secret"`     // optional
    RemoteDir     string  `json:"remote-dir"`     // only when using remote aria2
    LocalDir      string  `json:"local-dir"`      // only when using remote aria2
}
```

### Downloader Interface

```go
type Downloader interface {
    // Download tells aria2 to fetch url into the project's download dir.
    // Returns the local filesystem path to the downloaded file and the aria2 GID.
    Download(ctx context.Context, url, filename string) (localPath, gid string, err error)
    // Remove cleans up the download result from aria2.
    Remove(gid string) error
}

type Aria2Downloader struct {
    client    *aria2rpc.Client
    remoteDir string // empty if local aria2
    localDir  string // empty if local aria2
}
```

**Implementation:**
- Uses **WebSocket callbacks** (no HTTP polling):
  - Connect with `ws://` or `wss://` endpoint
  - Register `WithNotificationCallbacks` for `OnDownloadComplete` / `OnDownloadError` events
  - Each download creates a channel; callback closes the channel when done
- If `remoteDir != ""`: download dir = `remoteDir/projectName`, file read from `localDir/projectName/filename`
- If `remoteDir == ""`: download dir = `<installPath>/downloads`, file read from same path
- `AddURI` with opts: `dir`, `out` (filename), `split=16`, `max-connection-per-server=16`, `continue=true`
- Block on channel until callback fires (complete or error)
- Support custom headers from project config (passed via `header` aria2 option)
- Support proxy from main config

---

## Config Schema (preserved exactly)

### Main config (`config.json`)

```go
type Config struct {
    Aria2        Aria2Config         `json:"aria2"`
    Binaries     BinariesConfig      `json:"binarys"`
    Requests     RequestsConfig      `json:"requests"`
    Projects     []ProjectEntry      `json:"projects"`
    Repositories []string            `json:"repository"`
    Defaults     map[string]any      `json:"defaults"`
}
```

### Per-project config (from `updater-config/`)

```go
type ProjectConfig struct {
    Basic      BasicConfig       `json:"basic"`
    Build      BuildConfig       `json:"build"`
    Download   DownloadConfig    `json:"download"`
    Process    ProcessConfig     `json:"process"`
    Decompress DecompressConfig  `json:"decompress"`
    Version    VersionConfig     `json:"version"`
    JSONVer    string            `json:"jsonver"`
}
```

Key fields observed across real configs:
- `basic.api_type`: `"github"` | `"appveyor"` | `"sourceforge"` | `"simplespider"` | `"apijson"`
- `basic.account_name`, `basic.project_name` — repo identifiers
- `basic.api_url` — for `apijson` type (e.g. Skyline APK builds)
- `basic.page_url` — for `simplespider` type (e.g. Go download page)
- `basic.headers` — custom HTTP headers (e.g. NVIDIA driver config)
- `download.path` — nested JSON path array for `apijson` (e.g. `["IDS", 0, "downloadInfo", "DownloadURL"]`)
- `download.keyword` / `download.exclude_keyword` — filename filters (supports `%arch`, `%OS` variables)
- `download.filetype` — archive extension filter (e.g. `"zip"`, `"tar.gz"`, `"7z"`, `"apk"`)
- `download.url` — direct download URL (bypasses API)
- `download.regexes` — regex patterns for `simplespider` page scraping
- `download.index` / `download.indexes` — match index for keyword/regex selection
- `download.try_redirect` — follow HTTP 302/303 redirects
- `download.add_version_to_filename`, `download.filename_override` — filename control
- `download.data` — POST body data for download
- `decompress.skip` — skip extraction (e.g. APK files)
- `decompress.clean_install` — clean install before extracting
- `decompress.exclude_file_type_when_update` — extra exclusions on re-update
- `decompress.single_dir` — extract to temp, move inner dir contents to install path
- `decompress.keep_download_file` — retain archive after extraction
- `process.allow_restart`, `process.image_name`, `process.service`, `process.restart_wait`, `process.stop_cmd`, `process.start_cmd` — process management
- `version.path` — JSON path for `apijson` version extraction
- `version.regex` — regex for version from filename or page
- `version.use_exe_version` — Windows PE version (not implemented in v1)
- `version.from_page` — extract version from webpage instead of filename
- `build.branch`, `build.no_pull` — AppVeyor build filtering

---

## Key Interfaces

### API Layer (`internal/api/api.go`)

```go
// API discovers the latest version and its download URL for a project.
type API interface {
    Name() string
    GetVersion(ctx context.Context) (string, error)
    GetDownloadURL(ctx context.Context, version string) (string, error)
}

// NewAPI creates the appropriate API adapter based on project config.
func NewAPI(name string, cfg *ProjectConfig) (API, error)
```

Dispatches on `cfg.Basic.APIType`:
- `"github"` → `GithubAPI{account, project, branch, noPull}`
  - `noPull=False`: uses `GET /repos/{user}/{repo}/releases` → first release in list (includes drafts)
  - `noPull=True`: uses `GET /repos/{user}/{repo}/releases/latest` → latest non-draft release (filters PR builds)
- `"appveyor"` → `AppveyorAPI{account, project, branch, noPull}`
  - `noPull=True`: skips builds with `pullRequestId` (PR-triggered builds)
  - `noPull=False`: includes all builds
- `"sourceforge"` → `SourceforgeAPI{project}` — parses RSS feed, matches filename keywords
- `"simplespider"` → `SimpleAPI{pageURL, regexes, headers, tryRedirect}` — fetches page, applies regex chain with redirect following
- `"apijson"` → `ApiJsonAPI{apiURL, versionPath, dlPath}` — fetches JSON, navigates nested paths

All adapters share an HTTP client with retry (configurable retries, timeout, proxy support).

### Downloader (`internal/downloader/aria2.go`)

```go
type Downloader interface {
    Download(ctx context.Context, url, filename string) (localPath, gid string, err error)
    Remove(gid string) error
}
```

Uses your `aria2rpc-go` client with **WebSocket callbacks** (no polling):
- **Connect**: `aria2rpc.New(ctx, "ws://<ip>:<port>/jsonrpc", aria2rpc.WithSecret(secret))`
  - Uses `ws://` or `wss://` endpoint for WebSocket notifications
  - Registers `WithNotificationCallbacks` for `OnDownloadComplete` / `OnDownloadError` events
- **Download**: `client.AddURI(ctx, []string{url}, opts)` → GID
  - `dir`: `remoteDir/projectName` (remote) or `<installPath>/downloads` (local)
  - `out`: filename (with version if `add_version_to_filename`)
  - `split`: "16", `max-connection-per-server`: "16", `continue`: "true"
  - `header`: custom headers from project config
  - `proxy`: from main config
- **Wait**: blocks on channel until `OnDownloadComplete` or `OnDownloadError` callback fires
- **Read**: file is at `localPath = localDir/projectName/filename` (remote) or `dldir/filename` (local)
- **Cleanup**: `client.RemoveDownloadResult(ctx, gid)`
- **Shutdown**: `client.Shutdown(ctx)` on exit

The `Downloader` implementation creates a channel per download, registers the callback, and blocks on the channel. When the callback fires, the channel is closed and the goroutine unblocks. This is more efficient than HTTP polling and matches the event-driven nature of aria2's WebSocket notifications.

### Extractor (`internal/extractor/extractor.go`)

```go
type Extractor interface {
    Extract(ctx context.Context, archivePath, destPath string, cfg *DecompressConfig) (selectedFiles []string, err error)
}

func NewExtractor(ext string) (Extractor, error)
```

Format dispatch:
- `.zip` → `archive/zip` (stdlib)
- `.tar`, `.tar.gz`, `.tgz` → `archive/tar` + `compress/gzip` (stdlib)
- `.tar.xz` → `archive/tar` + `github.com/ulikunitz/xz` (pure Go)
- `.7z` → `github.com/bodgit/sevenzip` (pure Go, no CGO)

After extraction, `file_selector.go` filters files by include/exclude keywords and file type patterns. Supports `single_dir` mode (extract to temp, move inner dir contents to install path) and `clean_install` mode (remove existing files first).

### Updater (`internal/updater/updater.go`)

```go
type Updater struct {
    name        string
    installPath string
    cfg         *ProjectConfig
    api         api.API
    downloader  downloader.Downloader
    extractor   extractor.Extractor
    procCtrl    process.Controller
}

func (u *Updater) Run(ctx context.Context, force bool, currentVersion string) (newVersion string, err error)
```

**Run flow:**
1. `api.GetVersion(ctx)` → latest version
2. Compare with `currentVersion` (skip unless `force`)
3. `api.GetDownloadURL(ctx, version)` → download URL
4. `downloader.Download(ctx, url, filename)` → local file path (aria2 downloads to remote, file read from local mount)
5. If `decompress.skip`: skip extraction, use file directly
6. Otherwise: `extractor.Extract()` on local file → filter → copy to install path
7. If `allow_restart`: stop process → wait → extract → start process
8. If not `allow_restart`: log warning → wait for process exit → extract
9. Run post-cmds with `%PATH`, `%NAME`, `%DL_FILENAME` substitutions
10. Return new version string

### Process Controller (`internal/process/process.go`)

```go
type Controller interface {
    Stop(ctx context.Context) error
    Start(ctx context.Context) error
    WaitForStop(ctx context.Context) error
    IsRunning() bool
}
```

Uses `os/exec` to find/signals processes by image name. Windows: `taskkill`. Linux/macOS: `pkill` / `kill`. Service mode: `systemctl` / `sc`.

### Metadata Store (`internal/metadata/metadata.go`)

```go
type Store struct {
    repos []string
    cache map[string]*Entry
}

type Entry struct {
    ConfigPath string `json:"config_path"`
    Date       string `json:"date,omitempty"`
}
```

Fetches `metadata.json` from each configured repo URL. Maps project name → config file path within the repo. On update, fetches the actual project config JSON from `repo_url/config_path`.

---

## CLI (`cmd/updater/main.go`)

Built with [spf13/cobra](https://github.com/spf13/cobra) for auto-completion support.

```
updater [projects...] [flags]

Flags:
  -c, --config path    config file path (default: ./config.json or ~/.config/updater-rpc/config.json)
  -f, --force          force update regardless of version
  -a, --add2conf       persist added project to config
  -w, --wait           pause before exit (Windows convenience)
  -j, --jobs N         max parallel update workers (default: GOMAXPROCS)
  -v, --verbose        enable debug logging
```

Cobra provides:
- Shell auto-completion (bash, zsh, fish, powershell) via `updater completion <shell>`
- Consistent flag parsing and validation
- Hierarchical command structure (ready for future subcommands like `updater status`, `updater remove`)

Flow:
1. Load config → apply defaults → handle legacy migration (old `projects: map` → new `projects: []`)
2. Connect to aria2 RPC (remote if configured, else try local; fallback: spawn aria2c subprocess)
3. Load metadata from all repos
4. Resolve each project's config: local `config/<name>.json` first, then remote from repo
5. Expand `%arch` / `%OS` variables in keywords based on `runtime.GOOS` / `runtime.GOARCH`
6. Submit updates to worker pool (`errgroup` + semaphore, bounded by `--jobs`)
7. Collect errors, log per-project results
8. Update `config.json` with new `currentVersion` values
9. Shutdown aria2, exit

---

## Concurrency Model

- `golang.org/x/sync/errgroup` + `golang.org/x/sync/semaphore.Weighted` for bounded parallelism
- Worker count = `--jobs` (default: `runtime.GOMAXPROCS(0)`)
- Each project update runs in its own goroutine; errors collected via `errgroup`
- Config writes serialized via `sync.Mutex`
- Shared resources (aria2 client, HTTP client) created once, passed in — no per-goroutine duplication

---

## Dependencies

| Package | Source | Purpose |
|---------|--------|---------|
| `github.com/deorth-kku/aria2rpc-go` | local `../aria2rpc-go` | aria2 JSON-RPC client |
| `github.com/bodgit/sevenzip` | GitHub | Pure Go 7z extraction |
| `github.com/ulikunitz/xz` | GitHub | Pure Go XZ decompression |
| `github.com/spf13/cobra` | GitHub | CLI framework with auto-completion |
| `golang.org/x/sync` | Go module | `errgroup` + semaphore |

Everything else is Go stdlib: `net/http`, `encoding/json`, `archive/zip`, `archive/tar`, `compress/gzip`, `log/slog`, `os/exec`, `context`.

---

## Testing Strategy

- **Unit tests** for each API adapter — mock HTTP via `net/http/httptest` with fixtures from real `updater-config` JSONs
- **Unit tests** for `file_selector.go` — keyword matching, `%arch`/`%OS` expansion, type filtering
- **Unit tests** for config loading — valid JSON, missing fields, legacy migration, variable expansion
- **Unit tests** for extractor — verify format dispatch and single-dir mode (mock file I/O)
- **Integration test** for updater flow — local HTTP server serves a test ZIP; verify full download→extract→copy cycle
- **No test** for aria2 RPC integration — requires running aria2c daemon (tested separately)

---

## Assumptions & Defaults

1. **Go 1.22+** — `log/slog`, generics, `slices` package
2. **aria2 RPC is primary** — assume an aria2 server is running; local aria2c subprocess is a fallback
3. **Remote dir mapping is transparent** — downloader abstracts whether file is local or on NFS/SMB mount
4. **Version comparison**: lexicographic string compare (matching current Python behavior); semver parsing deferred
5. **Windows popup messages** → replaced with `slog.Warn`; no GUI dependency
6. **`pefile` / `use_exe_version`** → not implemented in v1; logs warning if config requests it
7. **Logging**: `log/slog` with text handler by default, verbose with `--verbose`
8. **Config path resolution**: `--config` flag → `./config.json` → `$HOME/.config/updater-rpc/config.json`
9. **Project config resolution**: local `config/<name>.json` → remote from metadata repo
10. **Variable expansion**: `%arch` and `%OS` in download keywords expanded at runtime based on `runtime.GOOS` / `runtime.GOARCH`
