# docs/ARCHITECTURE.md · 架构与数据组织

## 1. 分层

```
main.go / app.go              桌面壳：Wails 生命周期、单实例、托盘、嵌入前端资源
   │
   ├─ internal/server         HTTP 层：gin 路由（/api/v1）、统一响应、SSE Hub
   ├─ internal/api            业务 handler（薄）+ 系统能力绑定（唯一允许 import wails runtime 的包）
   │
   │  ── 以下为业务层，互不横向依赖，只向下依赖 ──
   ├─ internal/service        编排层：会话、审批、任务、文件、技能、MCP、知识库、记忆…
   ├─ internal/agent          Agent 内核（PI 形态）：ReAct 循环、护栏链、事件、压缩、检查点
   ├─ internal/tool           工具契约、注册表、内置工具
   ├─ internal/llm            Provider 抽象与三家协议实现
   ├─ internal/capability     能力注册表（上下文装配 / 工具暴露 / run 后沉淀）
   ├─ internal/memory         长期记忆（Markdown + FTS5）
   ├─ internal/rag            知识库分块与检索
   ├─ internal/skill          技能解析与注册表
   ├─ internal/resource       内置资源装载（AGENTS.md / 斜杠命令 / frontmatter）
   ├─ internal/mcp            MCP 子进程客户端
   ├─ internal/runtime        数据目录、内置运行时、工作区沙箱、归档
   ├─ internal/event          事件总线与 run 事件日志
   ├─ internal/tray           系统托盘（Windows 原生 + 非 Windows 空实现）
   ├─ internal/singleinstance 单实例保护（命名互斥 + 本地 TCP IPC）
   │
   ├─ internal/repo           数据访问（gorm），每个 DO 一个 repo
   ├─ internal/domain         数据对象与状态机常量（无逻辑）
   ├─ internal/bootstrap      App 组合根：repo 实例唯一装配点
   ├─ internal/config         Viper 配置
   ├─ internal/db             SQLite 连接、迁移、FTS5
   └─ internal/pkg            叶子工具库（错误、ID、日志、加密、路径、分词…）
```

**为什么这样分**：桌面应用的复杂度不在 HTTP，而在「Agent 编排 + 本地能力 + 前端交互」三者。
所以把 HTTP 与 Wails 绑定压到最外两层（都只做参数透传），业务规则全部落在 service 与能力域，
使它们能脱离 gin/wails 单测；`domain` 只放数据、`pkg` 只放无业务语义的工具，两者是稳定公共底座。

## 2. 依赖约束（`scripts/check-boundaries.ps1` 强制，违反即失败）

| 约束 | 含义 |
|---|---|
| `pkg-no-internal` | `internal/pkg` 不得依赖任何 internal 包（保持叶子） |
| `domain-no-upward` | `domain` 只定义数据与常量 |
| `repo-no-upward` | `repo` 不得依赖 service / api |
| `api-no-repo` | `api` 不得直接 import repo（必须经 service） |
| `service-no-http` | `service` 不得 import gin / wails |
| `server-no-downward` | 业务层不得反向依赖 server |
| `agent-no-upward` / `agent-no-http` | 内核不依赖上层，也不感知 HTTP |
| `contract-version-sync` | 后端 `domain.ContractVersion` 与前端 `UI_API_CONTRACT_VERSION` 必须一致 |

`scripts/check-contract.ps1` 另外校验前端 API 路径白名单与后端路由表一致（杜绝 404）。

## 3. 启动装配

`api.Handler.Startup(ctx)` 是唯一装配入口，顺序即依赖顺序：

