# TODO — 未完成功能清单

> 基于代码审查，按优先级排列。

---

## 🔴 P0 — 核心功能缺失

### 1. `process.Start` 未实现
- **文件**: `internal/process/process.go:55`
- **现状**: 返回 `fmt.Errorf("process start not yet implemented for %s", c.imageName)`
- **需要**: 根据 `image_name` 解析可执行文件路径并启动进程

### 2. `process.service` 模式未实现
- **文件**: `internal/process/process.go`
- **现状**: `ProcessConfig.Service` 字段已定义，但 `Stop`/`Start` 中未使用
- **需要**:
  - Linux: `systemctl stop/start <service>`
  - Windows: `sc stop/start <service>`

### 3. `process.restart_wait` 未使用
- **文件**: `internal/process/process.go`
- **现状**: `RestartWait` 字段已定义（默认 3），但 `Stop` 后未等待指定秒数
- **需要**: `Stop` 后调用 `time.Sleep(time.Duration(c.cfg.RestartWait) * time.Second)`

### 4. `process.stop_cmd` / `start_cmd` 未实现
- **文件**: `internal/process/process.go`
- **现状**: `StopCmd`/`StartCmd` 字段已定义，但未被使用
- **需要**: 当 `StopCmd` 非空时执行自定义命令而非 `pkill`/`taskkill`

### 5. Post-cmds 执行未实现
- **文件**: `internal/updater/updater.go`
- **现状**: 更新完成后未执行 post-cmds
- **需要**: 执行 `%PATH`/`%NAME`/`%DL_FILENAME` 变量替换后的 post-cmds

### 6. Config 写回未实现
- **文件**: `internal/updater/updater.go` / `cmd/updater/main.go`
- **现状**: 更新成功后未将 `currentVersion` 写回 `config.json`
- **需要**: 更新完成后将 `CurrentVersion` 写回项目配置文件

---

## 🟠 P1 — 配置/集成缺失

### 7. `metadata` 未集成到 `main.go`
- **文件**: `cmd/updater/main.go`
- **现状**: `metadata.Store` 已实现（`internal/metadata/metadata.go`），但 CLI 中未调用
- **需要**: 加载 metadata → 解析项目配置路径 → 用于项目发现

### 8. `--add2conf` 功能未实现
- **文件**: `cmd/updater/main.go`
- **现状**: Flag 已定义（`flagAdd2Conf`），但无持久化逻辑
- **需要**: 将新项目追加到 `config.json` 的 `projects` 数组并写回

### 9. Shell 自动补全未实现
- **文件**: `cmd/updater/main.go`
- **现状**: Cobra 已引入，但未调用 `CompletionCmd()`
- **需要**: 添加 `updater completion <shell>` 子命令

### 10. `Headers` 未从 `BasicConfig` 读取
- **文件**: `internal/api/simple.go`
- **现状**: SimpleSpider 使用硬编码 `defaultHeaders`，未读取 `BasicConfig.Headers`
- **需要**: 在 `NewSimpleSpiderAPI` 中合并 `cfg.Headers` 到默认 headers

### 11. `Requests.Proxy` 未应用到 HTTP 请求
- **文件**: `internal/api/http.go`
- **现状**: `RequestsConfig.Proxy` 字段已定义，但 `httpClient` 未使用
- **需要**: 在 `NewHTTPClient` 中根据 `cfg.Requests.Proxy` 设置 `http.Transport.Proxy`

---

## 🟡 P2 — 功能字段未使用

### 12. `single_dir` 模式未实现
- **文件**: `internal/extractor/extractor.go`
- **现状**: `DecompressConfig.SingleDir` 字段已定义，提取器未实现
- **需要**: 提取到临时目录 → 检测内部单目录 → 将内容移动到安装路径

### 13. `clean_install` 模式未实现
- **文件**: `internal/extractor/extractor.go`
- **现状**: `DecompressConfig.CleanInstall` 字段已定义，提取器未实现
- **需要**: 提取前删除目标目录中的现有文件

### 14. `keep_download_file` 未实现
- **文件**: `internal/updater/updater.go`
- **现状**: `DecompressConfig.KeepDownloadFile` 字段已定义，提取后未保留压缩包
- **需要**: 提取完成后根据此字段决定是否删除下载的压缩包

### 15. `exclude_file_type_when_update` 未使用
- **文件**: `internal/extractor/file_selector.go`
- **现状**: `DownloadConfig.ExcludeFileTypeWhenUpdate` 字段已定义，但 `FileSelector` 未使用
- **需要**: 在 `Match` 方法中增加 `ExcludeFileTypeWhenUpdate` 过滤

### 16. `data` POST body 未实现
- **文件**: `internal/api/simple.go`
- **现状**: `DownloadConfig.Data` 字段已定义，SimpleSpider 仅支持 GET
- **需要**: 当 `Data` 非空时使用 `http.MethodPost` 并传入 body

### 17. `index`/`indexes` 匹配索引未实现
- **文件**: `internal/api/*.go`
- **现状**: `DownloadConfig.Index`/`Indexes` 字段已定义，API 层未使用
- **需要**: 在 `selectDownloadURL` 中根据 index/indexes 选择匹配的资产

### 18. `path` 嵌套 JSON 路径未在 `Latest()` 中使用
- **文件**: `internal/api/apijson.go`
- **现状**: `dictPathGet` 已实现，但 `Latest()` 中未使用 `Version.Path` 提取版本号
- **需要**: 在 `Latest()` 中通过 `verCfg.Path` 从 JSON 中提取版本号

---

## ⚪ P3 — 可选/低优先级

### 19. `noPull` 分支过滤未完整实现
- **文件**: `internal/api/github.go`
- **现状**: GitHub API 未实现 `noPull` 逻辑（AppVeyor 已实现 PR 过滤）
- **需要**: GitHub 端根据 `BuildConfig.NoPull` 使用 `/releases/latest` 而非 `/releases`

### 20. `branch` 过滤未从 config 读取
- **文件**: `internal/api/appveyor.go`
- **现状**: `SetBranch` 方法存在，但 `NewAPI` 工厂未传入 `cfg.Build.Branch`
- **需要**: 在 `NewAPI` 中调用 `appveyorAPI.SetBranch(cfg.Build.Branch)`

### 21. `add_version_to_filename` 未处理 `%arch`/`%OS`
- **文件**: `internal/updater/updater.go`
- **现状**: `{version}` 替换已实现，但未处理 `%arch`/`%OS` 变量
- **需要**: 在 `downloadFilename` 中扩展变量替换

### 22. `regexes` 链式匹配不完整
- **文件**: `internal/api/simple.go`
- **现状**: 链式 regex 已实现，但 `extractVersion` 中的 `FromPage` 逻辑可能不完整
- **需要**: 完善 `FromPage` 模式下的版本提取

### 23. `use_exe_version` 按 PLAN 声明 v1 不实现
- **文件**: `internal/updater/updater.go`
- **现状**: 字段已定义，未实现（符合 PLAN 设计）
- **需要**: 跳过，v1 不实现

---

## 📊 统计

| 类别 | 数量 |
|------|------|
| 🔴 P0 核心缺失 | 6 |
| 🟠 P1 配置/集成 | 5 |
| 🟡 P2 字段未使用 | 7 |
| ⚪ P3 可选 | 3 |
| **总计** | **21** |
