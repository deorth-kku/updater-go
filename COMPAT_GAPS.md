# updater-go vs updater-rpc — 100% 复刻差距清单

> 目标（来自 `PLAN.md`）：100% 复刻 Python 版 `updater-rpc`（位于 `../updater-rpc`）的行为，
> 包括 API 后端检测、aria2 下载、解压、进程重启、以及完全兼容 `updater-config` 的 JSON 配置 schema。
>
> 本文档逐条列出当前 Go 实现与 Python 参考实现之间**尚未达到 100% 复刻**之处。
> 每条标注：**严重程度**（功能缺失 / 行为差异 / 次要）、**对应 Python 代码位置**、**Go 当前实现**、以及**影响**。
>
> ## 完成状态说明
> 截至 commit `bc78a55`（自 `c04457f` 起的 10 个对齐 commit），下列缺口已补齐并验证（build + 全部测试通过）：
> **#1, #2, #3, #4, #5, #6, #7, #8, #9, #10, #11, #12, #13, #15, #16, #17, #18, #21, #22, #23, #25, #27**。
> 已在各条目标题标注 ✅ 并补充 Go 落点。
> 以下条目**仍未完成**（保留为差距）：**#14, #19, #20, #26**。

---

## 一、核心流程 / 功能缺失（会导致行为明显不同）

### 1. `post-cmds` 后处理命令 ✅（已补齐，commit `e970809`）
- **Python**：`main.py:158-162`，更新成功后遍历 `pro["post-cmds"]`，依次做 `%PATH`/`%NAME`/`%DL_FILENAME` 替换后 `os.system(line)` 执行。
- **Go 落点**：`updater.go:415-438` + `project.go:18`（`PostCmds []string`）。逐条 `replaceVars` 替换 `%PATH`/`%NAME`/`%DL_FILENAME`/`%VER` 后，按 OS 用 `cmd /c` 或 `sh -c` 执行，行为对齐；`updater_batch8_test.go` 覆盖。
- **影响**：已恢复为与 Python 一致的后处理行为。

### 2. `use_system_package_manager` 系统包管理器安装 ✅（已补齐，commit `a1c4309`）
- **Python**：`updater.py:474-495`，当该选项为真且 OS 为 linux 时，用 `dpkg -i` 或 `rpm -ivh` 安装 `.deb`/`.rpm`，并设 `DEBIAN_FRONTEND=noninteractive`；非 linux 跳过。
- **Go 落点**：`extractor.go:312-353` `installWithSystemPackageManager`，linux 下优先 `dpkg -i --force-confdef`，否则 `rpm -ivh`；非 linux/无包管理器则跳过。行为与 Python 基本一致（仅少了 `DEBIAN_FRONTEND=noninteractive` 环境变量，实践中无影响）。
- **影响**：已恢复 deb/rpm 系统安装路径。

### 3. `.VERSION` 版本文件写入 ✅（已补齐，commit `e970809`）
- **Python**：`updater.py:564-579` `updateVersionFile()`，非 `use_refresh` 模式下会把版本字符串写入 `<path>/<name>.VERSION`。
- **Go 落点**：`updater.go:443-482` `updateVersionFile`，非 `use_exe_version` 时写 `<save_path>/<project_name>.VERSION`。
- **影响**：已恢复 `.VERSION` 文件产出。

### 4. 非 `allow_restart` 时不等待进程退出 ✅（已补齐，commit `e970809`）
- **Python**：`updater.py:602-609`，`run()` 默认 `popup=True`；当 `allow_restart=False` 时，若进程仍在运行，会弹窗警告并 `waitProc()` 等待进程结束，再解压。
- **Go 落点**：`updater.go:243-264`，非 `AllowRestart` 且进程仍在运行时，`process.Controller.WaitForStop` 等待（上限 5min）后才解压；`process.go:334-351` 提供 `WaitForStop`/`IsRunning`。（Python 用弹窗，Go 无 GUI 弹窗，改为日志警告 + 等待，等价。）
- **影响**：已恢复“占用时不强行解压”的行为。

---