| # | 步骤 |
|---|---|
| 1 | `runtime.Resolve()` 定位数据根（`WORKBABY_HOME` 或 `%APPDATA%\WorkBaby`） |
| 2 | 日志初始化、配置初始化并补齐 `model.json` / `mcp.json` 模板 |
| 3 | 打开 SQLite（WAL、`busy_timeout`、连接数固定 1），执行迁移与 FTS5 建表 |
| 4 | `bootstrap.New(db)` 一次性构造全部 repo |
| 5 | `registry.Build` 解密 API Key 后构造 Provider（单条失败只告警，不阻塞启动） |
| 6 | `SessionContext`：工作区根与过程数据目录的唯一解析器 |
| 7 | `Emitter`：事件统一出口（chat / task / approval / 变更 / 工件共用同一实例，seq 落同一序列） |
| 8 | 文件变更 / 工件 / 审批 / 信任服务 |
| 9 | 注册全部工具 → `SelfCheckSchema()`（schema 不合法即启动失败） |
| 10 | 记忆、技能（内置 → 全局 → 工作区）、子智能体、斜杠命令、MCP（`mcp.json` 为源同步进 DB 并拉起子进程） |
| 11 | 能力注册表（Preload / Tools / Capture 三通道） |
| 12 | 文件 / 文件夹 / 会话工作区服务 + 用户钩子服务 |
| 13 | `ChatService`：`ChatDeps` 一次性注入 → `MissingDeps()` 自检（缺核心依赖即启动失败） |
| 14 | `approvalSvc.WithResumeHook(chatSvc.ResumeRun)`：唯一一处构造后绑定（审批 ↔ chat 是真实循环依赖） |
| 15 | 跨重启恢复：`RearmPending / LoadGrants / ReapInterrupted` |
| 16 | `ChatTaskService`（复用 chat 的工具注册表、护栏链与事件出口） |
| 17 | `bindEventBridge()` 把 `app:*` 桥到 Wails |

**装配铁律**：编排服务的依赖在构造期一次性注入（`XxxDeps` 结构体），禁止后置 `With*` setter——
后置注入会让「漏接」在运行期表现为功能静默失效（无报错、无日志），因此配套 `MissingDeps()` 启动自检。

`app.go` 随后启动内嵌 HTTP Server（随机端口）并发 `app:ready{server_port}`，前端据此初始化基址。

## 4. 双通道

| 通道 | 承载 | 为什么 |
|---|---|---|
| HTTP `/api/v1` + SSE | 全部业务 API 与实时事件 | 浏览器可见、可 curl 调试、可重放；前端可独立于桌面壳开发 |
| Wails 绑定 | 系统能力：窗口、托盘、文件对话框、剪贴板 | 只有这些需要操作系统原生能力，IPC 不承担业务流量 |

`app:*` 事件经 `bindEventBridge` 转发到 Wails；`chat:*` 不转发——SSE 已覆盖。

## 5. 并发模型

| 场景 | 机制 |
|---|---|
| 会话内串行、跨会话并行 | `ChatService.locks` 会话级互斥锁，关键区只覆盖「占位落库 + run 登记」 |
| 一次 run | 独立 goroutine + `context.WithCancel`，句柄存 `runRegistry`，前端「停止」即取消 |
| 工具并行 | 工具自声明 `ExecutionMode`：`parallel` 且批内 >1 时信号量并发（`Loop.cfg.Parallel` 兜底），批内任一 `sequential` 则整批串行；结果一律按调用顺序回填 |
| 后台任务 | 队列容量 64 + 2 个 worker，取消走 per-task `CancelFunc` |
| 事件发布 | `event.Bus` 在调用方 goroutine 同步执行；SSE Hub 每客户端独立 256 缓冲，慢客户端断连触发重连 |
| SQLite | `SetMaxOpenConns(1)` + WAL，规避写锁竞争 |

## 6. 一次对话的完整数据流

