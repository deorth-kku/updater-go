# updater-go vs updater-rpc — 100% 复刻差距清单

> 目标（来自 `PLAN.md`）：100% 复刻 Python 版 `updater-rpc`（位于 `../updater-rpc`）的行为，
> 包括 API 后端检测、aria2 下载、解压、进程重启、以及完全兼容 `updater-config` 的 JSON 配置 schema。
>
> 本文档逐条列出当前 Go 实现与 Python 参考实现之间**尚未达到 100% 复刻**之处。
> 每条标注：**严重程度**（功能缺失 / 行为差异 / 次要）、**对应 Python 代码位置**、**Go 当前实现**、以及**影响**。

---

## 一、核心流程 / 功能缺失（会导致行为明显不同）

### 1. `post-cmds` 后处理命令——完全缺失 ⛔
- **Python**：`main.py:158-162`，更新成功后遍历 `pro["post-cmds"]`，依次做 `%PATH`/`%NAME`/`%DL_FILENAME` 替换后 `os.system(line)` 执行。
- **Go**：`updater.go:417-421` `getPostCmds()` 直接返回 `nil`（占位符，注释写明未实现）。
- **影响**：配置里带 `post-cmds` 的项目更新后不会执行任何后处理脚本。这是**用户可见的功能丢失**。

### 2. `use_system_package_manager` 系统包管理器安装——完全缺失 ⛔
- **Python**：`updater.py:474-495`，当该选项为真且 OS 为 linux 时，用 `dpkg -i` 或 `rpm -ivh` 安装 `.deb`/`.rpm`，并设 `DEBIAN_FRONTEND=noninteractive`；非 linux 跳过。
- **Go**：配置字段已解析（`project.go:131`），但 `extractor.go` 与 `updater.go` 中**无任何处理逻辑**，只会走普通解压。
- **影响**：带此选项的 deb/rpm 项目行为完全错误。

### 3. `.VERSION` 版本文件写入——缺失 ⛔
- **Python**：`updater.py:564-579` `updateVersionFile()`，非 `use_exe_version` 模式下会把版本字符串写入 `<path>/<name>.VERSION`。
- **Go**：`updater.go` 只把 `currentVersion` 写回 `config.json` 的 `projects[].currentVersion`，**从不写 `<name>.VERSION` 文件**。
- **影响**：依赖 `.VERSION` 文件存在性的外部脚本/工具会失效。

### 4. 非 `allow_restart` 时不等待进程退出 ⛔
- **Python**：`updater.py:602-609`，`run()` 默认 `popup=True`；当 `allow_restart=False` 时，若进程仍在运行，会弹窗警告并 `waitProc()` 等待进程结束，再解压。
- **Go**：`updater.go:293` 仅在 `AllowRestart` 为真时才处理进程；否则**直接下载+解压，从不检查/等待目标进程退出**（PLAN.md 第 276-277 行明确要求“log warning → wait for process exit → extract”，未实现）。
- **影响**：进程占用文件时，解压会失败或覆盖正在运行的文件。

---

## 二、下载 / 解压细节差异

### 5. aria2 下载未传递 `proxy` / `header` / `split` / `max-connection` / `continue` 等选项 ⛔
- **Python**：`updater.py:443-455`，`wget()` 传入 `proxy`、`retry`、以及 `basic.headers` 拼成的 `header` 列表，且全局 aria2 带 `split=16`、`max-connection-per-server=16`、`continue=true`。
- **Go**：`downloader.go:170-173` 的 `AddURI` 只设置了 `dir` 和 `out`，**没有** `header`/`proxy`/`split`/`max-connection-per-server`/`continue`/`retry`。
- **影响**：
  - 自定义 HTTP 头（`basic.headers`，如 NVIDIA 驱动配置）不会下发到 aria2 → 下载可能被拒。
  - 代理配置对 aria2 下载无效（只对 `api` 的 HTTP 客户端有效）。
  - 丢失并行分片与断点续传设置。