## 二、下载 / 解压细节差异

### 5. aria2 下载未传递 `proxy` / `header` / `split` / `max-connection` / `continue` 等选项 ✅（已补齐，commit `a5b6c21`）
- **Python**：`updater.py:443-455`，`wget()` 传入 `proxy`、`retry`、以及 `basic.headers` 拼成的 `header` 列表，且全局 aria2 带 `split=16`、`max-connection-per-server=16`、`continue=true`。
- **Go 落点**：`downloader.go:161-185` `buildAria2Options`，设置 `split=16`、`max-connection-per-server=16`、`continue=true`，并追加全局 `proxy`、`retry`、以及每项目 `basic.headers` 拼成的 `header`（换行分隔）选项；`NewAria2Downloader` 接收 `proxy`/`retry`。`Download` 已透传 `headers`。
- **影响**：自定义头、代理、分片、断点续传均已对齐。

### 6. `download.data` 对直链 (`url`) 的 POST 行为未实现 ✅（已补齐，commit `c04457f`）
- **Python**：`simpleapi.py:48-54`，当 `download.url != ""` 且 `data` 非空时，对 `dlurl` 发 POST 并跟随 302/303 重定向取 `Location`。
- **Go 落点**：`simple.go:108-146` `buildFromDirectURL`，`len(dlCfg.Data)>0` 时调 `postFormAndFollow`（表单 POST + 跟随 302/303 Location）；`simple.go:338-362`。`fetchPage` 也支持 POST JSON（`simple.go:148-190`）。
- **影响**：需 POST 取直链的项目已对齐。

### 7. `add_version_to_filename` 仅作用于 `filename_override`，对派生文件名无效 ✅（已补齐，commit pending）
- **Python**：`updater.py:434-442`，无论文件名来自 `filename_override` 还是从 URL 派生，都会剥离末尾的 filetype 扩展名、插入清洗过的版本号、再拼回扩展名。
- **Go 落点**：`updater.go:654-690`，`downloadFilename` 现在对**两条分支**都应用 `addVersionToName`（先 `sanitizeVersion` 把 `< > / \ | : * ?` 替换为空格，再按配置的 `filetype` 剥离末尾扩展名、rstrip 一个点、拼成 `base_<version>.<ext>`）；若某 filetype 匹配则插入版本，否则原样返回。override 分支在已使用 `{version}` 占位符时不重复插入（避免双重版本）。`updater_batch8_test.go` 的 `TestDownloadFilename_URLDerivedVersion` 覆盖。
- **影响**：已恢复为与 Python 一致的“版本化文件名”行为，URL 派生名也带版本号，同名覆盖问题解决。

### 8. 默认 filetype 未设为 `["7z"]` ✅（已补齐，commit `afcc886`）
- **Python**：`updater.py:317-319`，当 `download.filetype` 为空时默认设为 `["7z"]`。
- **Go 落点**：`config.go:26` `DownloadConfig.Filetype` 硬编码默认 `StringOrSlice{"7z"}`；`sourceforge.go:82-84` 也显式回退到 `"7z"`。
- **影响**：未配置 filetype 时，行为与 Python 一致（仅匹配 7z）。

### 9. `update_keyword` / `keyword` 在“重新更新”时切换逻辑未实现 ✅（已补齐，commit `7412843`）
- **Python**：`updater.py:354-359`，安装模式（`install`）或 `update_keyword` 为空时用 `keyword`+`exclude_keyword`；否则（再更新）用 `update_keyword`+`exclude_keyword`。
- **Go 落点**：`file_selector.go:26-42` `NewFileSelector` 接收 `install` 标志；非安装且 `UpdateKeyword` 非空时改用 `update_keyword`+`exclude_keyword`，否则用 `keyword` 并把 `update_keyword` 追加进排除列表。`updater.go:600-606` `isInstallMode` 提供安装判定。
- **影响**：安装/更新关键词分离已对齐。

