# AGENTS.md · 工程规范

> 本文件是代码风格、分包、依赖方向、工作流的唯一权威。
> 架构与模块设计见 `docs/`；冲突时以本文件为准。

---

## 1. 定位与技术栈

WorkBaby 是 **Windows 单机 AI 工作台**：Wails3 壳 + **进程内原生引擎**。
智能体循环、模型协议、工具执行、会话落盘全部由本仓库的 Go 代码实现。

用户配好一个服务商的 API Key 就能开始干活。不需要装引擎、不需要装 node、
不需要插件生态。机器上已有的 git / node / python 按需探测、直接调用。

| 层 | 选型 | 版本 |
|---|---|---|
| 桌面壳 | Wails v3（无边框窗口，前端自绘标题栏，WebView2） | 3.1.x |
| 语言 | Go（模块 `WorkBaby`） | 1.26.x |
| 业务服务 | gin（业务 API + SSE 全走 HTTP，127.0.0.1 随机端口） | 1.12.x |
| 持久化 | 设置台账用标准库 JSON（原子整写 + 互斥锁）；派生数据用 SQLite（纯 Go 驱动） | — |
| ID | ULID，带业务前缀（`oklog/ulid`） | 2.1.x |
| 智能体引擎 | 进程内原生实现（`backend/agent/`） | 本仓库 |
| 模型协议 | OpenAI 兼容 `/chat/completions` 流式 | — |
| 外部环境 | git / node / python 一律探测系统已装，不随包分发 | — |
| 前端 | Vue 3.4.34 + Vite 5.2.12 + Element Plus 2.9.0 + Pinia 2.1.7 + Vue Router 4.3.2 + Axios 1.6.8 | — |
| 前端辅件 | UnoCSS 0.58.6 · ECharts 5.5.0 · CodeMirror 6 · Vue I18n 9.10.2 | — |
| 测试 | Go 原生 `testing`（仓库根 `tests/`） | — |

**平台**：只考虑 Windows；子进程一律隐藏窗口（走 `pkg/proc`，不裸用 os/exec）。

### 1.1 边界（强制）

- 壳只写自己的数据目录：`data\{workbaby.json, workbaby.db, logs, webview, cache, sessions, skills, templates}`。
- **引擎在进程内**：`backend/agent/` 是纯 Go 包，只依赖标准库与 `pkg`，
  不知道 HTTP、界面与上层编排存在。没有子进程要拉起、没有握手协议、
  没有崩溃重启、没有版本探测。