### 6. `download.data` 对直链 (`url`) 的 POST 行为未实现 ⛔
- **Python**：`simpleapi.py:48-54`，当 `download.url != ""` 且 `data` 非空时，对 `dlurl` 发 POST 并跟随 302/303 重定向取 `Location`。
- **Go**：`simple.go:108-142` `buildFromDirectURL` 只做 `followRedirect`（HEAD 请求），**从不 POST `data`**。
- **影响**：需要 POST 数据才能拿到真实下载地址的直链项目会拿到错误 URL。

### 7. `add_version_to_filename` 仅作用于 `filename_override`，对派生文件名无效 ⚠️
- **Python**：`updater.py:434-442`，无论文件名来自 `filename_override` 还是从 URL 派生，都会剥离末尾的 filetype 扩展名、插入清洗过的版本号、再拼回扩展名。
- **Go**：`updater.go:572-596` `downloadFilename` 只在 `FilenameOverride != ""` 时应用 `{version}`/`%arch`/`%OS` 替换；**从 URL 派生的文件名从不插入版本号**。
- **影响**：同一项目多次下载会产生同名的覆盖文件，与 Python 的“版本化文件名”不一致。

### 8. 默认 filetype 未设为 `["7z"]` ⚠️
- **Python**：`updater.py:317-319`，当 `download.filetype` 为空时默认设为 `["7z"]`。
- **Go**：`config.go:97-109` `hardcodedDefaults()` 中 `Filetype` 为空，`applyDefaults` 也未补默认值；`file_selector.go:25` 取 `Filetype.First()` 得到 `""`，**不做任何扩展名过滤**。
- **影响**：未配置 filetype 的项目在 Python 下只匹配 7z，在 Go 下匹配所有文件，可能选错资源。

### 9. `update_keyword` / `keyword` 在“重新更新”时切换逻辑未实现 ⚠️
- **Python**：`updater.py:354-359`，安装模式（`install`）或 `update_keyword` 为空时用 `keyword`+`exclude_keyword`；否则（再更新）用 `update_keyword`+`exclude_keyword`。
- **Go**：`file_selector.go:23-24` 只把 `download.keyword` 当作关键词，**从不读取 `download.update_keyword`**，也没有“安装 vs 更新”的切换。
- **影响**：区分首次安装与增量更新关键词的项目选错资源。

### 10. `include_file_type` 在解压阶段被忽略 ⚠️
- **Python**：`updater.py:233-251` + `file_sel`，若设置了 `include_file_type`，只**提取**匹配这些类型的文件。
- **Go**：`extractor.go` 的 `makeHandler` 只应用 `excludeSkipper`（排除），`file_selector.go` 里也没有 include 逻辑；`include_file_type` 字段解析了但**不参与实际文件选取**。
- **影响**：只想保留特定类型文件的配置会解出多余文件。

### 11. `exclude_file_type_when_update` 的应用时机不同 ⚠️
- **Python**：`updater.py:520-522`，仅在**非安装（再更新）**模式下才把 `exclude_file_type_when_update` 追加到排除列表。
- **Go**：`file_selector.go:26` 把 `ExcludeFileTypeWhenUpdate` 无条件传入 `Match`，即**安装模式也会排除**（且 Python 安装模式本就跳过解压阶段文件过滤）。
- **影响**：首次安装时可能多/少排除文件，与 Python 不一致。

### 12. `single_dir` 字符串形式（指定前缀目录）未实现 ⚠️
- **Python**：`updater.py:515-518`，`single_dir` 可以是字符串——表示归档内的固定前缀目录；为 `false` 时 `prefix=""`，为 `true` 时 `prefix=getPrefixDir()`。
- **Go**：`config.go:128` `BoolOrString`，但 `extractor.go:127-161` `extractWithSingleDir` 只用 `prefix.Bool()` 判断开关，字符串值仅用于额外 `prefixSkipper` 过滤，**没有“用字符串作为固定前缀目录”的能力**；且 Python 在 `single_dir` 为字符串但 `prefix==""` 时不进 single_dir 分支（Go 仍会进）。
- **影响**：用字符串指定归档内子目录的配置解压结果可能错位。

