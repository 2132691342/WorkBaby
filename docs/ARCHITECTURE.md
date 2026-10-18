# 架构设计

WorkBaby 是一个 Windows 单进程桌面壳（Wails 出窗口、gin 出本地 HTTP），
**Agent 内核是外部的 PI 进程**。本机这层只做四件事：发现与安装 PI、
拉起并守护它、编解码它的 RPC 协议、把它的会话事件翻译成界面能读的 SSE。
SQLite 存 WorkBaby 自己的账（会话元数据、用量、运行记录、技能、知识库、记忆）。

一句话定位：**PI 负责跑 Agent，WorkBaby 负责让你看见并控制它怎么跑。**

工程规范（编码 / 依赖 / 工作流）见 `AGENTS.md`；数据模型见 `docs/DATA-MODEL.md`，
接口契约见 `docs/API-CONTRACT.md`。

## 1. 技术栈与形态

| 层 | 选型 | 为什么 |
|---|---|---|
| 语言 | Go 1.24 单 module | 并发模型贴合「流式事件 + 多子进程守护」的形态；单二进制分发 |
| 桌面壳 | Wails v2（Frameless） | 系统 WebView 复用，不打包 Chromium；前端自绘标题栏 |
| HTTP / 实时 | gin + SSE | 业务 API 与事件流统一走 HTTP，一个通信面 |
| 存储 | GORM + SQLite（WAL + FTS5 trigram，pure-Go） | 本机单用户零运维；FTS5 解决中文分词检索 |
| 前端 | Vue 3 + TS + Vite + Pinia + 原生 CSS 令牌 | 无 UI 组件库，控件全自绘，色值 / 字体只在 `themes.css` |
| **内核** | **PI（`pi --mode rpc`，Node 子进程）** | Agent 能力由 PI 演进，本机不追版本 |
| **运行时** | **Node 24 / Python 3.12，内置；PowerShell 走系统** | 双击即用，用户不装环境 |

## 2. 分层与依赖方向

```
WebView (Vue3 SPA)
   │  axios（HTTP）               ▲ SSE /api/v1/events
   ▼                              │
backend/server ──► backend/api ──► backend/service ──► 能力域
                                                        pi / skill / knowledge / git / history / archmap
                                                          │
                                                          ▼ (stdio JSONL)
                                                       PI 子进程
                                     backend/repo ──► backend/domain
backend/pkg：叶子工具包（错误 / ID / 日志 / 加密 / 路径），任何层可用，自身不依赖业务包
系统集成（不进依赖链）：backend/tray、backend/singleinstance、backend/config、backend/db
```

依赖方向由 `tools/check-boundaries` 门禁强制（`go run ./tools/check-boundaries`，
读编译器视角的真实 import 关系）。

要点：`backend/pi` 不依赖 api / service / server / domain / repo——
它只认 `pkg` 与 `runtime`，通过接口把事件交给上层。
因此它可以被一个假 PI 进程驱动着做全链路测试。

### 2.1 模块与规格对照

概要在本文件，实现细节在对应规格：

