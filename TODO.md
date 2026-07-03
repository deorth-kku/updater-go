# TODO — 未完成功能清单

> 基于代码审查，按优先级排列。

---

## �� P1 — 配置/集成缺失

### 1. `Requests.Proxy` 未应用到 HTTP 请求
- **文件**: `internal/api/http.go`
- **现状**: `RequestsConfig.Proxy` 字段已定义，但 `httpClient` 未使用
- **需要**: 在 `NewHTTPClient` 中根据 `cfg.Requests.Proxy` 设置 `http.Transport.Proxy`

---

## 🟡 P2 — 功能字段未使用

### 2. `single_dir` 模式未实现
- **文件**: `internal/extractor/extractor.go`
- **现状**: `DecompressConfig.SingleDir` 字段已定义，提取器未实现
- **需要**: 提取到临时目录 → 检测内部单目录 → 将内容移动到安装路径

### 3. `clean_install` 模式未实现
- **文件**: `internal/extractor/extractor.go`
- **现状**: `DecompressConfig.CleanInstall` 字段已定义，提取器未实现
- **需要**: 提取前删除目标目录中的现有文件

### 4. `keep_download_file` 未实现
- **文件**: `internal/updater/updater.go`
- **现状**: `DecompressConfig.KeepDownloadFile` 字段已定义，提取后未保留压缩包
- **需要**: 提取完成后根据此字段决定是否删除下载的压缩包

### 5. `exclude_file_type_when_update` 未使用
- **文件**: `internal/extractor/file_selector.go`
- **现状**: `DownloadConfig.ExcludeFileTypeWhenUpdate` 字段已定义，但 `FileSelector` 未使用
- **需要**: 在 `Match` 方法中增加 `ExcludeFileTypeWhenUpdate` 过滤

### 6. `data` POST body 未实现
- **文件**: `internal/api/simple.go`
- **现状**: `DownloadConfig.Data` 字段已定义，SimpleSpider 仅支持 GET
- **需要**: 当 `Data` 非空时使用 `http.MethodPost` 并传入 body

### 7. `index`/`indexes` 匹配索引未实现
- **文件**: `internal/api/*.go`
- **现状**: `DownloadConfig.Index`/`Indexes` 字段已定义，API 层未使用
- **需要**: 在 `selectDownloadURL` 中根据 index/indexes 选择匹配的资产

### 8. `path` 嵌套 JSON 路径未在 `Latest()` 中使用
- **文件**: `internal/api/apijson.go`
- **现状**: `dictPathGet` 已实现，但 `Latest()` 中未使用 `Version.Path` 提取版本号
- **需要**: 在 `Latest()` 中通过 `verCfg.Path` 从 JSON 中提取版本号

---

## ⚪ P3 — 可选/低优先级

### 9. `noPull` 分支过滤未完整实现
- **文件**: `internal/api/github.go`
- **现状**: GitHub API 未实现 `noPull` 逻辑（AppVeyor 已实现 PR 过滤）
- **需要**: GitHub 端根据 `BuildConfig.NoPull` 使用 `/releases/latest` 而非 `/releases`

### 10. `branch` 过滤未从 config 读取
- **文件**: `internal/api/appveyor.go`
- **现状**: `SetBranch` 方法存在，但 `NewAPI` 工厂未传入 `cfg.Build.Branch`
- **需要**: 在 `NewAPI` 中调用 `appveyorAPI.SetBranch(cfg.Build.Branch)`

### 11. `add_version_to_filename` 未处理 `%arch`/`%OS`
- **文件**: `internal/updater/updater.go`
- **现状**: `{version}` 替换已实现，但未处理 `%arch`/`%OS` 变量
- **需要**: 在 `downloadFilename` 中扩展变量替换

### 12. `regexes` 链式匹配不完整
- **文件**: `internal/api/simple.go`
- **现状**: 链式 regex 已实现，但 `extractVersion` 中的 `FromPage` 逻辑可能不完整
- **需要**: 完善 `FromPage` 模式下的版本提取

### 13. `use_exe_version` 按 PLAN 声明 v1 不实现
- **文件**: `internal/updater/updater.go`
- **现状**: 字段已定义，未实现（符合 PLAN 设计）
- **需要**: 跳过，v1 不实现

---

## 📊 统计

| 类别 | 数量 |
|------|------|
| 🟠 P1 配置/集成 | 1 |
| 🟡 P2 字段未使用 | 7 |
| ⚪ P3 可选 | 5 |
| **总计** | **13