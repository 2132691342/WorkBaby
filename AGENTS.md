# AGENTS.md · WorkBaby 工程规范

> 本文件是代码风格 / 分包 / 依赖方向 / 工作流的唯一权威。
> 设计说明见 `docs/` 与 `specs/`；冲突时以本文件为准。

---

## 1. 技术栈（强制锁定）

| 层 | 选型 | 版本 |
|---|---|---|
| 语言 | Go | 1.24.x（单 module `WorkBaby`） |
| 桌面壳 | Wails | v2.12.x |
| HTTP 服务 | gin | ^1.10.x（业务 API + SSE 全部走 HTTP） |
| 前端 | Vue 3 + TypeScript + Vite | 3.5 / 5.6 / 6 |
| UI 组件 | Element Plus | ^2.8.x（全量引入 + zh-cn） |
| 状态管理 | Pinia | ^2.3.x（Setup Store） |
| 样式引擎 | Tailwind CSS | ^4.x（设计令牌经 `themes.css` 语义变量落地） |
| 图表 / 预览 | echarts · @vue-flow/core · lucide-vue-next · docx-preview / vue-pdf-embed / xlsx | 见 `frontend/package.json` |
| ORM | GORM + glebarez/sqlite | ^1.30.x（pure-Go，无 CGO） |
| 数据库 | SQLite | 3.45+（WAL + FTS5 trigram） |
| 配置 | Viper | ^1.19.x（YAML + ENV；运行时配置走 KV 表） |
| 模型配置 | model.json | 文件为源 + DB 同步 |
| MCP 配置 | mcp.json | 文件为源 + DB 同步 |
| 网络搜索 | DuckDuckGo | 免 Key，HTML 解析 |
| 日志 | log/slog（标准库） | 落文件 + 轮转 |
| 实时通信 | SSE | 业务实时通信全部走 SSE（`/api/v1/events`），未引入 WebSocket |
| 测试 | testing + testify | ^1.9.x |
| ID | oklog/ulid/v2 + google/uuid | 业务 ULID 带前缀；trace 用 UUID |
| Agent | 自研 agent（两层 for 循环 + 护栏中间件链） | 见 specs/01-04 (agent) |
| LLM 适配 | 自研协议适配 | OpenAI / Anthropic / Ollama |
| Schema 校验 | santhosh-tekuri/jsonschema/v6 | ^6.0.x |
| MCP | 自研 stdio 客户端 | spec 2025-06-18 |
| PDF | ledongthuc/pdf | 知识库加载 |
| 文本编码 | golang.org/x/text | exec 输出 GBK/UTF-16 归一化 |

### 1.1 依赖原则

标准库 → `golang.org/x/...` → 生态库。新增依赖必须说明理由与替代方案。

评估结论：`gotool`（cnlesscode）**不引入**——其 gfs / gZip / request / random 能力与 `internal/pkg` 及标准库重叠，且缺本项目必需的编码归一化（GBK/UTF-16）、AES-GCM、日志轮转与 HTTP 流式；引入只增依赖面不补缺口。

---

## 2. 强制编码规范

### 2.1 包结构

