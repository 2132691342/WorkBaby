# AGENTS.md · 工程规范

> 本文件是代码风格、分包、依赖方向、工作流的唯一权威。
> 架构与模块设计见 `docs/`；冲突时以本文件为准。

---

## 1. 定位与技术栈

WorkBaby 是 **Windows 单机 AI 工作台**：Wails3 壳把内置 Node.js + pi 引擎 + 插件 +
技能 + MCP + Git/Python 运行时装进 `%LOCALAPPDATA%\WorkBaby` 沙箱，由 GUI 统一管理与调用。

外部装没装 node / pi 与本壳无关；壳内管理的 pi 不支持绕过 GUI 直接使用。

| 层 | 选型 | 版本 |
|---|---|---|
| 桌面壳 | Wails v3（无边框窗口，前端自绘标题栏，WebView2） | 3.1.x |
| 语言 | Go（模块 `WorkBaby`） | 1.26.x |
| 业务服务 | gin（业务 API + SSE 全走 HTTP，127.0.0.1 随机端口） | 1.12.x |
| 持久化 | 标准库 JSON 文件（`workbaby.json`，原子整写 + 互斥锁） | — |
| ID | ULID，带业务前缀（`oklog/ulid`） | 2.1.x |
| Agent 引擎 | pi（`@earendil-works/pi-coding-agent`，RPC 模式子进程） | 随 npm 最新 |
| 内置运行时 | node v24.21.0 / PortableGit / Python 3.13（zip 随包分发，首启解压） | — |
| 前端 | Vue 3.4.34 + Vite 5.2.12 + Element Plus 2.9.0 + Pinia 2.1.7 + Vue Router 4.3.2 + Axios 1.6.8 | — |
| 前端辅件 | UnoCSS 0.58.6 · ECharts 5.5.0 · CodeMirror 6 · Vue I18n 9.10.2 | — |
| 测试 | Go 原生 `testing`（仓库根 `tests/`） | — |

**平台**：只考虑 Windows；子进程一律隐藏窗口（走 `pkg/proc`，不裸用 os/exec）。

### 1.1 沙箱原则

- 引擎与工具链全部落在数据目录：`data\runtime\{node,PortableGit,python}`、
  `data\pi\{settings,models,mcp,auth,sessions,skills,extensions,plugins,npm}`、`data\pi-app`。
- 子进程 PATH **内置运行时在前、系统在后**（`pkg/env.go::SandboxEnv`），HOME 指向数据目录；
  npm 的 `cache` / `prefix` 同样锁进 `data\cache\`。
- **内置 node / git / python 取不到时直接报错，绝不回退系统同名命令**——
  回退等于拿用户环境跑包，沙箱就不存在了。探测有无用 `NodeReady()` 这类不抛的版本。
- pi 的整个家目录由 `PI_CODING_AGENT_DIR` 重定向；`PI_SKIP_VERSION_CHECK=1`
  与 `PI_TELEMETRY=0` 关掉引擎自带的远端探测与上报。
- **不设 `PI_PACKAGE_DIR`**：它改写的是 pi 找自己 `dist/` 与主题的根，指错引擎就起不来。
  插件安装根是 `pi/npm`，由 pi 自己的包管理决定。
- **不做 OAuth**：服务商一律 API Key，`auth.json` 里只有 `type: "api_key"`。
- 管理面（插件 / 技能 / MCP / 模型）只经 GUI 暴露，不对外提供 CLI。

---

## 2. 强制编码规范

### 2.1 目录结构

```
WorkBaby/
├── AGENTS.md / README.md / DESIGN.md
├── docs/                          # 架构 / 模块 / API / 数据 / 错误码 / 开发 / 部署
├── main.go                        # 装配根：内嵌前端产物 + 版本号，交给 shell.Run
├── backend/
│   ├── domain/                    # 纯类型：VO/REQ/RESP/DTO/DO + 错误码常量
│   ├── pkg/                       # 叶子工具：apperror/codes/ulid/fsutil/log/proc/env/paths/...
│   ├── repo/                      # JSON 存储文件：设置 KV、技能开关台账
│   ├── sse/                       # SSE hub：seq 发号、断线补帧、心跳
│   ├── api/                       # gin：server + 分组路由（薄壳，只解析与转发）
│   ├── service/                   # 业务编排：app/chat/file/git/session/skill/plugin/mcp/model/settings
│   │   ├── engine/                # 内置运行时路径与版本探测、npm 执行
│   │   ├── parser/                # pi 会话 JSONL → 条目树/活动分支
│   │   ├── chat/                  # rpc_client / frames / chat_service
│   │   └── platform/              # 开机自启 / 系统打开等 Windows 专属动作
│   └── shell/                     # 窗口 / 托盘 / 端点装配
├── frontend/src/{api,components,composables,locales,stores,types,utils,views,router}
├── assets/{runtime,skills}        # 编译期随包分发：运行时 zip、内置技能包
├── build/                         # wails3 打包配置与图标（Taskfile / config.yml / nsis）
└── tests/                         # Go 场景测试，按被测对象一文件
```

**开目录门槛**：有一组独立概念 + 独立生命周期才开目录，否则做现有模块里的一个文件。
每个文件 ≤ 500 行（强制），超了就拆。

### 2.2 依赖方向（强制）

```text
main(装配) ──► shell ──► api ──► service ──► {repo, engine/runtime, chat, parser, platform}
                                  │
                   service ──► pkg / domain（只读类型）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| pkg | 标准库 / 第三方纯工具 | domain / repo / service / api / shell |
