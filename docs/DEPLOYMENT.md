# 构建与分发

## 产物

```
build/bin/
└── WorkBaby.exe    # 主程序（前端与内置运行时都已 embed，单文件可跑）
```

发布流程（GitHub Release）在此基础上额外产出 NSIS 安装器
（`WorkBaby-amd64-installer.exe`），安装器模板在 `build/windows/installer/`。

## 构建

```powershell
git lfs pull   # 首建必做：内置运行时归档经 LFS 托管
wails build
```

产物只有一个 exe：Python 与 PowerShell 的归档由 `go:embed` 打进主程序，
首次启动解压到用户数据目录。exe 因此明显变大（约 130MB+），
发布流程用「不到 100MB 视为归档未嵌入」兜底。

## 内置运行时

1. 归档在 `backend/runtime/bundled/`（`python-3.12.8-embed-amd64.zip` +
   `PowerShell-7.4.2-win-x64.zip`，Git LFS 托管，构建时嵌入 exe）
2. 启动装配期（`Handler.Startup` → `NewToolDeps`）自动解压到数据目录、打开
   `site` 并写 `.version` 标记，命中版本跳过解压；网络/文件系统问题不影响启动
3. 解压校验：逐条目路径检查（拒绝绝对路径与 `..`）、512MB 解压上限、
   版本切换先清旧目录、内嵌字节是 LFS 指针文本时按「包不可用」报错
4. 升级归档：从官方渠道下载新 zip 替换 `bundled/` 下的文件 →
   改 `pythonVersion` / `powershellVersion` → 算出新归档 SHA-256 填进对应的
   `*ArchiveSHA256` 常量 → `go test ./backend/runtime/` 守护三者一致

没有内置 Python 时应用照常运行：`python` 工具返回明确错误，
设置页状态灯显示未就绪，系统 PATH 上的 Python 会被自动采用。
内置 PowerShell 缺失时静默兜底系统 `powershell.exe`（win5.1 或 7），工具不断档。

## 用户数据

首启自动创建 `%APPDATA%/WorkBaby/`，全部状态都在这里：

| 内容 | 位置 |
|---|---|
| 数据库（会话 / 条目 / 用量 / 审批） | `workbaby.db`（WAL） |
| 配置（含 MasterKey） | `config.yaml` |
| 日志 | `logs/app.log`（全量）、`logs/warn.log`（warn 与 error） |
| 内置运行时解压产物 | `runtime/python/`、`runtime/powershell/` |
| 技能与知识库索引 | `skills/`、`builtin-skills/`、DB 内的分块 |
| WebView2 profile（纯缓存） | `webview/` |
| 单实例锁与 IPC 端口 | `app.lock`、`ipc.port` |

安装器把程序装进 `$PROGRAMFILES64`，建桌面与开始菜单快捷方式，并注册卸载器与
文件关联（md / txt）。**覆盖安装**会问一句要不要先清空本机数据（默认保留，
静默安装 `/S` 不删）；**卸载**同样问一句是否删除本机数据——选「否」数据留在
`%APPDATA%\WorkBaby`，下次装回来接着用。旧版本留在 `%APPDATA%\WorkBaby.exe`
的 WebView2 profile 属纯缓存，卸载时直接清掉。绿色版（只用 `WorkBaby.exe`）同样可用：
卸载就是删 exe，用户数据留在 `%APPDATA%` 里。

## 分发注意

- 单实例锁 + 本地 TCP IPC：二次启动会唤起已有窗口并转交文件路径
- 端口随机绑定 127.0.0.1，无外网暴露面
- API Key 以 AES-256-GCM 加密存库，MasterKey 在 config.yaml（首启生成）
- Wails 绑定面只有 `*main.App` 的 5 个系统能力方法（ForceQuit / 打开文件与目录
  对话框 / 自启读写）：`App` 持有而不是嵌入 `*api.Handler`，gin handler 不会被
  绑成 JS 方法；窗口控制由前端直接调 `wailsjs/runtime`，复制走 `navigator.clipboard`

## 取舍

- **归档嵌进 exe**：分发只有一个文件，没有「忘了拷 runtimes 目录」这种失败模式；
  代价是 exe 130MB+、构建期强依赖 LFS 拉取，以及首启多等一次解压。
- **不做自动更新**：没有更新服务与签名校验链路，安装包就是全部；
  代价是修 bug 要靠用户重新下载安装，覆盖安装时先杀常驻进程。
- **只发 Windows amd64**：单平台让托盘、WebView2、路径分隔符都不必抽象；
  代价是其他平台完全不可用。