```
WorkBaby/
├── main.go                       # 入口：embed + wails.Run 装配
├── app.go                        # App 结构体（嵌入 *api.Handler，生命周期委托）
│
├── internal/
│   ├── server/                   # ① HTTP 层（gin）：路由 / 中间件 / SSE hub / 统一响应
│   ├── api/                      # ② 业务路由 handler（薄）+ 系统能力绑定（唯一允许 import wails 的包）
│   ├── service/                  # ③ 业务编排层（事务边界；不写 SQL；不引 gin/Wails）
│   ├── repo/                     # ④ 持久层（GORM；不引上层）
│   ├── domain/                   # ⑤ 域模型（一个聚合根一个文件，DO/DTO/REQ/VO/RESP 同居一处）
│   ├── agent/                    # ⑥ Agent 内核（ReAct 循环 + 护栏链 + 压缩 + 检查点 + 事件）
│   ├── llm/                      # ⑦ LLM 适配（providerbase + openai/anthropic/ollama + registry + toolcall）
│   ├── tool/                     # ⑧ 工具系统（registry + functools 30 个工具 + exec/file/...）
│   ├── skill/                    # ⑨ Skill（parser/registry/builtin）
│   ├── mcp/                      # ⑩ MCP stdio 客户端（client/adapter/manager）
│   ├── resource/                 # ⑪ 内置资源装载（AGENTS.md / 斜杠命令 / frontmatter 解析）
│   ├── capability/               # 能力接入：Preload / Tools / Capture 三通道
│   ├── memory/                   # ⑫ 长期记忆（单一 MEMORY.md + FTS5 派生索引）
│   ├── rag/                      # ⑬ 知识库（loader/chunker/indexer/retriever）
│   ├── runtime/                  # ⑭ 运行时设施（paths/runtimes/archive/sandbox；当前仅内置 python）
│   ├── config/                   # ⑮ 配置（Viper）
│   ├── event/                    # ⑯ 应用内事件总线
│   ├── db/                       # ⑰ SQLite 打开 + 迁移
│   ├── bootstrap/                # ⑱ App 组合根：repo 实例唯一装配点
│   ├── pkg/                      # ⑲ 自研底层工具（叶子：AppError/ID/日志/路径/加密/HTTP 规则）
│   ├── tray/                     # ⑳ 系统托盘（Windows Win32 + 非 Windows 空实现）
│   └── singleinstance/           # ㉑ 单实例保护（命名互斥 + 本地 TCP IPC）
│
├── frontend/                     # Vue 3 工程
├── assets/                       # 内置 Skill / 图标 / 用户手册（embed）
├── build/                        # 平台资源与产物
├── docs/                         # 项目级文档（架构/规范/流程）
└── specs/               # 功能规格（拍平：01-21 连续编号）
```

**禁止**：

- 在 `service / repo / api` 包内定义实体 / DTO / VO / 枚举（必须放 `domain/`）
- 错误码 / 工具类分散到各业务包（统一 `internal/pkg/`）
- 在 `main` 包内写业务代码
- `internal/pkg/` 内出现业务词汇（session / provider / workflow 等）
- `internal/pkg/` 依赖任何其他 `internal/` 业务包（叶子工具包铁律）
- `agent/` import wails / api / service / server / gin

注：`internal/agent` 是当前唯一的 Agent 内核包；历史文档中的 `core/` 字样均指 `agent/`。

### 2.2 依赖方向（强制）