- **运行时不随包分发**：git / node / python 一律探测系统已装，
  缺了就在应用页标出来。`assets\` 只剩内置技能包。
- 子进程环境只追加必须项（`pkg/paths.go::ChildEnv`）：`NO_COLOR=1`、
  `GIT_TERMINAL_PROMPT=0`。**不遮蔽 HOME、不改 PATH 顺序**——
  工具跑的是用户自己的环境，它该看见真实的 shell 与工具链。
- **不做 OAuth**：服务商一律 API Key。
- 管理面（技能 / MCP / 模型 / 工作区）只经 GUI 暴露，不对外提供 CLI。

---

## 2. 强制编码规范

### 2.1 目录结构

```
WorkBaby/
├── AGENTS.md / README.md / DESIGN.md
├── docs/                          # 架构 / 模块 / API / 数据模型 / 开发
├── main.go                        # 装配根：内嵌前端产物 + 版本号，交给 shell.Run
├── backend/
│   ├── domain/                    # 纯类型：VO/REQ/RESP/DTO/DO + 错误码常量
│   ├── pkg/                       # 叶子工具：apperror/codes/ulid/fsutil/log/proc/paths/...
│   ├── repo/                      # 持久化：JSON 设置台账 + SQLite 派生数据
│   ├── agent/                     # 原生引擎：轮次循环 / 模型适配 / 工具集 / 会话落盘
│   ├── web/                       # 联网能力：搜索适配器 / 正文抽取 / 地址安全
│   ├── index/                     # 工作区预索引：文件与符号抽取 / 模糊检索
│   ├── sse/                       # SSE hub：seq 发号、断线补帧、心跳
│   ├── api/                       # gin：server + 分组路由（薄壳，只解析与转发）
│   ├── service/                   # 业务编排：app/chat/plan/search/web/model/skill/mcp/file/git/session/template
│   │   ├── parser/                # 会话 JSONL → 条目树/活动分支/待办派生
│   │   └── platform/              # 开机自启 / 系统打开等 Windows 专属动作
│   └── shell/                     # 窗口 / 托盘 / 端点装配
├── frontend/src/{api,components,composables,locales,stores,types,utils,views,router}
├── assets/skills                  # 编译期随包分发：推荐技能包
├── build/                         # wails3 打包配置与图标（Taskfile / config.yml / nsis）
└── tests/                         # Go 场景测试，按被测对象一文件
```

**开目录门槛**：有一组独立概念 + 独立生命周期才开目录，否则做现有模块里的一个文件。
每个文件 ≤ 500 行（强制），超了就拆。

### 2.2 依赖方向（强制）

```text
main(装配) ──► shell ──► api ──► service ──► {repo, agent, web, index, pkg}
                                  │
                   service ──► pkg / domain（只读类型）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| pkg | 标准库 / 第三方纯工具 | domain / repo / service / api / shell |
| domain | pkg | 任何实现层 |
| repo | pkg / domain | service / api / shell |
| agent | pkg / domain | api / service / 界面 |
| web / index | pkg | api / service / agent |
| service | repo / agent / web / index / pkg / domain | api 的路由层 |
| api | service / domain / pkg | 越过 service 直摸 repo |
| shell | wails / service（仅设置与端口回填） | api |
| 前端 | 自身 + `/api/v1` | 直摸文件系统 / 子进程 |

**组合根例外**：`main.go` 直接开库与起服务是装配本职；`shell/*.go` 读
settings 服务的关窗偏好同理。

判定标准是**下层不知道上层存在**。`agent` 不知道 HTTP 与界面的存在，
所以它可以脱离壳单测；`api` 只解析与转发，所以业务规则全在 service 里。

### 2.3 domain 单文件多形态

一个域一个类型文件，VO / REQ / RESP / DTO / DO 同居，字段一律带业务前缀：

```go
// SessionRowVO 是会话列表行。
type SessionRowVO struct {
	// SessionId 会话 ID
	SessionId string `json:"session_id"`
	// SessionTitle 标题（首条用户消息截断，或改名后的名字）
	SessionTitle string `json:"session_title"`
}
```

| 后缀 | 用途 |
|---|---|
| `VO` | 给界面看的视图对象（RESP 载荷） |
| `REQ` | 入参（POST body / query） |
| `RESP` | 接口响应体 |
| `DTO` | 跨层传递的内部结构 |
| `DO` | 持久化行映射（repo 内 / SQLite 表） |

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

**错误码段位**：1000–1999 通用/文件/git；2000–2999 持久化；3000–3999 服务商/模型；
5000–5999 会话运行；7000–7999 外部进程；8000–8999 技能；9100–9199 MCP。

### 2.5 命名 / 注释 / 行数

| 类别 | 规则 |
|---|---|
| 文件名 | 全小写 + 下划线（`session_format.go`） |
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
- 探测程序路径用 `pkg.LookPath`（自己扫 PATH），**禁止跑 `where` / `which`**（会闪控制台）
- 新增子进程出口自觉对齐 `pkg/proc.go` 的 `RunProc`

### 2.7 HTTP 与 API 路径约定（强制）