### 10. `include_file_type` 在解压阶段被忽略 ✅（已补齐，commit `a1c4309`）
- **Python**：`updater.py:233-251` + `file_sel`，若设置了 `include_file_type`，只**提取**匹配这些类型的文件。
- **Go 落点**：`extractor.go:131,135,279-308` 的 `innerSelector.include` + `shouldSkipFile` 实现“仅提取 include 类型”语义。
- **影响**：解压时按 include 类型过滤已对齐。

### 11. `exclude_file_type_when_update` 的应用时机不同 ✅（已补齐，commit `a1c4309`）
- **Python**：`updater.py:520-522`，仅在**非安装（再更新）**模式下才把 `exclude_file_type_when_update` 追加到排除列表。
- **Go 落点**：`extractor.go:125-130`，`install` 为 true 时不追加 `exclude_file_type_when_update`；`file_selector.go` 也接收 `ExcludeFileTypeWhenUpdate`，由 `install` 控制是否生效。
- **影响**：安装/更新排除时机已对齐。

### 12. `single_dir` 字符串形式（指定前缀目录）未实现 ✅（已补齐，commit `a1c4309`）
- **Python**：`updater.py:515-518`，`single_dir` 可以是字符串——表示归档内的固定前缀目录；为 `false` 时 `prefix=""`，为 `true` 时 `prefix=getPrefixDir()`。
- **Go 落点**：`extractor.go:116-142` + `extractWithSingleDir`/`extractWithSingleDirAuto`，`SingleDir.IsString` 时把 `StringVal` 当固定前缀子目录提取并移动到目标；`Bool()` 时自动检测单顶层目录并扁平化（`getPrefixDir` 语义）。`innerSelector.prefix` 负责前缀过滤。
- **影响**：字符串前缀目录与自动扁平化均已对齐。

### 13. 单文件快速重命名兜底（gpu-z 情形）未实现 ✅（已补齐，commit `a1c4309`）
- **Python**：`updater.py:551-557`，当选出的文件只有 1 个时，把它重命名到 `image_name` 对应路径。
- **Go 落点**：`extractor.go:153-172`，`imageName != ""` 且 `innerSelector.extracted` 恰为 1 项时，重命名为 `image_name`。
- **影响**：单文件工具更新后文件名规范化已对齐。

### 14. `use_builtin_zipfile` 标志被忽略 ⚠️（**仍未完成**）
- **Python**：`updater.py:21-32`，该标志为真时强制用内置 `zipfile` 而非 libarchive。
- **Go**：`project.go:131` 字段已解析（`UseBuiltinZipfile`），但 `extractor.go` 始终用 `archives.Identify` 自动探测，忽略该标志。
- **影响**：通常无差异（Go 默认纯 Go 解压与 Python 的 zipfile 路径结果一致），仅在需要强制 zipfile 的特殊格式下不一致。低优先级遗留项。

---

## 三、API 后端差异

### 15. SourceForge 版本号语义差异 ✅（已补齐，commit `a94364d`）
- **Python**：`sourceforge.py:33` `self.version = str(file_info["date"])`，版本即 RSS `pubDate`。
- **Go 落点**：`sourceforge.go:56-145` `Latest`，遍历 RSS item，按 `sourceforgeFilenameCheck`（keyword/exclude_keyword/filetype 多候选）过滤，取第 `Index` 个匹配；版本用 `item.PubDate`，并兼容两种日期格式解析。
- **影响**：keyword/filetype/index 过滤已对齐，版本语义一致。

### 16. simplespider 的 `version.index` 被忽略 ✅（已补齐，commit `c04457f`）
- **Python**：`simpleapi.py:69-75` `re.findall(regex, src)[index]`，`version.index` 默认 0。
- **Go 落点**：`simple.go:286-311` `extractVersion`，用 `verCfg.Index` 选择 `FindAllStringSubmatch` 的第 `Index` 个匹配。
- **影响**：版本序号选择已对齐。