### 13. 单文件快速重命名兜底（gpu-z 情形）未实现 ⚠️
- **Python**：`updater.py:551-557`，当选出的文件只有 1 个时，把它重命名到 `image_name` 对应路径。
- **Go**：`extractor.go` 无此逻辑。
- **影响**：单文件工具（如 GPU-Z）更新后文件名不会被规范化为 `image_name`。

### 14. `use_builtin_zipfile` 标志被忽略 ⚠️
- **Python**：`updater.py:21-32`，该标志为真时强制用内置 `zipfile` 而非 libarchive。
- **Go**：`project.go:130` 字段已解析，但 `extractor.go` 始终用 `archives.Identify` 自动探测，忽略该标志。
- **影响**：通常无差异（Go 默认纯 Go 解压），但在需要强制 zipfile 的特殊情形不一致。

---

## 三、API 后端差异

### 15. SourceForge 版本号语义差异 ⚠️
- **Python**：`sourceforge.py:33` `self.version = str(file_info["date"])`，版本即 RSS `pubDate`。
- **Go**：`sourceforge.go:81,94` 同样存 `item.PubDate`。但 Python 的 `getVersion()` 直接返回该值，而 Go 的 `Release.Version` 也是 `item.PubDate`——**一致**。不过 Python 的 `filename_check` 还会按 keyword/filetype 匹配；Go 的 `Latest` **完全不调用任何 keyword/filetype 过滤**（`sourceforge.go:64-99` 直接拿第一个能解析日期的 item），而 Python `getDlUrl` 会遍历并按 `filename_check` 命中第 `index` 个。
- **影响**：带 keyword/filetype/index 的 SourceForge 配置在 Go 下会忽略过滤条件，选错资源。

### 16. simplespider 的 `version.index` 被忽略 ⚠️
- **Python**：`simpleapi.py:69-75` `re.findall(regex, src)[index]`，`version.index` 默认 0。
- **Go**：`simple.go:232-255` `extractVersion` 只取 `FindStringSubmatch(...)[1]`，**从不使用 `verCfg.Index`**。
- **影响**：版本正则命中多个结果时，Python 可取指定序号，Go 只能取第一个捕获组。

### 17. simplespider 多级 regex 的每级 `indexes` 未实现 ⚠️
- **Python**：`simpleapi.py:56-66`，`indexs` 是每个正则层级独立选择的序号（`indexs[lv]`），缺省为每级 `0`。
- **Go**：`simple.go:194-217` `extractURLFromPage` 对每级正则都用 `FindStringSubmatch` 的第 1 组，**没有每级 index 选择**。
- **影响**：需要每级取第 N 个匹配的多级爬虫配置选错 URL。

### 18. GitHub/AppVeyor 的 `download.index` 语义不一致 ⚠️
- **Python**：`appveyor.py:123` / `github.py:156` `return match_urls[index]`——返回**单个**位于 `index` 的资源。
- **Go**：`updater.go:461-469`，当 `Index > 0` 时执行 `matched[Index-1:]`——返回的是**从该位置起的切片（多个）**，而非单个元素；随后循环里取的是切片中第一个（即 `matched[Index-1]`）。
- **影响**：当 `index>0` 且命中多个资源时，Python 取第 `index` 个并开始下载该单个 URL；Go 会把第 `index-1` 个起的所有资源都保留，最终只取其第一个——多数情况结果相同，但 `index` 与 Python 的“从 0 计数取单个”语义不同，且在边界（`index` 越界）表现不同。

---

## 四、版本比较 / exe 版本

