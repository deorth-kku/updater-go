# TODO — 未完成功能清单

> 基于代码审查，按优先级排列。

---

## 📊 统计

| 类别 | 数量 |
|------|------|
| **总计** | **0** |

---

## ✅ 已完成功能

### P1 — 配置/集成
- ✅ `Requests.Proxy` 已应用到 HTTP 请求 (`internal/api/http.go`)

### P2 — 功能字段
- ✅ `single_dir` 模式已实现 (`internal/extractor/extractor.go`)
- ✅ `clean_install` 模式已实现 (`internal/extractor/extractor.go`)
- ✅ `keep_download_file` 已实现 (`internal/updater/updater.go`)
- ✅ `exclude_file_type_when_update` 已实现 (`internal/extractor/file_selector.go`)
- ✅ `data` POST body 已实现 (`internal/api/simple.go`)
- ✅ `index`/`indexes` 匹配索引已实现 (`internal/updater/updater.go`)
- ✅ `path` 嵌套 JSON 路径已在 `Latest()` 中使用 (`internal/api/apijson.go`)

### P3 — 可选功能
- ✅ `noPull` 分支过滤已实现 (`internal/api/github.go`)
- ✅ `branch` 过滤已从 config 读取 (`internal/api/appveyor.go`)
- ✅ `add_version_to_filename` 已处理 `%arch`/`%OS` (`internal/updater/updater.go`)
- ✅ `regexes` 链式匹配已完成 (`internal/api/simple.go`)
- ⏭️ `use_exe_version` 按 PLAN 声明 v1 不实现 (跳过)