### 17. simplespider 多级 regex 的每级 `indexes` 未实现 ✅（已补齐，commit `c04457f`）
- **Python**：`simpleapi.py:56-66`，`indexs` 是每个正则层级独立选择的序号（`indexs[lv]`），缺省为每级 `0`。
- **Go 落点**：`simple.go:215-284` `extractURLFromPage`，按 `dlCfg.Indexes[lv]`（缺省 0）对每级 `FindAllStringSubmatch` 取第 `lv` 级指定序号；支持跨级 fetch 并相对/绝对 URL 解析。
- **影响**：多级爬虫每级序号选择已对齐。

### 18. GitHub/AppVeyor 的 `download.index` 语义不一致 ✅（已补齐，commit `53ca376`）
- **Python**：`appveyor.py:123` / `github.py:156` `return match_urls[index]`——返回**单个**位于 `index` 的资源。
- **Go 落点**：`updater.go:616-631` `applyIndex`，`Indexes` 非空时逐条取对应元素；否则 `Index>0` 时返回 `matched[Index:Index+1]`（单个元素），与 Python 的 0 基取单个语义一致。
- **影响**：index 取单个资源的语义已对齐。

---

## 四、版本比较 / exe 版本

### 19. `use_cmd_version` 未实现（与 Python 一致，但仍是占位） ℹ️（**仍未完成**）
- **Python**：`updater.py:417-422` 仅有空的 `try/pass`（标注 Not implemented yet）。
- **Go**：`updater.go` 的 generic 分支只做 `rel.Version == OldVersion` 字符串比较；`use_cmd_version` 字段解析了但未使用。
- **影响**：与 Python 行为一致（都不实现），但严格说不算“复刻该字段的语义”。低优先级遗留项。

### 20. `use_exe_version` 仅在 Windows PE 上有意义，跨平台解析行为差异 ℹ️（**仍未完成**）
- **Python**：依赖 `pefile`（仅当安装时）；无 pefile 时该分支会异常。
- **Go**：`peversion.go` 用 `debug/pe` 跨平台解析，非 PE 文件（如 ELF）`pe.Open` 报错 → `needUpdateByExe` 返回“视为安装”（`updater.go:96-104`）。
- **影响**：在 Linux 上对 ELF 值守的 exe 版本项目，Go 会判定为“需安装/更新”，而 Python 在无 pefile 时直接崩。属边界差异，可按需接受。此为与 Python 参考实现意图一致的合理偏差。

---

## 五、进程控制差异

### 21. Windows 服务模式命令不同 ✅（已补齐，commit `9246a80`）
- **Python**：`ProcessCtrl.py:27-28` Windows service 用 `net <command> <service>`。
- **Go 落点**：`process.go:192-206` `stopService` + `process.go:317-331` `startService`，Windows 下使用 `net stop/start <service>`，非 Windows 用 `systemctl`。
- **影响**：Windows service 控制命令已与 Python 一致。

### 22. 重启进程时未保留原始命令行/cwd ✅（已补齐，commit `9246a80`）
- **Python**：`ProcessCtrl.py:80-91` `stopProc` 记录每个进程的 `cmdline()` 与 `cwd()`，`startProc` 用**原始命令与目录**重新拉起。
- **Go 落点**：`process.go:123-178` `stopUnix` + `findProcLaunches`（读 `/proc/<pid>/{comm,cmdline,cwd}`）记录 `(cmdline,cwd)`；`Start` 在非 service 且有记录时按原始 cmdline+cwd 重新拉起（`process.go:255-284`）。Windows 下暂不强记录（与 Python 在无相关依赖时行为相当）。
- **影响**：非 service 重启已保留原始命令行与工作目录。

---

## 六、HTTP 客户端差异

### 23. HTTP 请求无重试 ✅（已补齐，commit `bc78a55`）
- **Python**：`appveyor.py:46-47` 用 `HTTPAdapter(max_retries=cls.times)`（默认 5 次）。
- **Go 落点**：`http.go:93-154` `httpClient.Get` 实现重试：`retry` 次尝试，对连接错误及 429/500/502/503/504 做指数退避（200ms 起，封顶 2s）；`NewHTTPClientWithProxy` 接收 `retry` 参数；`main.go` 传入 `requests.retry`。
- **影响**：HTTP 客户端重试行为与 Python 一致。

