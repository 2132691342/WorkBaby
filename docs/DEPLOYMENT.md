# 构建与分发

## 产物

```
build/bin/
└── WorkBaby.exe    # 主程序（前端与内置运行时都已 embed，单文件可跑）
```

发布流程（GitHub Release）在此基础上额外产出 NSIS 安装器
（`WorkBaby-amd64-installer.exe`），安装器模板在 `build/windows/installer/`。

## 流水线

两条 GitHub Actions，分工是「日常门禁」与「出包」，两条都跑与本地同一个入口：

| 工作流 | 触发 | 做什么 |
|---|---|---|
| `.github/workflows/ci.yml` | push main / PR / 手动（**tag 不触发**） | 装 Go + Node + 依赖 → 跑 `pwsh scripts/check.ps1 -Full`（gofmt / 编译 / vet / 依赖边界 / 全量测试 / 前端构建）；失败时脚本自己把「哪条链坏了、错在哪一行」写进 job summary |
| `.github/workflows/release.yml` | tag `v<major>.<minor>.<patch>` / 手动 | 轻量静态门禁（gofmt / vet / `go build` / 依赖方向，不重跑已由 ci 门过的测试矩阵）+ 校验运行时归档是真文件 → `wails build -clean -nsis` → 校验产物体积 → 建 GitHub Release 并上传安装器 |

**「推 main 不发版、只有推 tag 才发版」是靠两个工作流各自的触发条件锁死的**，
两条同时生效：

- `ci.yml` 的 `push` 显式写了 `branches: [main]` **与** `tags-ignore: ['v*']`。
  只写 `branches` 时，GitHub 的过滤器是「或」关系，tag 推送仍会命中 `push` 触发器；
  补上 `tags-ignore` 才能把 tag 彻底排除在这条之外
- `release.yml` 只认 `push.tags`，且用 `v[0-9]+.[0-9]+.[0-9]+` 而不是 `v*`：
  形如 `v1.4.0` 的 tag 才会发版，顺带挡掉误推的 `v-next` 之类分支式 tag
- 建 Release 那一步额外判一次 `startsWith(github.ref, 'refs/tags/v')`，
  防的是「tag 事后被移动」——那时产物与版本号就对不上了

其余约定，改动流水线前要知道：

- **门禁与出包分层**：日常质量门禁集中在 `ci.yml`（跑 `check.ps1 -Full`）；`release.yml` 只做静态编译门禁，不重跑已由 main 门过的测试与独立前端构建（`wails build` 自己会跑前端）
- **两个 job 都跑 `windows-latest`**：目标平台只有 Windows amd64，门禁与出包必须在同一套系统调用上验
- **checkout 必须 `lfs: true`**：运行时归档走 Git LFS，没拉下来时 `go:embed` 嵌的是指针文本，
  exe 能构建、用户首次启动才炸；`release.yml` 在打包前逐份读文件头确认不是指针
- **tag 指向的 commit 必须先随 main 推上去**：孤立 commit 会让 tag 触发的 checkout 失败
- **版本号与 PI 保持一致**：WorkBaby 是 PI 的外壳，tag 与 PI 的版本对齐才说得清兼容性

发版顺序：确保 main 上的 `ci.yml` 全绿 → 打 tag → 观察 `release` workflow → Release 里出现安装器。
`release` 的编译与出包失败时，`workbaby-windows-debug` 诊断包会带上 `build/bin/` 的全部产物，保留 7 天。

## 构建

```powershell
git lfs pull   # 首建必做：内置运行时归档经 LFS 托管
wails build
```

产物只有一个 exe：Node 与 Python 的归档由 `go:embed` 打进主程序，
首次启动解压到用户数据目录。exe 因此明显变大（约 90MB），
发布流程用「不到 55MB 视为归档未嵌入」兜底。

## 内置运行时

1. 归档在 `backend/runtime/bundled/`（`node-v24.21.0-win-x64.zip` +
   `python-3.12.8-embed-amd64.zip`，Git LFS 托管，构建时嵌入 exe）
