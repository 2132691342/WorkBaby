# 架构

WorkBaby 是 Windows 单机桌面应用：**Wails3 壳把全部依赖装进自己的沙箱**，
用 GUI 完全接管 pi 引擎、插件、技能、MCP 与运行时的生命周期。

## 进程拓扑

```
┌─────────────────────────── WorkBaby.exe ───────────────────────────┐
│                                                                     │
│  Wails 应用（Go 主进程）                                            │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ shell/  窗口 · 托盘 · 单实例 · 绑定面（Endpoint/选择目录/窗控）│  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ api/    gin 路由（薄壳：解析入参 → 调 service → 回信封）      │  │
│  │         127.0.0.1:<随机端口>  /api/v1/*                       │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ service/  业务编排：app chat file git session skill plugin    │  │
│  │           mcp model settings                                  │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ repo/        SQLite（设置 KV、技能开关台账）                  │  │
│  │ domain/      纯类型（VO/REQ/RESP/DTO/DO）                     │  │
│  │ pkg/         叶子工具（错误、路径、进程、日志、ULID、zip）    │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                              │ 子进程（隐藏窗口）                   │
│         ┌────────────────────┼────────────────────┐                 │
│         ▼                    ▼                    ▼                 │
│   node.exe pi CLI      node.exe npm-cli.js     git.exe / python.exe │
│   （RPC 模式，常驻）    （装包/卸包）          （工作区操作）        │
│                                                                     │
│  WebView2 渲染前端（Vue 3 + Element Plus）                          │
│  └── 只经 /api/v1 + window.wails 绑定，不碰文件系统与子进程         │
└─────────────────────────────────────────────────────────────────────┘
```

**两条通道分工**：业务全部走回环 HTTP（可 curl、可测）；Wails 绑定只暴露
系统能力（获取端点、选目录、窗控、托盘提示），不承载业务。

## 装配顺序

`shell.Run` 是装配根，顺序不能换：

```
1. resolveLayout    exe 同级有 assets\ → 安装态；否则往上找仓库根 → 开发态
2. pkg.SetLogDir    日志目录先就绪，后面每步才好排错
3. application.New  Wails 应用 + 单实例 + WebView2 数据目录锁进沙箱
4. 建窗（Hidden）   先不显示，避免装配阶段闪白屏
5. 建托盘           用户可能一启动就关窗，托盘必须先存在
6. ServiceStartup   开 SQLite → 装配 service → 起 gin（拿到端口）
7. Provision（后台） 解压运行时 → 落技能 → 声明插件 → 装 pi 与插件
8. 前端调 Endpoint  拿到 {port, token} 后建 SSE、拉首屏
9. 前端调 Ready()   由前端决定何时显形
```

第 7 步必须在**后台**跑：运行时解压 118MB，同步做会让窗口一启动就假死。
装配失败不弹窗卡死——原因留在 `Endpoint()` 的错误里，前端渲染成可读诊断页。

## 沙箱布局

```
%LOCALAPPDATA%\WorkBaby\           （开发态加 -dev 后缀）
├── workbaby.db                    # 壳的设置 KV + 技能开关台账
├── cache\npm\                     # npm 缓存（npm_config_cache）
├── cache\npm-global\              # npm 全局前缀（npm_config_prefix）
├── logs\                          # 按天滚动
├── webview\                       # WebView2 用户数据（卸载时一起删）
├── runtime\node\                  # 内置 Node.js 24.21
├── runtime\PortableGit\           # 内置 git + bash
├── runtime\python\                # 内置 Python 3.13
├── pi-app\node_modules\           # pi 本体（内置 npm --prefix 装的）
└── pi\                            # PI_CODING_AGENT_DIR：pi 的家
    ├── settings.json              # 模型 + packages[] + npmCommand
    ├── models.json / auth.json / mcp.json
    ├── npm\node_modules\           # 插件安装根（pi 自己的约定）
    ├── sessions\                  # 会话 JSONL（唯一持久真相）
    ├── skills\                    # 启用态技能
    └── skills_disabled\           # 停用态技能（目录搬移即失联）
```

**为什么整个 pi 的家要重定向**：pi 默认读写 `~/.pi`。不重定向，壳内装的
插件会与用户自己装的 pi 混在同一目录，卸载壳就删了用户的东西。

## 引擎生命周期

| 阶段 | 行为 |
|---|---|
| 首启 | 内置 npm 装 `@earendil-works/pi-coding-agent@latest` 到 `pi-app` |
| 绑定 | 一个工作区 ⇔ 一个 pi RPC 子进程；切工作区即重建 |
| 通信 | stdin/stdout 行分隔 JSON，命令带 request_id，事件带 event 名 |
| 中断 | 用户点停止 → 发 `abort`；超时未回则杀进程并重建 |
| 退出 | 先发优雅退出，超时再杀；托盘挂起不算退出 |

引擎的 stdout 严格按 LF 切帧（不用 `bufio.Scanner` 默认切分——U+2028/2029 会误切）。

## 插件与技能的真相来源

| 对象 | 唯一真相 | 壳做什么 |
|---|---|---|
| 插件 | `pi\settings.json` 的 `packages[]` | 改写数组（启用 = 裸串，停用 = `{source, autoload:false}`）、用内置 npm 装包 |
| 技能 | `pi\skills\` 与 `pi\skills_disabled\` 的目录位置 | 搬目录、写文件、从 `assets\skills\` 补位 |
| MCP | `pi\mcp.json`（用户级）+ 工作区 `.pi\mcp.json` | 读写 JSON、校验 |
| 模型 | `pi\models.json` + `auth.json` | 读写、探测上游可用性 |

**壳不建第二份注册表**。内置清单（`plugin_catalog.go` / `skill_catalog.go`）只是
「随包分发哪些、默认开哪些」的策展表，不是运行时状态——状态一律现场读盘。

## 安全边界

- 令牌每次启动随机生成，只经 Wails 绑定交给自己的 WebView；HTTP 走
  `X-Desktop-Token`，SSE 走 `?token=`（EventSource 带不了自定义头）
- 服务只监听 `127.0.0.1`，不监听 `0.0.0.0`
- 文件接口一律校验路径在允许的根目录内（`CodePathOutside`）
- 密钥读出来恒为掩码；`api_key` 传空串语义是「保留原值」