```
前端 ChatInput
  │ POST /api/v1/chat/stream {session_id, content, file_ids}
  ▼
server → api.Handler → ChatService.SendStream
  │  ① 会话级锁 → 取会话 → 补齐 Provider/Model
  │  ② prepareRun：分配 runID、登记执行平面、落 user 消息 + assistant 占位（streaming）
  │  ③ 返回 {run_id, assistant_msg_id}，后台 goroutine 继续
  ▼
runLLM（goroutine）
  │  上下文装配：能力 Preload（环境/工作区/记忆/知识/技能）→ BuildSystem 预算裁剪
  │  历史清洗：RebuildHistory 剔除空占位与孤儿 tool 结果
  │  组装 agent.Loop：Provider + 工具暴露 + 护栏链 + Sink(事件映射) + 队列(插话/续跑)
  ▼
agent.Loop.Run
  │  outer：drain followUpQueue（自动续跑 / 目标模式续接统一走 follow-up）
  │  inner 每轮：PrepareNextTurn（压缩 / 切模型 / 注入）→ drain steeringQueue → 请求模型（流式）
  │            → 按 ExecutionMode 执行工具 → 回填 → 写检查点
  │  事件经 Sink → service 映射为 chat:* → Emitter（注入归属 + seq + 重放缓冲）→ 总线 → SSE
  ▼
收尾
  │  assistant 落库（正文/思考/工具调用/用量/耗时/停止原因）
  │  发 chat:done（落库后才发，避免前端空内容覆盖）
  │  能力 Capture（记忆沉淀等）→ 目标模式判定是否继续
  ▼
前端 SSE 收到 chat:done → 拉权威快照对账（连接保持打开，等待可能的续跑 run）
```

前端同时监听 `GET /api/v1/events?scope=chat&session_id=...&run_id=...`：**订阅维度是会话**，
run 只用于重放定位——目标模式的自动续跑会起新 run，按 run 过滤会让前端在 `chat:done` 之后错过续跑事件。

## 7. 数据组织

### 7.1 存储约定

- 主键一律字符串 ID：`pkg.NewID("前缀")` 生成 `前缀_ULID`（同毫秒自增熵，天然按时间有序）
- 时间字段统一 `int64` 毫秒；JSON 字段以 `...JSON` 结尾存文本
- 软删：仅 `knowledge_docs` 用 `gorm.DeletedAt`，其余物理删除
- 文件作配置源 / 快照 / 导出；配置真相源：`model.json` / `mcp.json`

### 7.2 表清单（24 张）

| 表 | 用途 |
|---|---|
| `ai_providers` | 模型服务配置（密钥加密存储） |
| `chat_sessions` | 会话（含工作区、权限模式、目标、元数据） |
| `chat_messages` | 消息（含用量、耗时、停止原因、工具调用） |
| `message_blocks` | 消息块（思考/正文/工具调用/产物，供刷新后复现） |
| `session_todos` | 会话待办 |
| `token_usages` | 每次上游调用的 token 明细（按来源拆分） |
| `system_settings` | 键值设置（工具启停、exec 白名单、外观…） |
| `skills` | 技能（内置 / 下载 / 自定义，含启用状态） |
| `agent_profiles` | 自定义子智能体 |
| `user_commands` | 自定义斜杠命令 |
| `user_hooks` | 用户钩子 |
| `mcp_servers` | MCP 服务器配置（env 加密） |
| `knowledge_docs` / `knowledge_chunks` | 知识库文档（软删）与分块 |
| `agent_checkpoints` | run 检查点（每 run 只留最新一轮） |
| `run_records` | 运行历史索引 |
| `approval_records` | 审批 / 补问记录（暂停恢复的落点） |
| `approval_grants` | 免审授权 |
| `chat_tasks` | 后台任务 |
| `folders` / `files` | 受管文件夹与文件 |
| `file_changes` | 文件变更（快照 + diff + 回滚） |
| `artifacts` | 会话产出物登记 |
| `workspace_trust` | 目录信任 |
| `memory_fts` / `knowledge_chunks_fts`（虚拟表） | 全文索引 |

**FTS5 必须用 `tokenize='trigram'`**：默认 `unicode61` 不按字切分 CJK，整句中文会退化成单个 token，检索全面失效。
代价是短于 3 字的查询零命中，由调用方子串兜底承担。

### 7.3 状态机

| 对象 | 状态流转 |
|---|---|
| 消息 | `pending → streaming → completed / failed / cancelled → archived` |
| 停止原因 | `completed / cancelled / max_turns / tool_error_limit / token_budget / stagnation / error` |
| 审批 | `pending → approved / denied / timeout / cancelled / answered / skipped → consumed` |
| 后台任务 | `pending → running → completed / failed / cancelled` |
| 知识文档 | `pending → parsing → indexed | failed` |
| 运行记录 | `running → done / error` |
| 执行平面 | `chat_turn / task / tool_only / delegate`；`planning / running / paused / waiting_input / completed / failed / cancelled` |
| 目标 | `active / paused / done` |
| 用量来源 | `chat / memory / delegate / task`（委派与后台任务单独归因） |