### 19. `use_cmd_version` 未实现（与 Python 一致，但仍是占位） ℹ️
- **Python**：`updater.py:417-422` 仅有空的 `try/pass`（标注 Not implemented yet）。
- **Go**：`updater.go` 的 generic 分支只做 `rel.Version == OldVersion` 字符串比较；`use_cmd_version` 字段解析了但未使用。
- **影响**：与 Python 行为一致（都不实现），但严格说不算“复刻该字段的语义”。低优先级。

### 20. `use_exe_version` 仅在 Windows PE 上有意义，跨平台解析行为差异 ℹ️
- **Python**：依赖 `pefile`（仅当安装时）；无 pefile 时该分支会异常。
- **Go**：`peversion.go` 用 `debug/pe` 跨平台解析，非 PE 文件（如 ELF）`pe.Open` 报错 → `needUpdateByExe` 返回“视为安装”（`updater.go:96-104`）。
- **影响**：在 Linux 上对 ELF 值守的 exe 版本项目，Go 会判定为“需安装/更新”，而 Python 在无 pefile 时直接崩。属边界差异，可按需接受。

---

## 五、进程控制差异

### 21. Windows 服务模式命令不同 ⚠️
- **Python**：`ProcessCtrl.py:27-28` Windows service 用 `net <command> <service>`。
- **Go**：`process.go:127-131,216-220` Windows service 用 `sc stop/start <service>`。
- **影响**：在 Windows 上对 service 类型项目的停止/启动命令不同；`net` 与 `sc` 语义不完全等价。

### 22. 重启进程时未保留原始命令行/cwd ⚠️
- **Python**：`ProcessCtrl.py:80-91` `stopProc` 记录每个进程的 `cmdline()` 与 `cwd()`，`startProc` 用**原始命令与目录**重新拉起。
- **Go**：`updater.go:363-377` 重启时只 `Start(ctx, exePath)`，`exePath = savePath/imageName`，**不传原始参数、不切回原始 cwd**。
- **影响**：需要带参数启动或用特定工作目录的服务/程序，重启后行为可能与 Python 不同。

---

## 六、HTTP 客户端差异

### 23. HTTP 请求无重试 ⛔
- **Python**：`appveyor.py:46-47` 用 `HTTPAdapter(max_retries=cls.times)`（默认 5 次）。
- **Go**：`http.go:67-87` `httpClient.Get` 单次请求，**无重试**；`NewHTTPClientWithProxy` 只设了代理和超时。
- **影响**：网络抖动时 Python 会自动重试，Go 直接报错，更新更稳定度下降。

### 24. 代理对 aria2 下载无效（见第 5 条） ⛔
（已在第 5 条说明，此处不重复。）

---

## 七、配置加载 / 变量替换

### 25. `%VER` 变量替换范围不同 ℹ️
- **Python**：`updater.py:392` 在版本检测后对整个配置做 `var_replace("%VER", self.version)`，影响后续所有含 `%VER` 的字段（含 filename、路径等）。
- **Go**：`replaceVars`（`updater.go:65-71`）只替换 post-cmds 里的 `%VER`（而 post-cmds 未实现，见第 1 条），**不对配置做全局 `%VER` 替换**；`downloadFilename` 里只处理 `{version}`（仅 override 场景）。
- **影响**：依赖 `%VER` 出现在下载 URL/路径/文件名中的配置在 Go 下不会展开。

### 26. 配置回写格式差异 ℹ️
- **Python**：`JsonConfig.dumpconfig` 默认 `ensure_ascii=False`、4 空格缩进、`sort_keys=False`。
- **Go**：`writeJSON`（`main.go:277`）用 `json.MarshalIndent(..., "", "  ")`（2 空格、ASCII 转义、按字段顺序）。
- **影响**：功能等价但输出文本不同；若用户用外部工具 diff 配置会看到格式变化（非功能问题）。

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

