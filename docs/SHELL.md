# 桌面壳 · SHELL

Tauri 2 壳只承担系统能力：窗口、托盘、单实例、对话框、自启、剪贴板、外链打开。
**业务一律不走壳**——壳内不写业务逻辑。

## 进程模型

```
┌─────────────────────────────────────────────┐
│ WorkBaby 进程                                │
│  ┌───────────────────────────────────────┐  │
│  │ src-tauri（壳）                        │  │
│  │  · WebView2 窗口（Frameless）          │  │
│  │  · 托盘 / 单实例 / 自启 / 对话框       │  │
│  │  · 剪贴板 / 外链打开（插件）           │  │
│  └───────────────┬───────────────────────┘  │
│                  │ invoke / listen          │
│  ┌───────────────▼───────────────────────┐  │
│  │ wb-api：axum HTTP + SSE hub            │  │
│  │  监听 127.0.0.1:0（随机端口）           │  │
│  └───────────────┬───────────────────────┘  │
│                  │ HTTP + SSE               │
│  ┌───────────────▼───────────────────────┐  │
│  │ frontend（Vue）                        │  │
│  └───────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

**端口握手**：壳启动后绑定随机端口，通过 Tauri 事件 `app:ready` 把端口带给前端，
前端据此设 baseURL。启动失败则发 `app:startup-error`，前端渲染失败页。

用随机端口而不是固定端口：避免与用户机上的其它服务撞端口，
也避免残留进程占着端口导致新实例起不来。

## 窗口

- Frameless：标题栏由前端自绘（`layout/TitleBar.vue`），与界面同一套令牌
- 窗控三键（最小化 / 最大化 / 关闭）走壳命令 `window_ctl`
- 关闭策略可配：默认收进托盘（并发 `app:to-tray`），设置项 `close_action=quit` 时直接退出

## 托盘

托盘图标常驻，右键菜单：

| 项 | 行为 |
|---|---|
| 显示 WorkBaby | 唤起窗口并置前 |
| 新建对话 | 发 `app:new-session` 事件，前端切到新会话 |
| 退出 | 真正退出进程（绕过「关窗收托盘」策略） |

## 单实例

第二次启动时不新开进程，而是把已有窗口唤到前台（`tauri-plugin-single-instance`）。

## 自启

Windows 注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，
不需要管理员权限。读写都走壳命令（`autostart_get` / `autostart_set`），
界面只传布尔值。

## 壳命令

| 命令 | 用途 |
|---|---|
| `server_port` | 取 HTTP 端口（`app:ready` 之外的第二条路径） |
| `pick_file` / `pick_file_save` / `pick_folder` | 打开 / 另存为 / 选目录对话框 |
| `autostart_get` / `autostart_set` | 自启开关读写 |
| `window_ctl` | 最小化 / 最大化 / 关闭 |
| `quit_app` | 真正退出 |

对话框走壳命令（内部用 `rfd`），组件不直接碰 Tauri API。
壳另注册了 clipboard-manager 与 opener 插件——剪贴板读写与外部链接打开走这两个插件，
不另写命令。

## 文件对话框的边界

对话框返回的是**绝对路径**。后端处理时一律过 `wb_pkg::realpath` 归一
（短名路径、符号链接）再判是否在工作区内——界面给的路径不能直接信任。

## 前端加载

- 生产：`frontend/dist`（构建产物打进安装包）
- 开发：`devUrl` 指向 Vite 开发服务器

## 启动顺序

```
main()
  → 装配 ShellState（启动结果 / 失败原因 / 退出信号）
  → 建单实例监听 + 窗口
  → spawn HTTP 启动（随机端口，装配 Services / 功能工具 / MCP），随后建托盘
  → 发 app:ready（带端口）
  → 事件循环
```

启动不等重物：内置运行时解压与 MCP 连接都在后台跑，端口与界面先就绪，
进度经 `GET /runtime` 与 `GET /mcp` 报出。

## 独立调试入口

`cargo run -p wb-api --bin wb-api-standalone [工作目录]` 不起桌面壳，
直接起 HTTP 服务并打印端口——浏览器里调前端用（配合 `localStorage.wb_dev_port`）。

## 数据目录

`%APPDATA%\WorkBaby`：

| 内容 | 说明 |
|---|---|
| `settings.db` | SQLite：设置 KV / 会话索引 / 用量统计 / 三类开关 |
| `sessions\` | 会话 JSONL，按工作区分目录 |
| `skills\builtin\` | 解压的内置技能包 |
| `skills\user\` | 用户技能 |
| `features\` | 各功能的自有存储与库文件 |
| `models.json` | 服务商与模型配置 |
| `auth.json` | 凭据 |
| `mcp.json` | MCP 服务器配置 |
| `runtime\PortableGit\` | 内置 Git Bash / git / GNU 工具 |
| `runtime\python\` | 内置 Python 3.13 与 pip |
| `logs\` | tracing 日志，按天分文件 |

工作区在启动时解析一次（见 [WORKSPACE.md](WORKSPACE.md)）：设置 KV `workspace`
里存过且目录仍存在就用之，否则用壳传入目录并写回 KV；运行期可经
`POST /workspace/switch` 热切换。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 业务走 HTTP 而非全部 invoke | 前端 api 层统一，浏览器可直接调试 | 多一层端口握手 |
| 随机端口而非固定端口 | 不与用户机上的服务撞端口 | 每次启动都要重新握手 |
| 壳内不写业务 | 业务逻辑集中在一处，可测 | 壳与后端之间多一层调用 |
| 自启写 HKCU | 不需要管理员权限 | 仅当前用户生效 |
| Frameless + 前端自绘标题栏 | 与界面同一套视觉语言 | 原生窗口行为要自己实现（拖拽、缩放） |
| 文件对话框走壳命令（rfd） | 与窗控等系统能力同一条 invoke 通道，前端统一收口 | 多一层命令封装 |
