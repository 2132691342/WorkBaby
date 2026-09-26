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
| Agent | 自研 agent（两层 for 循环 + 护栏中间件链） | 见 specs/features/agent |
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
│   ├── agent/                    # ⑥ Agent 内核（ReAct 循环 + 护栏链 + 压缩 + 检查点 + 事件；PI pi-agent-core 等价物）
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
└── specs/features/               # 功能规格（按业务域分组）
```

**禁止**：

- 在 `service / repo / api` 包内定义实体 / DTO / VO / 枚举（必须放 `domain/`）
- 错误码 / 工具类分散到各业务包（统一 `internal/pkg/`）
- 在 `main` 包内写业务代码
- `internal/pkg/` 内出现业务词汇（session / provider / workflow 等）
- `internal/pkg/` 依赖任何其他 `internal/` 业务包（叶子工具包铁律）
- `agent/` import wails / api / service / server / gin

注：原 `internal/core/` 已重命名为 `internal/agent/`（PI `pi-agent-core` 等价物）；后续若文档出现 `core/` 字样均指 `agent/`。

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

- 根级：`README.md` / `AGENTS.md` / `DESIGN.md` / `CHANGELOG.md` / `TODO.md`
- `docs/`：项目级说明（架构 / 规范 / 页面 / 流程 / 契约 / 部署）
- `specs/features/<域>/`：功能规格，编号连续，一个功能一个文件
- 每篇只写现状：定位 → 设计 → 契约（表 / 字段 / 端点 / 事件与代码一致）→ 关键流程 → 约束 → 取舍
- 表格与短段落优先；不写 TODO 与未来计划（进度统一进 `TODO.md`）

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

### 2.13 Agent 内核 PI 契约（强制）

`internal/agent` 严格遵循 PI `pi-agent-core` 的契约；只在 §9.7 列出的 7 项上偏离，全部偏离都写入规格并在 PR 中标注。

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

详见 `specs/features/agent/01-react-loop.md` / `02-guard-chain.md` / `03-context.md`。

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

## 9. PI 形态重构（2026-09）

按 PI（HuggingFace `pi-mono`，对齐基线 v0.85.0）的设计哲学对本工程做定向精简。删除冗余、合并 fan-out，不引入插件系统，不动 agent 层的 ReAct 语义。

### 9.1 删除清单

| 删除 | 理由 |
|---|---|
| `internal/pet/` 整包 + `internal/repo/pet.go` + `internal/domain/pet.go` + `internal/api/api_pet.go` + `internal/server/routes_pet.go` | 与「干活型个人 AI 助手」定位不符；用户已确认删除 |
| 前端 `components/pet/`（4 文件） + `stores/pet.ts` + `SettingsView` 的 pet tab + `App.vue` 的 pet:show/pet:hide 监听 + `router` `/pet/desktop` + `AssistantAvatar` 的 sprite 引用 | 同上 |
| 前端 `components/{channel,cron,home,workflows,folders}/` 5 个空 / 残留目录 | 死路径 |
| 11 个 settings 视图的孤立目录 `components/{agents,commands,docs,files,hooks,mcp,memory,runs,skills,tools,wiki}/` | 单一视图无理由独占子目录；统一迁到 `components/settings/views/` |
| `domain/contract.go` | 仅一个常量 `ContractVersion`，已并入 `domain/id.go` |
| `domain/approval.go` | `ApprovalPendingRESP` 并入 `domain/approval_grant.go` |
| `service/chat_gates.go` | 2 个内部辅助函数并入 `service/chat.go` |
| `service/chat_usage.go` | 4 个 usage 落库函数并入 `service/chat_finalize.go` |
| `service/{meta,docs}.go` | 壳函数，由调用方内联（`docs.go` 后重建为内置用户手册服务 `DocsService`） |
| `internal/llm/{openai,anthropic,ollama}/client.go` 中的 `mustMarshal` / `intPtr` | 重复实现 3 次；统一到 `internal/llm/providerbase.go` |

### 9.2 合并清单

| 操作 | 前 → 后 |
|---|---|
| `internal/server/routes_*.go`（11 文件） | → 1 个 `routes.go`（963 LOC，单一 register 入口） |
| `internal/api/api_*.go`（24 文件） | → 3 个：`api_chat.go` + `api_provider.go` + `api_handlers.go`（其余 22 个薄壳合并） |
| `internal/tool/functools/*.go`（12 文件） | → 4 个：`base.go`（FuncTool 骨架 + `All()`）+ `tools_data` / `tools_text` / `tools_io`（30 个工厂，按主题分） |
| `internal/service/chat_*.go`（12 文件） | → 10 文件（gates + usage 并入 chat / finalize） |
| 三个 provider 的 `mustMarshal` / `intPtr` | → `internal/llm/providerbase.go` 导出 `MustMarshal` / `IntPtr` |
| 前端 11 个 settings 视图子目录 | → `components/settings/views/` 一个目录 |
| 前端 `SettingsView` 中 11 个 `lazySection(() => import('@/components/{agents,commands,…}/X.vue'))` | → `@/components/settings/views/X.vue` 统一 |

### 9.3 维护原则

- **`agent/`（Agent 内核）ReAct 语义不变**：Phase 2 改名、Phase 3 契约对齐后，循环 / 护栏 / 压缩 / 检查点语义与 PI 形态一致
- **`internal/mcp/` 零修改**：4 文件包结构已 well-shaped（client / adapter / manager / platform 切分正确）
- **`internal/pkg/` 零修改**：叶子工具包铁律
- **前端 `main.ts` / `App.vue` 主体 / `themes.css` / `wb-ui.css` / `i18n` 零修改**：设计令牌与外壳骨架保留
- **不引入插件系统**：PI 的 Extension API 不移植；skill / command / hook 仍按内部资源加载
- **不引入新依赖**：所有变化都在 Go 标准库 + 已有第三方库内完成

### 9.4 架构门禁同步

- `scripts/check-boundaries.ps1`：`ContractVersion` 读取路径从 `internal/domain/contract.go` 改为 `internal/domain/id.go`
- `scripts/check-boundaries.ps1`：`core-no-upward` / `core-no-http` 改名为 `agent-no-upward` / `agent-no-http`；glob 由 `internal/core*` 改为 `internal/agent*`
- `scripts/check-contract.ps1`：`routes_*.go` glob 改为 `routes*.go`（合并后只有 routes.go）
- 前端 `api/client.ts` 的 `KNOWN_PREFIXES`：删除 `/api/v1/pet`（已无对应后端路由）；`/api/v1/folders` 后端路由与 ChatInput 引用链路仍存活，保留

### 9.5 Phase 2：核心包重命名（core → agent）

PI 的 `pi-agent-core` 在 Go 侧落到 `internal/agent/`（原 `internal/core/`）。重命名覆盖：

- `internal/core/*.go`（16 文件） → `internal/agent/*.go`，`package core` → `package agent`
- 34 个调用点的 import 路径与 `core.X` 引用全部更新
- `service/chat_task.go` 中变量名 `agent` 与包名冲突，重命名为 `agentName`（函数签名同步）

### 9.6 Phase 2 未做的项（评估后保留现状）

| 候选 | 评估 | 决定 |
|---|---|---|
| 新建 `internal/session/`（JSONL + SQLite 双轨） | service/chat_sessions.go 已封装 chat session 全部持久化逻辑；迁移涉及 ~30 个调用点，改动量大于收益 | 延后 |
| 新建 `internal/resource/`（合并 skill + agents.md + commands） | 已落地：`service/agents_md.go` + `command_file.go` + `frontmatter.go` 迁入 `internal/resource/`（agents_md / commands / loader）；`internal/skill/` 保留独立包 | 完成 |
| 新建 `internal/settings/`（分层 SettingsManager） | `service/settings.go`（140 LOC）+ `bootstrap` KV 表已承载分层语义；新建包价值边际 | 延后 |
| `internal/tool/builtin/`（聚合所有内置工具） | 各子包的 `Recorder` / `ScriptResolver` / `ExecPolicy` 等类型分散在子包内；新建聚合包形成反向耦合（子包需被 builtin 反向引用）。当前 `tool.Registry` 已支持 map 查找，handler 中 20 个注册点（18 直注 + 30 个 functools 批量 + 2 个能力域工具）是「显式优于隐式」的取舍 | 不做 |
| `bootstrap` 接管 `api/handler.go Startup` 460 LOC | Startup 是 Wails 生命周期钩子（ctx 注入 + runtime 调用），必须留在 handler；bootstrap 已接管 db/config/runtime 等横切关注点 | 不做 |

PI 形态重构已完成第 1 期（删除 + 合并）、第 2 期（`core → agent`）与 Phase 3（内核契约对齐）；`internal/resource/` 已落地。`session / settings` 两个新包属于「包装型重构」（零行为变化、纯结构调整），待需求驱动。

### 9.7 Phase 3：内核契约对齐（2026-09）

#### 9.7.1 7 项差距已对齐

| 差距 | WorkBaby 落地 | PI 等价 |
|---|---|---|
| 单层循环 → 2 层循环 | `runLoop` 拆 outer（wait follow-up）+ inner（tool+steering） | `agentLoop` / `agentLoopContinue` |
| `BeforeTurn` 三合一 → 三回调 | `Hooks.TransformContext` + `Hooks.ConvertToLlm` + `Loop.PrepareNextTurn` | `transformContext` / `convertToLlm` / `prepareNextTurnWithContext` |
| Steering/FollowUp 直返 → 队列 | `SteeringQueue` / `FollowUpQueue` + `QueueMode` | `PendingMessageQueue` |
| 编译期 API key → 可刷新 | `Loop.GetAPIKey func(ctx, provider) (string, error)` | `getApiKey` callback |
| 启发式并行判定 → 工具级标注 | `Tool.ExecutionMode() ExecutionMode`（`sequential` / `parallel`）| `executionMode` |
| 压缩耦合 Compressor → PrepareNextTurn 通用位 | `CompressInfo` 通过 `NextTurnUpdate.CompressInfo` 上行 | 同上 |
| `EventKind` 补全 | `EventQueueDrained`（入队可见性）| `message_start/update/end` 链式 |

#### 9.7.2 保留的偏离

| 项 | 决定 | 理由 |
|---|---|---|
| Middleware 链（6 层护栏）| **保留** | Expose / Schema / Policy / Approval / Adaptive / Repeat 6 维度互相隔离；PI 的单一 before/after 回调会让审批门、路径信任、用户钩子互相污染（见 `specs/features/agent/02-guard-chain.md`）|
| `Hooks.Steering` / `Hooks.FollowUp` 旧字段 | **保留** | 2 处 `Hooks{}` 字面量调用零迁移成本；新代码走 `WithSteeringQueue` / `WithFollowUpQueue` |
| `Loop.Run(ctx, history)` / `Loop.Resume(ctx)` 签名 | **不变** | chat_run.go / delegate_core.go 调用点零变化 |
| `service.ChatService` 公开方法集 32 个 | **不变** | API 契约稳定，前端零变化 |
| SSE 事件名 / 载荷 / 数据库 schema | **不变** | 重构严格控制在内核内部 |

#### 9.7.3 service 简化

`chat_*.go` 6 文件（528 + 254 + 155 + 317 + 499 + 309 = 2062 LOC）合并为单个 `chat_run.go`：
- `chat.go` + `chat_agent.go` + `chat_prepare.go` + `chat_finalize.go` + `chat_runs.go` + `chat_stream.go` → `chat_run.go`
- `chat_sessions.go` / `chat_task.go` / `chat_test.go` 独立保留
- 公开 API 零变化（依赖图已预先核实：无符号冲突、无循环依赖）
- `tool/functools/tools.go` 1228 LOC 拆为 `tools_data.go` / `tools_text.go` / `tools_io.go` 3 主题文件

#### 9.7.4 验证

- `go build ./...` · `go vet ./...` · `go test ./internal/... -count=1 -timeout 60s`
- `scripts/check-boundaries.ps1` · `scripts/check-contract.ps1`
- `cd frontend && npm run typecheck && npm test`

**重构基线**：调研结论已合并进本节与 `docs/ARCHITECTURE.md`；原始调研报告归档于 `docs/archive/ANALYSIS-PI-ALIGNMENT.md`（不再维护）。