- **只允许 GET / POST**；动词靠路径段表达：`POST /sessions/delete` + `{ path }`
- 路径形状 `/{resource}/{action}`；查询参数 `?key=value`；前端统一 `/api/v1` 前缀
- 鉴权：HTTP 走 `X-Desktop-Token` 头；SSE 走 `?token=`（EventSource 带不了自定义头）
- **CORS 中间件排在鉴权前面**：预检不带令牌，先鉴权会把预检打成 401

### 2.8 服务不变量（强制）

| 不变量 | 含义 |
|---|---|
| 会话文件是真相 | 会话 JSONL 是唯一持久真相；界面状态重启后由回放重建 |
| 会话头只写一次 | 整份重写在断电时会把唯一真相写坏；标题因此由条目派生 |
| 事件顺序是契约 | SSE 帧 `{seq, event, data}`，seq 单调；改 `chat:*` 词汇等于改界面行为 |
| 工具集是能力边界 | 模型能做什么取决于注册表；系统提示的工具清单从注册表现读 |
| 未登记按写处理 | 计划模式的判定规则；漏登记写工具的代价远大于漏登记读工具 |
| 写入只有一套 | 技能 / MCP / 模型的落盘与校验只有 service 一套，不写第二套 |
| 只读接口不写盘 | `GET` 路由不改磁盘状态；需要落盘的动作在自己的写接口里做 |
| 启动不等重物 | 端口与界面先就绪；会话初始化后台跑 |
| 引擎不感知界面 | agent 只发 domain 词汇；翻译成 SSE 是 `chat_stream` 的职责 |
| 密钥不回显 | `api_key` 读出来恒为掩码；空串语义是「保留原值」 |
| 路径有边界 | 工具与文件接口一律校验路径在允许的根目录内 |

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
service → 存储文件 → 信封）、引擎能力边界（分片拼装 / 工具准入 / 输出折叠）、
内置清单与落位（技能目录搬移与生命周期）、索引与联网的安全边界。

**不写**：复述框架行为的用例、单 getter、纯函数往返、覆盖率驱动用例。

- 位置：仓库根 `tests/`，一文件一个主题，`go test ./tests` 数秒内跑完
- 命名说清断言的是什么；断言带失败信息
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`
- 测不了的路径（需真实网络 / 真实上游）不写假绿用例

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

### 4.1 诊断工具的坑（重要）

静态诊断有时会报「某符号未定义」，而 `go build` 是通过的。这不是代码问题，
是诊断工具的缓存陈旧。判定顺序：**先信编译器**。

```bash
go build ./...      # 真相：编译不过就是编译不过
go vet ./...        # 真相：vet 过了就是过了
gopls check <文件>  # 权威的 gopls 自检（比 IDE 里的提示可靠）
```

已知的三种陈旧来源：

| 现象 | 原因 | 处理 |
|---|---|---|
| 新加的符号报未定义，但 `go build` 通过 | gopls 的导出数据缓存陈旧 | 删 `%LOCALAPPDATA%\gopls` 并重启 gopls 进程 |
| 同上，且刚建了新文件 | gopls 的文件索引还没收录新文件 | 同上（清缓存 + 重启 gopls） |
| gopls 与项目 Go 版本不一致 | 导出数据格式对不上 | 用项目同版本的 Go 重建 gopls：`GOTOOLCHAIN=go1.27.4 go install golang.org/x/tools/gopls@v0.24.0` |
| 报告里出现早已删除的文件 | 项目快照是旧的 | 删 `%USERPROFILE%\.pi-lens\projects\<项目>\cache\project-snapshot.*` 与 `review-graph.json.gz` |
| 上一轮的问题已经修了，报告还在 | 缓存的是「上一轮的结论文本」 | 删 `cache\turn-end-findings-last.json` 与 `code-quality-warnings.json` |

**不要因为诊断报错就改代码**：先跑 `go build`。真要改，也要先确认编译器也报同样的错。
真的被编译器的报错卡住时，先 `gofmt -w` 那个文件——未格式化的新文件会被 gopls 跳过。

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