### 27. `%arch`/`%OS` 取值集合与 Python 不完全相同（次要） ℹ️
- **Python**：`updater.py:84-108` 为每种 OS/arch 生成**多个候选字符串**（如 x86_64 同时有 `"x86_64","amd64","x64","linux-64","linux64"`，windows 有 `"x86_64","amd64","x64","windows-64","win64"` 等），配置里任一候选命中即可。
- **Go**：`platform.go:12-47` 每个 arch/OS **只产出一个**字符串（如 amd64→`"amd64"`，windows→`"windows"`）。
- **影响**：用 `%arch`/`%OS` 且依赖 Python 多候选别名（如 `%OS==ubuntu`、`%arch==x64`）的 keywords 在 Go 下匹配失败。这是与 Python 实际运行行为的重要偏差。

---

## 差距汇总（按严重度）

| # | 项目 | 严重度 | 类别 |
|---|------|--------|------|
| 1 | `post-cmds` 缺失 | ⛔ 功能缺失 | 核心流程 |
| 2 | `use_system_package_manager` 缺失 | ⛔ 功能缺失 | 核心流程 |
| 3 | `.VERSION` 文件不写 | ⛔ 功能缺失 | 核心流程 |
| 4 | 非 allow_restart 不等待进程退出 | ⛔ 行为差异 | 核心流程 |
| 5 | aria2 未传 proxy/header/split/retry | ⛔ 功能缺失 | 下载 |
| 6 | `download.data` 直链 POST 未实现 | ⛔ 行为差异 | 下载 |
| 7 | `add_version_to_filename` 不作用于派生名 | ⚠️ 行为差异 | 下载 |
| 8 | 默认 filetype 未设为 7z | ⚠️ 行为差异 | 下载 |
| 9 | `update_keyword` 切换未实现 | ⚠️ 行为差异 | 下载 |
| 10 | `include_file_type` 解压忽略 | ⚠️ 功能缺失 | 解压 |
| 11 | `exclude_file_type_when_update` 时机 | ⚠️ 行为差异 | 解压 |
| 12 | `single_dir` 字符串前缀未实现 | ⚠️ 行为差异 | 解压 |
| 13 | 单文件重命名兜底缺失 | ⚠️ 行为差异 | 解压 |
| 14 | `use_builtin_zipfile` 忽略 | ⚠️ 次要 | 解压 |
| 15 | SourceForge keyword/filetype/index 过滤缺失 | ⚠️ 行为差异 | API |
| 16 | simplespider `version.index` 忽略 | ⚠️ 行为差异 | API |
| 17 | simplespider 每级 `indexes` 缺失 | ⚠️ 行为差异 | API |
| 18 | github/appveyor `download.index` 语义不同 | ⚠️ 行为差异 | API |
| 19 | `use_cmd_version` 未实现 | ℹ️ 占位 | 版本 |
| 20 | `use_exe_version` 跨平台差异 | ℹ️ 边界 | 版本 |
| 21 | Windows service 用 `sc` 而非 `net` | ⚠️ 行为差异 | 进程 |
| 22 | 重启未保留原 cmdline/cwd | ⚠️ 行为差异 | 进程 |
| 23 | HTTP 客户端无重试 | ⛔ 功能缺失 | HTTP |
| 25 | `%VER` 全局替换缺失 | ℹ️ 行为差异 | 配置 |
| 26 | 配置回写格式不同 | ℹ️ 次要 | 配置 |
| 27 | `%arch`/`%OS` 多候选别名缺失 | ⚠️ 行为差异 | 平台 |

**结论**：当前 Go 实现覆盖了 5 个 API 后端的主干逻辑、aria2 远程映射、PE 版本比较、配置 schema 90%+ 字段，但仍有 **3 项用户可见功能完全缺失（post-cmds / system package manager / .VERSION）**、**1 项核心流程行为缺失（非 restart 等待进程）**、以及若干下载/解压/API 层面的语义差异。要达到“100% 复刻”，建议优先补齐第 1–6、10、23、27 项。