2. 启动装配期（`Handler.Startup` → `reportRuntime`）探测内置运行时；**真正解压发生在首次需要它的时候**
   （选中内置 Node / 首次装 PI），解压后打开 `site` 并写 `.version` 标记，命中版本跳过；
   网络 / 文件系统问题不影响启动
3. 解压校验：逐条目路径检查（拒绝绝对路径与 `..`）、512MB 解压上限、
   版本切换先清旧目录、内嵌字节是 LFS 指针文本时按「包不可用」报错
4. 升级归档：从官方渠道下载新 zip 替换 `bundled/` 下的文件 →
   改 `nodeVersion` / `pythonVersion` → 算出新归档 SHA-256 填进对应的
   `*ArchiveSHA256` 常量 → `go test ./backend/runtime/` 守护两者一致

内置运行时是**兜底**：系统已有合规版本（Node ≥ 22.19.0、Python 3.x）时直接复用，
没有才解压内置那份。两个都没装也不阻断启动，只有对应的工具会报错。
PowerShell 从不内置，走系统自带的 5.1。

## 用户数据

首启自动创建 `%APPDATA%/WorkBaby/`，全部状态都在这里：

| 内容 | 位置 |
|---|---|
| 数据库（只有界面设置 KV） | `workbaby.db`（WAL） |
| 配置（工作目录等固定项） | `config.yaml` |
| 日志 | `logs/app.log`（全量）、`logs/warn.log`（warn 与 error） |
| 内置运行时解压产物 | `runtime/node/`、`runtime/npm-global/`、`runtime/python/` |
| 运行时落盘记录 | `runtime/shell.json`（命令执行环境改了什么）、`runtime/npm-cache/` |
| 终端里的 `pi` 入口 | `bin/pi.cmd`（同时把这个目录挂进当前用户 PATH） |
| 人设（附加系统提示） | `persona.md` |
| 自定义背景图副本 | `background/` |
| WebView2 profile（纯缓存） | `webview/` |
| 单实例锁与 IPC 端口 | `app.lock`、`ipc.port` |

**会话、消息、用量、模型、技能、插件都不在这里**：它们是 PI 的状态，住在
`~/.pi/agent/`（或用户设的 `PI_CODING_AGENT_DIR`）。卸载 WorkBaby 不会清掉它们——
这是设计目标：换掉外壳不该丢用户的对话与配置。

安装器把程序装进 `$PROGRAMFILES64`，建桌面与开始菜单快捷方式，并注册卸载器与
文件关联（md / txt）。数据去留在**组件页勾选框**里表达，不弹窗：

- 安装页的可选项「清空旧数据」与卸载页的「删除本机数据」默认都不勾；
- 静默安装 / 卸载（`/S`）跳过页面，未勾选的 section 不执行，永不删数据；
- `webview/` 里的 WebView2 profile 属纯缓存，卸载时直接清掉；
- `~/.pi/agent` 属于 PI，任何情况下都不删。

绿色版（只用 `WorkBaby.exe`）同样可用：卸载就是删 exe，
用户数据留在 `%APPDATA%` 里。

## 分发注意

- 单实例锁 + 本地 TCP IPC：二次启动会唤起已有窗口并转交文件路径
- 端口随机绑定 127.0.0.1，无外网暴露面
- Wails 绑定面只有 `*main.App` 的 7 个系统能力方法（端口查询 / 强制退出 / 打开文件、
  目录与保存对话框 / 自启读写）：`App` 持有而不是嵌入 `*api.Handler`，gin handler 不会被
  绑成 JS 方法；窗口控制由前端直接调 `wailsjs/runtime`，复制走 `navigator.clipboard`

## 取舍

- **归档嵌进 exe**：分发只有一个文件，没有「忘了拷 runtimes 目录」这种失败模式；
  代价是 exe 90MB、构建期强依赖 LFS 拉取，以及首次用到内置运行时的那一次要等解压。
- **不做自动更新**：没有更新服务与签名校验链路，安装包就是全部；
  代价是修 bug 要靠用户重新下载安装，覆盖安装时先杀常驻进程。
- **只发 Windows amd64**：单平台让托盘、WebView2、路径分隔符都不必抽象；
  代价是其他平台完全不可用。