| 模块 | 规格 |
|---|---|
| PI 子进程监督（进程 / 协议 / 翻译） | [specs/17](../specs/17-pi-supervisor.md) |
| 环境引导（Node / PI 安装 / config 互通） | [specs/18](../specs/18-pi-bootstrap.md) |
| 会话模型（两处存储的分工） | [specs/02](../specs/02-session.md) |
| 系统提示与设置（`defaultTools` / 拓展模块注入） | [specs/07](../specs/07-prompt-and-settings.md) |
| `skill` | [specs/03](../specs/03-skill.md) |
| `knowledge` | [specs/04](../specs/04-knowledge.md) |
| `git` 工作区版本视图 | [specs/09](../specs/09-workspace-git.md) |
| `runtime` 内置运行时 | [specs/01](../specs/01-runtime.md) |
| 桌面壳（窗口 / 托盘 / 单实例） | [specs/05](../specs/05-desktop-shell.md) |
| 用量与上下文水位 | [specs/08](../specs/08-usage-and-context.md) |
| 应用内帮助页（内置文档 + `/docs`） | [specs/06](../specs/06-user-manual.md) |
| MCP 外部工具接入 | [specs/10](../specs/10-mcp.md) |
| 运行观测与检查点 | [specs/15](../specs/15-runs-and-checkpoints.md) |
| 评估与回归 | [specs/11](../specs/11-eval-regression.md) |
| `history` 通用会话历史 | [specs/12](../specs/12-universal-history.md) |
| `archmap` 项目架构图 | [specs/13](../specs/13-arch-map.md) |
| 工作区文件面板（`/files` 列举） | [specs/20](../specs/20-workspace-files.md) |
| Harness 控制面（runs / checkpoints / evals / stats 总纲） | [specs/21](../specs/21-harness-console.md) |
| 记忆 | [specs/14](../specs/14-memory.md) |
| 子代理 | [specs/16](../specs/16-subagent.md) |
| 插件（PI 扩展与包） | [specs/22](../specs/22-extensions.md) |

## 3. 一次对话的生命周期

1. 前端 `POST /chat/send` → `ChatService.Send`：校验 → 落一条 user 消息意图 → 返回 runID
2. service 检查该工作区的 PI 进程：没有则 spawn，有的直接复用
3. 会话首次使用 → `new_session` 拿 `sessionId` 与 `sessionFile` 回写 SQLite
4. 发 `prompt` 命令（空闲）或 `steer` / `follow_up`（流式中），响应只表示"已接受"
5. PI 开始吐事件：`agent_start` → `turn_start` → `message_update`（逐字）→
   `tool_execution_*` → `turn_end` → …… → `agent_end` → `agent_settled`
6. 翻译层把每条事件转成 `chat:*` 并经 `service.Emitter`（分配 seq）→ `server.Hub` → SSE
7. `message_end` 时记 usage；`turn_end` 时更新上下文水位与运行记录
8. `agent_settled` → 记 `chat:done`，运行记录收尾，检查点落位

**`agent_end` 不是结束**：重试、溢出恢复、压缩、插话都可能接在它后面。
只有 `agent_settled` 表示「不会再自动继续了」。

## 4. 模块设计

### 4.1 backend/pi —— PI 监督

**设计**：一个工作区一个 PI 进程，生命周期由 `Supervisor` 管。
`Client` 负责 JSONL 编解码与命令关联，`Translator` 负责事件映射。
三者分开是因为它们的失效方式不同：进程死了要重启，协议错了要忽略，
翻译错了要改映射表——混在一起就无法判断是哪一类。

**为什么不做成自研内核**：那意味着永久追赶 PI 的每次工具与协议变更，
而收益只是省一个 Node 依赖。

**关键不变量**：
- stdout 只按 LF 分帧（U+2028/U+2029 在 JSON 字符串里合法）
- 未知记录忽略，不报错（装了扩展的环境首帧就是 `extension_ui_request`）
- 命令按 id 关联，不按顺序
- `chat:done` 只在 `agent_settled`
- 进程死亡不自动重发本轮（工具可能已有副作用）

**代价**：启动多一跳（首次 spawn 含扩展加载约 6s，用启动时预热抵掉）；
协议是 PI 的私有契约，PI 改事件名时要跟。

### 4.2 backend/service —— 业务编排

**设计**：会话、对话、配置、拓展模块的业务编排都在这层。事务边界在这层，
不写 SQL（走 repo），不引 gin / Wails。对话这里是「向 PI 发命令并翻译它的事件」，
不是自己跑 Agent 循环。

**关键不变量**：
- 事件出口串行化（沿用 `Emitter`，按会话分道）
- 停止必先 `clear_queue` 再 `abort`
- 会话运行中不删会话文件
- 同一工作区串行（一个 PI 进程只有一个 streaming 状态）

**代价**：同一工作区的两个会话不能并发跑。

### 4.3 backend/domain —— 域模型