| domain | pkg | 任何实现层 |
| repo | pkg / domain | service / api / shell |
| service | repo / pkg / domain | api 的路由层 |
| api | service / domain / pkg | 越过 service 直摸 repo |
| shell | wails / service（仅设置与端口回填） | api |
| 前端 | 自身 + `/api/v1` | 直摸文件系统 / 子进程 |

**组合根例外**：`main.go` 直接开库与起服务是装配本职；`shell/*.go` 读
settings 服务的关窗偏好同理。

### 2.3 domain 单文件多形态

一个域一个类型文件，VO / REQ / RESP / DTO / DO 同居，字段一律带业务前缀：

```go
// SessionRowVO 是会话列表行。
type SessionRowVO struct {
	// SessionID 会话 ID
	SessionID string `json:"session_id"`
	// SessionTitle 标题（首条用户消息截断）
	SessionTitle string `json:"session_title"`
}
```

| 后缀 | 用途 |
|---|---|
| `VO` | 给界面看的视图对象（RESP 载荷） |
| `REQ` | 入参（POST body / query） |
| `RESP` | 接口响应体 |
| `DTO` | 跨层传递的内部结构 |
| `DO` | 持久化行映射（repo 内） |

**禁止**：裸字段名（`Name` / `Code` 一律拒绝）；前端字段与后端 JSON 字段名不一致。

### 2.4 错误处理（强制 AppError，code 为字符串）

```go
// AppError 是业务错误的唯一形态。
type AppError struct {
	// Code 业务码（数字字符串）
	Code string
	// Message 人话描述（直接给用户看）
	Message string
	// Detail 补充上下文（stderr / 路径 / 上游响应原话）
	Detail string
}
```

- 业务码是**数字字符串**（`"1001"`）；HTTP 状态码是 number，信封 `code` 是 string
- HTTP 出口统一信封 `{ code, message, detail?, data }`，恒回 200，`code != "0"` 即业务失败
- 唯一例外是鉴权失败：回 401 且不带信封——那不是业务失败，是根本没进门
- 码值集中在 `pkg/codes.go`，与 `docs/API.md` 的错误码表一一对应；新增码先改文档再写代码

**错误码段位**：1000–1999 通用/文件/git；2000–2999 配置/DB；3000–3999 服务商/模型；
5000–5999 会话运行；7000–7999 外部进程；8000–8999 技能；9100–9199 MCP；9700–9799 引擎 RPC。

### 2.5 命名 / 注释 / 行数

| 类别 | 规则 |
|---|---|
| 文件名 | 全小写 + 下划线（`rpc_client.go`） |
| 类型 / 接口 | PascalCase，不加 I 前缀 |
| 函数 | camelCase（导出时首字母大写），常量 SCREAMING_SNAKE_CASE |
| JSON 字段 | snake_case + 业务前缀 |
| ID | `{PREFIX}_{26位ULID}`；现役前缀 `RUN` / `ASK` / `EVT` / `JOB` / `SES`，白名单在 `pkg/ulid.go` |
| 时间戳 | 一律毫秒 int64（`pkg.NowMs()`）；禁止 ISO 串进 DTO |
| 注释 | 文件头 1 行 `// Package x ...`；类型 / 字段 / 方法各 1 行说结论；连续注释 ≤ 3 行；只写「为什么」，不复述代码 |
| 行数 | 每文件 ≤ 500 行 |
| **禁止** | 注释出现外部项目名、过程痕迹（「旧版」「曾经」「改为」）；`uuid`；手拼 JSON；本地时区时间 |

### 2.6 子进程纪律

- argv 数组直传，永不进 shell；`SysProcAttr.HideWindow`（引 `pkg/proc`，不写十六进制）
- 超时必杀；stdout 严格按 LF 切帧（不用 `bufio.Scanner` 默认行为——U+2028/2029 会误切）
- 探测程序路径用 `pkg.Exists`，**禁止跑 `where` / `which`**（会闪控制台）
- 新增子进程出口自觉对齐 `pkg/proc.go` 的 `RunProc`
- 长驻子进程的 stdin 写入必须整行串行化：并发写会交错成半行

