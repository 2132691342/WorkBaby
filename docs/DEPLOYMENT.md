# docs/DEPLOYMENT.md · 构建、分发与运行时

## 1. 构建命令

```bash
wails dev                                        # 开发模式（前端 5173，后端内嵌 HTTP 随机端口）
wails build                                      # 生产构建（便携版）
wails build -nsis                                # 构建并打安装包
wails build -nsis -ldflags "-s -w" -trimpath     # 推荐：体积更小、路径更干净
```

## 2. 构建流水线

`wails.json` 定义：

| 项 | 值 |
|---|---|
| `frontend:install` | `npm install`（在 `frontend/` 下） |
| `frontend:build` | `npm run build` → `vue-tsc -b && vite build`，产物 `frontend/dist` |
| `outputfilename` | WorkBaby |
| `wailsjsdir` | `./frontend/src`（生成的绑定放这里） |
| `postBuildHooks`（windows/amd64） | `scripts/copy-runtimes.ps1 ${bin}` |

前端产物由 `main.go` 的 `//go:embed all:frontend/dist` 编进二进制，运行时经 AssetServer 提供；
`/files/**` 前缀转发到本地受管文件服务。

`copy-runtimes.ps1` 把 `runtimes/`（manifest + python / powershell 归档）复制到产物目录与 `build/windows/runtimes`，
**清单声明的归档缺失即构建失败**。

## 3. 版本号

单一来源 `wails.json` 的 `info.productVersion` → NSIS `INFO_PRODUCTVERSION` → 可执行文件版本资源。
前端 `package.json` 保持同版本。界面「版本」由 `GET /api/v1/meta/runtime` 提供。

## 4. 安装包

NSIS 脚本在 `build/windows/installer/project.nsi`（配 `wails_tools.nsh`、`build/windows/icon.ico`、`info.json`）。

产物：

| 文件 | 说明 |
|---|---|
| `build/bin/WorkBaby.exe` | 便携版 |
| `build/bin/WorkBaby-amd64-installer.exe` | 安装包 |

文件关联：`.md / .txt / .pdf` 双击可唤起 WorkBaby（配合单实例 IPC 传路径）。

## 5. 内置运行时

部分工具（技能脚本、命令执行）需要本机存在 python / powershell，但不应要求用户自己安装。
内置运行时把这些随安装包分发，首次启动时解压到数据目录。

### 5.1 清单

`runtimes/manifest.json`：`schemaVersion / platform / arch`，每项资产声明
`archiveFile / archiveType / stripComponents / sha256 / executableRelativePath / pathDirs / maxFiles / maxUncompressedBytes`。

| 资产 | 版本 | 格式 | 限额 |
|---|---|---|---|
| python | 3.12.13 | tar.gz | 6000 文件 / 240MiB；PATH 追加 `.` 与 `Scripts` |
| powershell | 7.5.2 | zip | 2000 文件 / 450MiB |

### 5.2 解压与安全

目标 `{home}/runtimes/{id}/{version}`；可执行文件已存在则跳过（幂等）。单资产失败只告警不阻塞启动（缺了只是相应工具不可用）。

| 约束 | 实现 |
|---|---|
| 路径穿越防护 | zip slip 用 `pkg.Within(root, sub)` 校验，越界直接报错 |
| 资源限额 | `maxFiles` 与 `maxUncompressedBytes` 用 `io.LimitReader` + 计数双重限制，超限中止 |

### 5.3 目录定位

`LocateBundledDir()` 依次尝试：显式指定 → exe 同级 `runtimes/` → 工作目录 `runtimes/`、`build/windows/runtimes/`、`assets/runtimes/`。

`BinDirs()` 动态检查目录是否存在；`EnhancePath(existing)` 把可用目录前置到**子进程** PATH
（只影响 WorkBaby 启动的子进程，不污染系统 PATH）。

排障：设置页「内置运行时状态」展示 `Status()`（已解压 / 缺失 / 版本），用于定位「工具报命令找不到」。

## 6. 运行时数据目录

数据根 `%APPDATA%\WorkBaby`（可用 `WORKBABY_HOME` 覆盖）：

```
config.yaml            主配置（含主密钥）
db/workbaby.db         SQLite（WAL）
logs/                  按等级分流的日志（10MiB×5 轮转）
runs/                  run 事件 JSONL（最近 50 个）
skills/                全局技能
sprites/               桌宠形象
memory/{sessionID}/    长期记忆
workspaces/{sessionID}/默认会话工作区
runtimes/{id}/{ver}/   内置运行时
files/                 受管文件
```

### 工作区沙箱

每个工作区目录下有 `.workbaby/` 沙箱（`runtime.Sandbox`）：`memory / snapshots / scripts / output / cache / tmp`。
会话绑定工作区时，记忆文件、文件快照、技能脚本都落在沙箱内（过程数据跟工作区走），
并写入 `.gitignore` 避免污染仓库。

## 7. 分发注意

| 项 | 说明 |
|---|---|
| WebView2 | 未安装时安装包会引导安装 |
| 主密钥 | 首次启动生成（`config.yaml` 的 `security.master_key_b64`），密钥用它 AES-256-GCM 加解密；**换机器或删配置会丢失已存密钥** |
| 全量卸载 | 需同时清理 `%APPDATA%\WorkBaby`（工作区内的 `.workbaby/` 沙箱不在其中） |
| 符号表 | `-ldflags "-s -w"` 后崩溃栈缺完整符号，排障需保留符号表副本 |

## 8. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| `-nsis` 单包分发 | 用户双击即装；WebView2 缺失时引导安装 | 安装包体积较大（含前端资源与内置运行时） |
| 主密钥随首启生成写入 `config.yaml` | 无需用户管理密钥 | 换机或删配置即丢失已存密钥（需重新录入模型 Key） |
| 内置运行时随包（python / pwsh） | 工具开箱可用，不依赖用户自备环境 | 首启解压耗时 + 包体积增加 |
| 解压带资源上限与路径校验 | 阻断 zip bomb 与越界写 | 超上限的资产需在 manifest 调整 |
| 工作区沙箱 `.workbaby/` | 过程数据跟工作区走，不污染仓库 | 用户清理工作区时会一并删除记忆与快照 |