一个聚合根一个文件，DO / DTO / REQ / VO / RESP 同居一处。
会话表只存归属、配置快照与 PI 会话指针（`pi_session_id` / `pi_session_file`）：
消息真身在 PI 的 JSONL 里，WorkBaby 不另持一份条目链，避免两份真相各自漂移。

### 4.4 backend/runtime —— 内置运行时

Node 24.21 / Python 3.12.8，两份归档 `go:embed` 进 exe，
首次启动解压到数据目录。Node 的解压必须剥掉归档顶层单目录并平铺到根——
多一层目录会让 `npm.cmd` 找错 node。

**注入 PATH**：`<node>;<npm-global>;<python>;<原 PATH>`，前置拼接。
PI 进程与它派生的所有工具共享这份环境。PowerShell 不内置：Windows 自带 5.1，
助手要跑命令时直接用系统那份，再嵌一份 110MB 的 pwsh 换不来新能力。

**代价**：exe 因此约 90MB。

### 4.5 拓展模块

三个 WorkBaby 自己的模块，各自接到 PI 能读的地方：

| 模块 | 接法 |
|---|---|
| 技能 | 目录即 PI 的 `<agent-dir>/skills/`，同一份 SKILL.md 两边都读 |
| 知识库 | 自己暴露成 stdio MCP server（`--mcp-knowledge`），写进 `mcp.json` |
| 记忆 | 写进 `<agent-dir>/APPEND_SYSTEM.md` 的受控标记块 |
| 插件 | 就是 PI 自己的 `extensions/` + `settings.json`，界面只做清单与启停（[specs/22](../specs/22-extensions.md)） |

**写 `settings.json` 只动自己掌管的顶层键**：PI 与用户还在里面写 `packages` / `cacheWarming` /
`codemode` 等字段，整体重写会把它们连同已装的包一起抹掉。

## 5. 两处存储

| 数据 | 位置 | 谁写 |
|---|---|---|
| 消息、工具结果、压缩摘要、分支树 | PI 的 JSONL | 只有 PI |
| 会话标题 / 工作区 / 模型 / 权限 | SQLite `sessions` | 只有 WorkBaby |
| 用量、运行记录、检查点、评估 | SQLite | 只有 WorkBaby |
| 技能、知识库、记忆、MCP 配置 | SQLite + 文件 | 只有 WorkBaby |

**绝不写 PI 的 JSONL**：格式是 PI 的私有契约，写坏它 PI 自己都读不回来。

## 6. 端口与启动时序

```
OnStartup
  ├─ 解析路径、打开 SQLite、跑迁移
  ├─ 解压内置运行时（幂等，命版本标记即跳过）
  ├─ 环境闸门：node? pi? model? → 写进 BootstrapVO
  ├─ 装配服务容器
  ├─ 启动 gin 监听（127.0.0.1:随机端口）
  ├─ 端口就绪 → 允许 domReady 广播 app:ready
  └─ 环境就绪则异步预热一个 PI 进程
```

端口握手必须早于 `domReady` 广播：监听器晚一步端口就为空，前端所有接口 404。

## 7. 为什么不重写 PI

| 取 | 舍 |
|---|---|
| 工具、压缩、重试、缓存、编辑、MCP、扩展、子代理全由 PI 演进 | 依赖 Node 与 PI 的存在，启动前必须过环境闸门 |
| 会话是纯 JSONL，可只读投影，不复制状态 | 两处持久化，删会话要删两处 |
| 协议是 JSONL over stdio，语言无关 | PI 改事件名要跟 |
| 崩溃隔离：PI 挂了不带走桌面进程 | 要一套重启与对账逻辑 |

重写的收益只是省一个 Node 依赖，成本是永久追版本。

**PI 不强制用户预先安装**。启动时按「系统已装 > 内置那一份」的顺序找：
用户机器上已经 `npm i -g` 过 PI 时直接复用它（带他自己的包与版本，命令行里改的配置也认），
只有找不到才退到内置那份。因此老用户不会被强制迁移，新用户也不用自己装环境。