### 7.4 关键字段

| 实体 | 字段 |
|---|---|
| `ChatSessionDO` | `ProviderID / Model / WorkspacePath / PermissionMode / Kind(normal\|side) / ParentID / Pinned / MessageCount / MetadataJSON`；目标模式与压缩边界存 `MetadataJSON` |
| `MessageDO` | `Seq`（会话内单调，工具消息与注入消息统一分配）/ `RunID` / `Role` / `ToolCallID` / `ToolCalls` / `Status` / `StopReason` / token 字段 / `LatencyMs` / `Cost` |
| `TokenUsageDO` | `Source / Agent / Turn`（`Turn = -1` 表示非对话轮次） |
| `ApprovalRecordDO` | `Kind(approval\|input) / Command / Risk / ExpiresAt` |
| `ChatTaskDO` | `SessionID`（宿主会话：工作区与模型来源）/ `Agent` / `Prompt` / `State` / `RunID` / `Result` / `Error` |

## 8. 目录约定

- 路由：`internal/server/routes.go`（Phase 1 合并：11 → 1）
- 处理：`internal/api/` 三文件：`api_chat.go` / `api_provider.go` / `api_handlers.go`（其余 22 个薄壳合并）
- 业务：`internal/service/`
  - **chat_run.go**（Phase 3 合并 `chat.go` + `chat_agent.go` + `chat_prepare.go` + `chat_finalize.go` + `chat_runs.go` + `chat_stream.go`，6 → 1）
  - `chat_sessions.go`（导出 `SessionContext` / `SessionTodoStore`）
  - `chat_task.go`（独立类型 `ChatTaskService`）
  - `approval.go` / `context.go` / `mcp.go` / `provider.go` / `skill.go` / `tool.go` 等
- 工具：`internal/tool/functools/` 拆 4 文件
  - `base.go`（`FuncTool` 骨架 + `All()`）
  - `tools_data.go`（数学 / JSON / CSV / 哈希 / 编码 / 数据 / IP，18 个工具）
  - `tools_text.go`（日期时间 / 文本 / 正则，9 个工具）
  - `tools_io.go`（随机 / UUID，3 个工具）
- 数据：`internal/domain/<表>.go` 定义 DO 与状态常量；`internal/repo/<表>.go` 提供访问
- 前端：`frontend/src/src` 下 `api / stores / components / chat / composables / i18n / types`

### 8.1 文件规模参考（Phase 3 落地后）

| 包 | 文件数 | 总 LOC | 备注 |
|---|---|---|---|
| `internal/agent` | 17 | ~2200 | 含 `phase3_test.go`（4 新测试 / 5 测试辅助类型）|
| `internal/service` | 46 | ~13K | `chat_run.go` 2062 LOC 为单文件最大 |
| `internal/tool/functools` | 4 | ~1300 | 拆 4 主题文件后均 ≤ 600 LOC |
| `internal/llm` | 22 | ~2200 | providerbase 已抽取 |
| `internal/tool` | 38 | ~5800 | 子包按域清晰 |

## 9. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 业务规则集中在 service / 能力域 | 可脱离 HTTP 与桌面壳单测 | service 层文件较多（按域拆分） |
| 单一装配入口 + 构造期注入 | 依赖顺序显式，漏接在启动期暴露 | 装配函数较长；循环依赖需单独绑定点 |
| 双通道（HTTP + 原生绑定） | 业务可调试、可重放 | 前端需处理端口注入与两条通道边界 |
| 会话级锁 | 跨会话天然并行 | 同会话无法并行（后续消息只能插话/排队） |
| SQLite 单连接 | 无写锁竞争，行为可预测 | 写吞吐受限；重查询阻塞写 |
