# Test Coverage Plan

## Current Test Status

| Package | File | Tests | Coverage Notes |
|---------|------|-------|----------------|
| `api` | `api_test.go` | 15 tests | Good coverage for GitHub, AppVeyor, SourceForge, ApiJson APIs |
| `config` | `config_test.go`, `project_test.go` | 18 tests | Good coverage for Load, ApplyDefaults, StringOrSlice, BoolOrString |
| `extractor` | `extractor_test.go`, `file_selector_test.go` | 25+ tests | Good coverage for all extractors, FileSelector |
| `metadata` | `metadata_test.go` | 6 tests | Good coverage for Store.Load, GetEntry, Entries |
| `platform` | `platform_test.go` | 4 tests | Good coverage for ExpandVariables, ArchName, OSName |
| `process` | `process_test.go` | 10 tests | Good coverage for Stop, Start, IsRunning |
| `updater` | `updater_test.go` | 6 tests | Good coverage for Update flow, ReplaceVars |

## Missing Test Coverage

### 1. `internal/extractor/utils.go`
- `detectExt()` - Extension detection for compound extensions
- `safePath()` - Path traversal prevention
- `copyFile()` - File copying
- `cleanInstall()` - Directory cleanup
- `moveDirContents()` - Directory content movement
- `copyDir()` - Recursive directory copy

### 2. `internal/extractor/extractor.go`
- `Decompressor.Extract()` - Skip mode, CleanInstall mode
- `newExtractor()` - Registry lookup for all extensions
- `prefixSkipper` - Prefix-based file skipping
- `mergeSkipper` - Merging multiple skipper

### 3. `internal/extractor/sfx.go`
- `sfxExtracter.ReadAt()` - Reading from SFX offset
- `sfxExtracter.Close()` - Cleanup

### 4. `internal/config/project.go`
- `BoolOrString.MarshalJSON()` - Custom marshaling
- `StringOrSlice.First()` - First element extraction
- `PathSegment.IsString()` - String segment detection
- `StringOrJsonPath` - Custom unmarshaling

### 5. `internal/api/http.go`
- `parseProxyURL()` - Proxy URL normalization
- `httpClient.Get()` - HTTP client implementation

### 6. `internal/api/simple.go`
- `extractFilename()` - Filename extraction from URL
- `unescapeHTML()` - HTML entity unescaping
- `siteName()` - Site name extraction
- `joinURL()` - URL joining
- `buildFromDirectURL()` - Direct URL mode

### 7. `internal/api/appveyor.go`
- `findJobID()` - Job ID selection logic

### 8. `internal/updater/updater.go`
- `selectDownloadURL()` - URL selection logic
- `downloadFilename()` - Filename generation
- `isArchive()` - Archive detection
- `assetNames()` - Asset name extraction
- `artifactNames()` - Artifact name extraction

### 9. `internal/metadata/metadata.go`
- `EnsureLocalConfig()` - Local config management
- `downloadConfig()` - Config download

### 10. `internal/downloader/aria2_local.go`
- `generateSecret()` - Secret generation

## Implementation Priority

1. **High Priority** - Core functionality without tests
   - `extractor/utils.go` - Utility functions used by all extractors
   - `extractor/extractor.go` - Decompressor public API
   - `api/simple.go` - SimpleSpider API helpers

2. **Medium Priority** - Important but less frequently tested
   - `config/project.go` - Configuration marshaling
   - `updater/updater.go` - Update flow helpers
   - `metadata/metadata.go` - Metadata management

3. **Low Priority** - Edge cases and internal helpers
   - `api/http.go` - HTTP client
   - `api/appveyor.go` - Job ID selection
   - `downloader/aria2_local.go` - Secret generation