```
internal/server ──► internal/api ──► internal/service ──► 能力域（agent/llm/tool/memory/rag/...）
                                          │
                                          ▼
                                     internal/repo ──► internal/domain
                                          │
                                          ▼
                              internal/pkg（叶子工具包：各层可依赖，自身不依赖任何 internal 业务包）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| server | api / domain / pkg / event / gin | service / repo / 能力域 |
| api | service / domain / pkg / event / gin | repo / 能力域 |
| service | repo / domain / pkg / 能力域（经 interface） | api / server / gin / wails |
| 能力域 | repo / domain / pkg / 其他能力域（经 interface） | api / service / server / wails |
| repo | domain / pkg / gorm | api / service / 能力域 |
| domain | pkg | 任何上层 |
| **internal/pkg** | 标准库 / golang.org/x / 第三方工具 | **任何其他 internal 业务包** |
| agent | llm / tool / memory / pkg | api / service / server |

**双主机**：业务 API 走 gin HTTP，Wails 绑定只保留系统能力（对话框/剪贴板/托盘/窗口）。前端事件走 SSE。端口注入：`app:ready` 携带 `serverPort`。

### 2.3 聚合根文件组织（domain 单文件多形态）

一个聚合根一个文件，该实体所有形态同居一处：

| 后缀 | 语义 | 标配 |
|---|---|---|
| DO | Domain Object，映射数据库表 | 必有 |
| DTO | 能力域间传输载体 | 按需 |
| REQ | 前端 → 后端入参 | 按需 |
| VO | 组装后的视图对象 | 按需 |
| RESP | 后端 → 前端出参 | 按需 |

包级错误变量集中在文件顶部声明（`var ErrXxx = pkg.New(...)`）。

### 2.3.1 内联结构体边界（强制）

`server / api / service / repo` 四层禁止就地声明入参/出参/实体结构体，必须引用 `domain.XxxREQ/DTO/VO/RESP/DO`。

例外：`internal/tool/*` 内的快速参数解析允许内联（须加注释 `// 内联快速解析；不暴露为领域类型`）；`internal/repo/*` 内 GORM 临时查询条件允许内联。

### 2.4 错误处理（强制 AppError）

```go
func New(code int, message, details string) *AppError
func Wrap(code int, message string, err error) *AppError
```

```go
// ✅ 业务错误
if path == "" { return nil, pkg.New(1001, "路径不能为空", "") }
// ✅ Wrap 底层错误
if err := os.WriteFile(p, data, 0o644); err != nil { return pkg.Wrap(1002, "写文件失败", err) }
// ✅ 包级错误变量
var ErrSessionNotFound = pkg.New(1101, "会话不存在", "")
// ❌ 禁止
return errors.New("provider not ready")   // 丢 code，前端无法分流
```

**错误码段位**：

| 段位 | 域 |
|---|---|
| 1000–1999 | 通用 / 文件 / 路径 |
| 2000–2999 | 配置 / 持久化 |
| 3000–3999 | LLM / Provider |
| 4000–4999 | 工具 / 命令审批 |
| 5000–5999 | Agent / Harness |
| 6000–6999 | Memory |
| 7000–7999 | Knowledge / RAG |
| 8000–8999 | Skill / MCP |
| 9000–9999 | 保留 |

### 2.5 命名规范

| 类别 | 风格 | 示例 |
|---|---|---|
| 文件名 | snake_case.go | ai_provider.go |
| 包名 | 全小写无下划线 | agent / tool / memory |
| 类型 / 接口 | PascalCase（不加 I 前缀） | AiProviderDO / Provider |
| 函数 | 导出 PascalCase / 私有 camelCase | ListProviders / resolveOne |
| 变量 | camelCase | runID / compressRatio |
| 错误变量 | ErrXxx | ErrSessionNotFound |
| JSON tag | **snake_case** | json:"user_message_id" |
| GORM tag | snake_case | gorm:"primaryKey;size:64" |
| ID 前缀 | SCREAMING_SNAKE_CASE | PROVIDER / SESSION |

**禁止**：接口加 I 前缀；`ToolInfo` 类冗余后缀；匈牙利命名；`xxxId` 与 `xxxID` 混排。

**JSON 契约铁律**：所有 HTTP 交互字段一律 snake_case。前端 `types/api.ts`、store、组件消费的字段名必须与后端 RESP 的 json tag 完全一致。

### 2.5.1 LLM 协议 DTO 例外

`internal/llm/openai|anthropic|ollama/client.go` 是上游 API 响应反序列化 DTO，json tag 必须忠实上游字段名。归一化层（`llm/message.go` 的 Message / TokenUsage）使用 snake_case，与业务契约一致。

### 2.6 注释规范（强制精简）

- 包：1 行职责；类型：2~4 行概要；方法：签名级；字段：仅在不显然时 1 行
- **连续注释块 ≤ 3 行**：超过 3 行说明它该拆成字段注释，或该写进 `docs/` / `specs/`
- 文件头注释不超过 5 行
- 行内只解释「为什么」
- 禁止：过程性内容、长篇 HTML 注释、TODO 历史、实现细节
- 注释只描述**最终设计与实现**：禁改动过程叙事（「修复」「原…」「新增」）、里程碑标记
- 禁止出现外部项目名与「参考/借鉴」字样

### 2.6.1 文档规范

- 根级：`README.md` / `AGENTS.md` / `DESIGN.md` / `CHANGELOG.md`
- `docs/`：项目级说明（架构 / 规范 / 页面 / 流程 / 契约 / 部署）
- `specs/`：功能规格，编号 01-21 连续，一个功能一个文件
- 每篇只写现状：定位 → 设计 → 契约（表 / 字段 / 端点 / 事件与代码一致）→ 关键流程 → 约束 → 取舍
- 表格与短段落优先；不写 TODO 与未来计划（变更统一进 `CHANGELOG.md`）

### 2.7 全局 ID：ULID 大写 + 领域前缀

```go
func NewID(prefix string) string   // "SESSION_01ARZ3NDEKTSV4RRFFQ69G5FAV"
func NewTraceID() string           // UUID v4
```

前缀常量集中在 `internal/domain/id.go`。本机用户 id 固定 `"local"`。

### 2.8 时间戳统一毫秒整数

```go
CreatedAt int64 `gorm:"autoCreateTime:milli" json:"created_at"`
DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
```

禁止 `time.Time` / `time.Duration` 出现在跨边界 struct。

### 2.9 平台特定代码（build tag）

```go
//go:build windows
package platform
```

禁止一个文件里 `runtime.GOOS` 分支处理所有平台。

### 2.10 横切关注点

| 关注点 | 方案 |
|---|---|
| 统一错误 | AppError + 错误码分段 |
| 日志 | slog 仅 info/warn/error；warn、error 单独落文件 + 轮转；ctx 注入 sessionID/runID |
| 实时通信 | SSE only（256 缓冲 / 慢客户端断连 / `chat:gap` 重放窗口溢出） |
| 上下文压缩 | MicroCompressor（确定性折叠，不调 LLM） |
| 并发控制 | 有界 goroutine 池 |
| 事件出口 | `service.Emitter` 唯一出口（注入 run/session 归属 + 分配 seq + 写重放缓冲） |
| 事件总线 | event.Bus + server/sse 桥接；SSE 订阅维度是**会话**（`session_id`） |
| Token 计量 | 每次 LLM 调用一行 token_usages（agent TurnUsage → service.persistUsage） |
| 配置 | Viper + system_settings KV |
| 加密 | AES-256-GCM（Provider API Key、MCP env） |

禁止引入：OpenTelemetry / Prometheus 客户端；AOP 框架；全局可变状态。

### 2.10.1 前端设计令牌（强制）

视觉语言见 `DESIGN.md`。核心约束：

- 色值只在 `themes.css` 定义（`--wb-*` 语义令牌 + `[data-theme]` 块）；`useTheme.ts` 的 THEMES 表取值必须与 CSS 同步
- 组件禁止写死颜色：用 `wb-*` token 类（`text-wb-ink` / `bg-wb-surface-2` / `border-wb-border`），不用 `gray-*` 等默认调色板
- 分层只靠「1px 中性描边 + 极轻阴影」：禁渐变、禁彩色描边、禁 backdrop-blur 叠层、禁 hover 位移
- 控件高度四档：`--wb-ctl-h-sm`(26px) / `--wb-ctl-h`(34px) / `--wb-ctl-h-lg`(38px) / `--wb-ctl-h-xl`(44px) + `--wb-ctl-icon`(32px)；禁止裸像素高度
- 按钮一律胶囊镂空（描边 + 主色/中性字），实底只出现在 hover；组件语法唯一实现在 `wb-ui.css`
- 字号用刻度类 `text-3xs` / `text-2xs` / `text-xs2` / `text-ctl`；禁止 `text-[Npx]` 任意值
- 字体三族：UI 系统无衬线 / 标题读数 Bahnschrift（`--font-display`）/ 代码 JetBrains Mono
- 空态与骨架统一用 `EmptyState` / `Skeleton`（列表页经 `PageState` 三态包装）；禁止 `el-empty` / `el-skeleton`
- 用户背景图提取的主色经 `applyExtractedPrimary()` 统一应用/移除

### 2.11 数据库迁移

- v1：GORM AutoMigrate；新增字段只增不删、零值兜底
- v2：golang-migrate；PR 必带 up/down sql

FTS5 虚拟表与触发器用 raw SQL 启动期单独创建。

### 2.12 HTTP 与 API 路径约定（强制）

| 维度 | 规则 |
|---|---|
| HTTP 方法白名单 | 业务 / 工具仅允许 GET 与 POST |
| 路径参数位置 | 变量参数放路径末尾（`/xxx/:id/delete` 风格） |
| 删除语义 | 删除走 `POST .../delete` |
| 流式事件 | `GET /api/v1/events?scope=chat&session_id={sid}&run_id={runId}` SSE |

**路径风格唯一标准**：`/api/v1/{resource}/:id/{action}`，示例：

- 中断会话正在跑的 run：`POST /api/v1/chat/sessions/:id/cancel`
- 从检查点续跑：`POST /api/v1/chat/runs/:id/resume`
- 删除：`POST /api/v1/chat/sessions/:id/delete`
- 切换模型：`POST /api/v1/chat/sessions/:id/model`

前端 API 层收敛在 `frontend/src/src/api/`，路径白名单与后端 router.go 一一对应。

白名单逻辑收口在 `internal/pkg/httprules.go`（`AllowMethod` / `NormalizeMethod`）。

**例外**：llm Provider 适配层调上游 API 可用全部 method；mcp 的 JSON-RPC Method 字段是 RPC 方法名。

### 2.13 Agent 内核契约（强制）

`internal/agent` 是整个工程最核心的子系统，下面是它对外的强契约。

#### 2.13.1 主循环形态（2 层）

```
runLoop(ctx, msgs, startTurn, out, emitStart):
    outer:
        pending = drain(followUpQueue)            // 外层：等收尾缝
        inner:
            if lastTurn != nil && PrepareNextTurn != nil:
                update = PrepareNextTurn(lastTurnCtx)
                apply Model/Thinking/ExtraMessages/CompressInfo
            pending = drain(steeringQueue)         // 内层：轮间插话
            res = streamTurn(ctx, turn, msgs)
            append assistant
            toolMsgs, runTerminate = executeTools(ctx, turn, calls)
                // 按工具 ExecutionMode 分组：sequential 串行 / parallel 并发
            append toolMsgs
            emit turn_end
            if ShouldStop(ctx, turn): return
            if runTerminate && pending empty: break inner
        // 内层退出：本轮已收尾
        if followUp empty: return
    // 外层循环：自动续跑、目标模式续接统一走 follow-up
```

#### 2.13.2 三回调分离

| 回调 | 时机 | 职责 |
|---|---|---|
| `Hooks.TransformContext` | 每轮请求前 | 上下文裁剪（删除空占位 / 孤儿 tool / 折叠历史） |
| `Hooks.ConvertToLlm` | 协议边界 | `AgentMessage → LlmMessage` 归一（仅在调上游前） |
| `Loop.PrepareNextTurn` | 上一轮结束、下轮开始前 | 压缩 + 切换模型 / 思考档 + 注入消息；返回 `NextTurnUpdate` |

`BeforeTurn` 旧字段保留为兼容入口；新代码只写 `TransformContext`。

#### 2.13.3 队列与 QueueMode

```go
type QueueMode string
const (
    QueueOneAtATime QueueMode = "one-at-a-time"  // 默认：每次 drain 1 条
    QueueAll        QueueMode = "all"             // 批量入队（多用户同步场景）
)

type SteeringQueue interface {
    Drain() []*llm.Message  // 内层每轮开调一次
    Enqueue(*llm.Message)
    HasItems() bool
}
```

#### 2.13.4 工具 ExecutionMode

```go
type ExecutionMode string
const (
    ExecutionSequential ExecutionMode = "sequential"
    ExecutionParallel   ExecutionMode = "parallel"
)

// Tool 接口新增方法；未实现默认 Parallel
type Tool interface {
    ExecutionMode() ExecutionMode
}
```

工具声明 `Sequential` 时整批按调用顺序串行；声明 `Parallel` 且 >1 时走信号量并发，结果按调用顺序回填。
`Loop.cfg.Parallel` 作为未声明 `ExecutionMode` 的工具默认并行度。

#### 2.13.5 不变量

| 不变量 | 锁死原因 |
|---|---|
| 结果严格按调用顺序回填 | `assistant(tool_calls)` 与 `tool` 配对完整，顺序错位上游 400 |
| 「报错但零产出」轮不算成功 | 否则空 assistant 落库，下一轮直接 400 |
| 延迟覆盖消费全程 | 在建流返回就取值会漏掉整段流耗时，仪表盘归因失真 |
| 最后一轮永不丢 | 极端压缩场景下当前任务上下文必须完整 |
| 工具执行不依赖 `Loop.cfg.Parallel` | 工具自己声明 `ExecutionMode`；`Parallel` 仅作兜底 |

详见 `specs/01-react-loop.md` / `02-guard-chain.md` / `03-context.md`。

---

## 3. 测试规范

**原则**：测试失败时必须意味着某条真实链路坏了。

### 3.1 保留判据

写之前先问：**这个测试失败时，是否意味着某个跨模块 / 跨轮次 / 跨协议的行为坏了？**

| 保留 | 删除 |
|---|---|
| ReAct 多轮循环 / 检查点续跑 / 跨重启审批闭环 | 单个纯函数的输入输出 |
| 压缩不拆散 assistant+tool 对等**协议硬约束** | 简单 CRUD、字段映射、枚举转换 |
| 审批 / 信任 / 注入防护 / 路径穿越等**安全护栏** | 幂等 setter、构造函数冒烟 |
| SSE 断线重放 / 文件变更快照回滚等**集成编排** | 同一行为在不同文件里的重复断言 |
| FTS/ 检索打分、压缩裁剪等**复杂算法** | 只验证「不 panic」「非 nil」的弱断言 |

### 3.2 规模约束

- 单个测试文件 ≤ 6 个 `Test` 函数；同类行为用 table-driven 合并（子测试 `t.Run` 区分场景）
- 全量 `go test ./internal/...` 本地应在 10 秒内完成
- 环境依赖（系统 shell、真实网络）用 `exec.LookPath` / `testing.Short()` 守卫后跳过
- 每个测试文件**首行必须有导航注释**（说明本文件覆盖什么），便于定位

### 3.3 测试索引

| 文件 | 覆盖 |
|---|---|
| `agent/loop_test.go` | 多轮 ReAct、只读并行、终局工具、检查点续跑、异常收尾 |
| `agent/guard_test.go` | 护栏链拒绝语义、权限矩阵、熔断、失败改道注入 |
| `agent/context_test.go` | 上下文预算裁剪、历史清洗的协议硬约束 |
| `agent/phase3_test.go` | 2 层循环、PrepareNextTurn、Steering/FollowUp 队列、ExecutionMode |
| `agent/helpers_test.go` | 内核测试共享替身（非测试） |
| `service/agent_test.go` | 内核装配接线、失败改道、错误摘要、子智能体档案 |
| `service/recovery_test.go` | 检查点语义、跨重启审批/补问闭环、授权回滚、悬挂 tool_calls 剥离 |
| `service/chat_test.go` | 会话操作、审批放行档位、插话队列、后台任务 |
| `service/event_mapper_test.go` | 内核事件 → 前端协议映射、父子 run 分流、事件出口契约 |
| `service/security_test.go` | 目录信任生命周期、工作区路径穿越与内部目录隐藏 |
| `service/hook_test.go` | 用户钩子匹配式与子进程协议 |
| `service/integration_test.go` | MCP 配置加密回滚、文件变更快照回滚、反幻觉核验 |
| `llm/conformance_test.go` | 跨 Provider 错误分类与重试退避 |
| `llm/{openai,anthropic,ollama}/client_test.go` | 三家协议流式归一化一致性 |
| `server/server_test.go` | SSE 送达、断线重放、窗口溢出、慢客户端 |
| `mcp/client_test.go` | stdio 往返、服务不可用降级、管理器装卸 |
| `memory/search_test.go` · `rag/rag_test.go` | FTS 检索与短查询兜底、索引重建 |
| `tool/exec/exec_test.go` | 内置运行时查找优先级、PATH 隔离、白名单 |
| `tool/file/guard_test.go` | gitignore 尊重、写前必读 |
| `tool/planmode/planmode_test.go` | 计划模式安全底线 |
| `tool/workspace_link_test.go` | 工作区绑定后的工具落点一致性 |
| `runtime/runtimes_test.go` | 解压路径穿越与限额 |
| `frontend/src/src/chat/__tests__/` | 消息块归一化、SSE 事件映射管线 |

---

## 4. 架构门禁

```bash
powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1   # 依赖方向 9 项
powershell -ExecutionPolicy Bypass -File scripts/check-contract.ps1     # 契约：cancel/resume 拆分 + 前后端路径段一致
```

---

## 5. 工作流

- 最小改动：外科手术原则；改动前后跑测试
- 验证优先：修 Bug 先写复现测试
- YAGNI：禁止过早抽象、单实现接口
- 不要猜测：模糊需求先明确假设与边界

---

## 6. 验收命令

```bash
go build ./...                                     # 全量编译
go test ./internal/...                             # 后端测试
go vet ./internal/...                              # 静态检查
cd frontend && npm run typecheck && npm test       # 前端类型检查 + 测试
powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1
wails dev                                          # 开发模式
wails build -nsis -ldflags "-s -w" -trimpath       # 生产构建（NSIS 安装包）
```

---

## 7. 关键架构决策

- **依赖一次性注入**：编排服务用 `XxxDeps` 结构体在构造期注入全部依赖，禁止后置 `With*` setter。`MissingDeps()` 启动自检，装配不完整即启动失败
- **会话路径解析唯一数据源**：`service.SessionContext` 提供工作区根与过程数据目录
- **事件载荷契约**：跨端事件用 `domain` 侧结构体（`ChatDoneEvent` / `ChatToolCallEvent` / `ChatToolResultEvent` / `ChatApprovalEvent` / `ChatStatsEvent` / `ChatCompressedEvent` / `ChatWarnEvent`）保证形状唯一；流式高频增量仍用 map 直传（零转换）
- **单一聚合根文件**：`domain/{name}.go` 含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量
- **internal/pkg 为叶子工具包**：各层可依赖，自身不依赖任何其他 internal 业务包
- **双主机通信**：业务 API 走 gin HTTP（POST/GET + `{code,message,data}`），流式走 SSE
- **端口注入**：gin 监听 `127.0.0.1:0`，`app:ready` 携带 serverPort
- **存储语义**：DB 存元数据 + 内容；文件作配置源/快照/导出
- **API Key 与 MCP env 用 AES-256-GCM 加密**，主密钥首启随机生成并写入 config.yaml
- **thinking 与 content 严格分离**
- **本地单用户**，gin 只绑定 127.0.0.1 回环 + 随机端口

---

## 8. 反模式

❌ 大杂烩式改动 / 错误提前抽象 / 隐形架构决策 / 只覆盖乐观路径 / 臆造 API / 代码风格漂移 / 失控式连锁重构 / internal/pkg 依赖其他 internal 业务包

---

## 9. 文档结构与定位

| 位置 | 内容 |
|---|---|
| `README.md` | 项目定位、技术栈总览、模块树、启动与构建命令 |
| `AGENTS.md` | 工程规范（技术栈 / 包结构 / 依赖方向 / 命名 / 错误码 / 横切关注点 / 测试 / 门禁 / 验收） |
| `DESIGN.md` | 视觉语言（设计令牌 / 控件 / 字体 / 阴影 / 动效） |
| `CHANGELOG.md` | 面向用户的版本变更摘要（按 release 聚合） |
| `docs/ARCHITECTURE.md` | 模块拓扑、依赖方向、数据流、关键设计决策 |
| `docs/API-CONTRACT.md` | HTTP 路由表、SSE 事件清单、请求 / 响应 DTO、错误码映射 |
| `docs/PAGE-STRUCTURE.md` | 前端页面信息架构、导航、组件约束 |
| `docs/COMPONENT-GUIDELINES.md` | 前端组件编写规范（设计令牌、状态、ShellBridge 通信） |
| `docs/DEVELOPMENT.md` | 开发环境搭建、本地运行、调试技巧 |
| `docs/DEPLOYMENT.md` | 构建产物、安装包、升级与回滚 |
| `docs/PROJECT-SPEC.md` | 产品定位、用户旅程、功能清单 |
| `docs/prd/` | 需求原型 |
| `specs/*.md` | 功能规格（定位 → 设计 → 契约 → 关键流程 → 约束） |

文档只描述**当前状态与设计**：现状是什么、为什么这样设计、关键边界是什么。
不写过程性叙事（删除/合并/迁移史）、不写未来规划、不写历史里程碑。