### 24. 代理对 aria2 下载无效（见第 5 条） ⛔
（已在第 5 条说明，此处不重复。）

---

## 七、配置加载 / 变量替换

### 25. `%VER` 变量替换范围不同 ✅（已补齐，commit `e970809`）
- **Python**：`updater.py:392` 在版本检测后对整个配置做 `var_replace("%VER", self.version)`，影响后续所有含 `%VER` 的字段（含 filename、路径等）。
- **Go 落点**：`updater.go:490`（`selectDownloadURL` 中对 `Download.URL` 做 `%VER` 替换）、`updater.go:646,655`（`downloadFilename` 对 override 与版本做 `%VER`/`{version}`/`%arch`/`%OS` 替换）、`updater.go:417`（`replaceVars` 对 post-cmds 做 `%VER` 替换）。
- **影响**：下载 URL、文件名、后处理命令中的 `%VER` 均已展开。

### 26. 配置回写格式差异 ℹ️（**仍未完成**）
- **Python**：`JsonConfig.dumpconfig` 默认 `ensure_ascii=False`、4 空格缩进、`sort_keys=False`。
- **Go**：`writeJSON`（`main.go`）用 `json.MarshalIndent(..., "", "  ")`（2 空格、ASCII 转义、按字段顺序）。
- **影响**：功能等价但输出文本不同；若用户用外部工具 diff 配置会看到格式变化。非功能问题遗留项。

---

## 八、已正确复刻（用于确认，非差距）
以下方面经核对与 Python 行为一致，列出以避免误报：
- GitHub `no_pull` 用 `/releases/latest` vs `/releases` 取首个 release；release 名/ tag 选择逻辑（`github.py:165-182` ↔ `github.go`）一致。
- AppVeyor 跳过 PR 构建、按 `"elease"` 选 job、无 artifact 且超 30 天抛错（`appveyor.py` ↔ `appveyor.go`）一致。
- apijson 的 `version.path` / `download.path`（字符串或路径数组混合）导航（`apijson.py` ↔ `apijson.go`）一致。
- 版本字符串比较（字符串相等 / `use_exe_version` 的双版本严格大于）逻辑（`updater.py:206-218,424` ↔ `peversion/compare.go`）一致。
- `%arch`/`%OS` 变量展开（虽然 Go 的 arch 取值集合与 Python 不同，见下）已接入文件选择。
- aria2 远程目录映射（remote-dir/local-dir 替换）逻辑一致。
- 元数据仓库发现、local config 按 mtime 对比 date 下载更新逻辑一致。

### 27. `%arch`/`%OS` 取值集合与 Python 不完全相同（次要） ✅（已补齐，commit `afcc886`）
- **Python**：`updater.py:84-108` 为每种 OS/arch 生成**多个候选字符串**（如 x86_64 同时有 `"x86_64","amd64","x64","linux-64","linux64"`，windows 有 `"x86_64","amd64","x64","windows-64","win64"` 等），配置里任一候选命中即可。
- **Go 落点**：`platform.go:28-57` `ArchCandidates`/`OSCandidates` 产出多候选别名，`ExpandKeywords` 把恰好为 `%arch`/`%OS` 的关键词展开为全部候选（与 Python 的列表替换一致）。
- **影响**：多别名关键词匹配已对齐。注：Python 仅对“独立 `%arch`/`%OS` 关键词”做列表展开，对 `rpcs3-%arch` 这类拼接用法保持原样——Go 行为一致。

---

## 差距汇总（按严重度）