### 2.7 HTTP 与 API 路径约定（强制）

- **只允许 GET / POST**；动词靠路径段表达：`POST /sessions/delete` + `{ path }`
- 路径形状 `/{resource}/{action}`；查询参数 `?key=value`；前端统一 `/api/v1` 前缀
- 鉴权：HTTP 走 `X-Desktop-Token` 头；SSE 走 `?token=`（EventSource 带不了自定义头）

### 2.8 服务不变量（强制）

| 不变量 | 含义 |
|---|---|
| 会话文件是真相 | pi 的 JSONL 会话文件是唯一持久真相；界面状态重启后由回放重建 |
| 事件顺序是契约 | SSE 帧 `{seq, event, data}`，seq 单调；改 `chat:*` 词汇等于改界面行为 |
| 一个工作区一个引擎 | pi RPC 子进程按当前工作区绑定；切工作区即重建 |
| 看得见就点得动 | 提问等面板可写；回答经 `extension_ui_response` 直达引擎 |
| 写入只有一套 | 插件 / 技能 / MCP 的落盘与校验只有 service 一套，不写第二套 |
| 插件走 pi 原生 | 插件只经 `settings.json` 的 `packages[]` 声明；不自造注册表索引第二份真相 |
| 只读接口不写盘 | `GET` 路由不改磁盘状态；需要落盘的动作在自己的写接口或首启里做 |
| 启动不等重物 | 端口与界面先就绪；运行时解压、技能落位后台跑，进度经 `app:progress` 报 |
| 引擎不感知界面 | service/chat 只发 domain 词汇；翻译成 SSE 是 chat_frames 的职责 |
| 密钥不回显 | `api_key` 读出来恒为掩码；空串语义是「保留原值」 |

### 2.9 前端设计令牌（强制）

- **色值只在 `themes.css` 定义**：组件只允许 `var(--wb-*)`，禁止写死色值
- 尺寸走刻度：间距 `--wb-sp-*`、字号 `--wb-fs-*`、圆角 `--wb-radius-*`、动效 `--wb-dur-*`
- UnoCSS 刻度已接令牌（`gap-4` = `var(--wb-sp-4)`）；Element Plus 变量由 `ep-theme.css` 接桥
- **按钮五态齐全**：默认 / 悬停（放大 1.06）/ 按下（缩小 0.92）/ 禁用 / 加载；自绘走 `.wb-btn`
- 引用不存在的 CSS 变量会**静默失效**——那是丢样式，不是「不好看」
- 交互反馈：所有异步动作要有 loading / 过渡；空态给引导；错误给可读文案（后端 `message` 直显）

---

## 3. 测试规范

测试的价值在「改这里会坏那里」的地方；平凡断言不写。
详细清单见 `docs/DEV.md`。

**保留**：状态机与解析（会话树回放 / 分支 / 隐藏轮）、端到端长链路（鉴权 → 路由 →
service → 存储文件 → 信封）、内置清单与落位（插件声明改写 / 技能目录搬移）、
真实运行时链路（`live` 标签，手动跑）。

**不写**：复述框架行为的用例、单 getter、纯函数往返、覆盖率驱动用例。

- 位置：仓库根 `tests/`，一文件一个被测对象，`go test ./tests` 一秒内跑完
- 命名说清断言的是什么；断言带失败信息
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`
- 测不了的路径（需真实引擎 / 网络）放 `live` 标签后，不留假绿

---

## 4. 架构门禁

提交前必须过四道，一条不能少：

```bash
go build ./...                                    # 1 全包可编译
go vet ./...                                      # 2 静态检查
cd frontend && npx vue-tsc --noEmit -p tsconfig.json   # 3 前端类型检查
go test ./tests -count=1                          # 4 场景测试全绿
```

发布前另跑 `task build`（产出 `bin/WorkBaby.exe`）与 `task package`（NSIS 安装包，
需要 `makensis`）。

**硬性约束**：文件 ≤ 500 行；依赖方向单向；只用 GET / POST；字段带业务前缀；
信封恒 200；注释无外部项目名与过程痕迹；子进程隐藏窗口 + 超时杀。

---

## 5. 工作流

1. **先读约束**：动 domain 前想清楚字段前缀与错误码段位；动前端前想清楚令牌归属
2. **定位落点**：能进现有模块就别开新文件；纯逻辑进 `pkg/`（可测），编排进 `service/`
3. **实现**：注释只写「为什么」，不留过程痕迹
4. **验证**：跑 §4 的门禁
5. **同步文档**：改了架构 / API / 错误码 / 页面结构，对应 `docs/` 一起改
6. **提交**：一次提交只做一件事，提交信息写清做了什么与为什么

**新增依赖的门禁**：写清「为什么需要」「替代方案是什么」「代价是什么」，
并同步到 `docs/MODULES.md` 的取舍表与本文 §1 的技术栈表。