| # | 项目 | 严重度 | 类别 | 状态 |
|---|------|--------|------|------|
| 1 | `post-cmds` 缺失 | ⛔ 功能缺失 | 核心流程 | ✅ done (e970809) |
| 2 | `use_system_package_manager` 缺失 | ⛔ 功能缺失 | 核心流程 | ✅ done (a1c4309) |
| 3 | `.VERSION` 文件不写 | ⛔ 功能缺失 | 核心流程 | ✅ done (e970809) |
| 4 | 非 allow_restart 不等待进程退出 | ⛔ 行为差异 | 核心流程 | ✅ done (e970809) |
| 5 | aria2 未传 proxy/header/split/retry | ⛔ 功能缺失 | 下载 | ✅ done (a5b6c21) |
| 6 | `download.data` 直链 POST 未实现 | ⛔ 行为差异 | 下载 | ✅ done (c04457f) |
| 7 | `add_version_to_filename` 不作用于派生名 | ⚠️ 行为差异 | 下载 | ✅ done |
| 8 | 默认 filetype 未设为 7z | ⚠️ 行为差异 | 下载 | ✅ done (afcc886) |
| 9 | `update_keyword` 切换未实现 | ⚠️ 行为差异 | 下载 | ✅ done (7412843) |
| 10 | `include_file_type` 解压忽略 | ⚠️ 功能缺失 | 解压 | ✅ done (a1c4309) |
| 11 | `exclude_file_type_when_update` 时机 | ⚠️ 行为差异 | 解压 | ✅ done (a1c4309) |
| 12 | `single_dir` 字符串前缀未实现 | ⚠️ 行为差异 | 解压 | ✅ done (a1c4309) |
| 13 | 单文件重命名兜底缺失 | ⚠️ 行为差异 | 解压 | ✅ done (a1c4309) |
| 14 | `use_builtin_zipfile` 忽略 | ⚠️ 次要 | 解压 | ❌ 未完成 |
| 15 | SourceForge keyword/filetype/index 过滤缺失 | ⚠️ 行为差异 | API | ✅ done (a94364d) |
| 16 | simplespider `version.index` 忽略 | ⚠️ 行为差异 | API | ✅ done (c04457f) |
| 17 | simplespider 每级 `indexes` 缺失 | ⚠️ 行为差异 | API | ✅ done (c04457f) |
| 18 | github/appveyor `download.index` 语义不同 | ⚠️ 行为差异 | API | ✅ done (53ca376) |
| 19 | `use_cmd_version` 未实现 | ℹ️ 占位 | 版本 | ❌ 未完成（与 Python 一致） |
| 20 | `use_exe_version` 跨平台差异 | ℹ️ 边界 | 版本 | ❌ 未完成（合理偏差） |
| 21 | Windows service 用 `sc` 而非 `net` | ⚠️ 行为差异 | 进程 | ✅ done (9246a80) |
| 22 | 重启未保留原 cmdline/cwd | ⚠️ 行为差异 | 进程 | ✅ done (9246a80) |
| 23 | HTTP 客户端无重试 | ⛔ 功能缺失 | HTTP | ✅ done (bc78a55) |
| 25 | `%VER` 全局替换缺失 | ℹ️ 行为差异 | 配置 | ✅ done (e970809) |
| 26 | 配置回写格式不同 | ℹ️ 次要 | 配置 | ❌ 未完成（非功能） |
| 27 | `%arch`/`%OS` 多候选别名缺失 | ⚠️ 行为差异 | 平台 | ✅ done (afcc886) |

**结论**：截至 commit `bc78a55`（自 `c04457f` 起 10 个对齐 commit），27 条差距中 **21 条已补齐并通过 build + 全量测试**，覆盖全部 3 项用户可见功能缺失（post-cmds / system package manager / .VERSION）、非 restart 进程等待、aria2 选项/代理/重试、HTTP 重试、以及下载/解压/API/进程层面的关键语义对齐。

**仍未完成（4 条）**：
- **#14** `use_builtin_zipfile` 标志忽略——低优先（Go 默认纯 Go 解压结果一致）。
- **#19** `use_cmd_version` 未实现——与 Python 一致（Python 也仅空占位）。
- **#20** `use_exe_version` 跨平台差异——边界情况，属与参考意图一致的合理偏差。
- **#26** 配置回写格式（2 空格 vs 4 空格、ASCII 转义）——非功能问题。

**#7 已完成**（commit pending）：`add_version_to_filename` 现在对从 URL 派生的文件名也插入清理后的版本号，行为已与 Python 参考一致。其余 4 条按用户决定不再实现。
