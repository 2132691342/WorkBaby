# CLAUDE.md · WorkBaby（Go 版）工程规范

> 本文件是工程代码风格 / 分包 / 依赖 / 工作流的唯一权威。
> 与 `doc/` 配套；冲突时以本文件为准。设计与实现见 `doc/` 各模块文档。

---

## 1. 技术栈（强制锁定）

| 层 | 选型 | 版本 |
|---|---|---|
| 语言 | Go | 1.24.x（单 module `WorkBaby`） |
| 桌面壳 | Wails | v2.12.x |
| HTTP 服务 | gin | ^1.10.x（业务 API + SSE 全部走 HTTP） |
| 前端 | Vue 3 + TypeScript + Vite | 3.5 / 5.6 / 6 |
| UI 组件 | Element Plus | ^2.8.x（全量引入 + zh-cn） |
| 状态管理 | Pinia | ^2.2.x（Setup Store） |
| ORM | GORM + glebarez/sqlite | ^1.30.x（pure-Go，无 CGO） |
| 数据库 | SQLite | 3.45+（WAL + FTS5 trigram） |
| 配置 | Viper | ^1.19.x（YAML + ENV；运行时配置走 KV 表） |
| 模型配置 | model.json | 文件为源 + DB 同步 |
| MCP 配置 | mcp.json | 文件为源 + DB 同步 |
| 网络搜索 | DuckDuckGo | 免 Key，HTML 解析 |
| 日志 | log/slog（标准库） | 落文件 + 轮转 |
| 测试 | testing + testify | ^1.9.x |
| ID | oklog/ulid/v2 + google/uuid | 业务 ULID 带前缀；trace 用 UUID |
| Agent | 自研 core（一个 for + 5 钩子 + 2 队列） | 见 doc/04 |
| LLM 适配 | 自研协议适配 | OpenAI / Anthropic / Ollama |
| Schema 校验 | santhosh-tekuri/jsonschema/v6 | ^6.0.x |
| MCP | 自研 stdio 客户端 | spec 2025-06-18 |
| PDF | ledongthuc/pdf | 知识库加载 |

### 1.1 依赖原则

标准库 → `golang.org/x/...` → 生态库。能用依赖和工具就不自研重复轮子；新增依赖必须说明理由与替代方案。

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
│   │   ├── server.go / router.go / respond.go / sse.go / middleware.go
│   ├── api/                      # ② 业务路由 handler（薄；一个文件一个功能域）
│   │   ├── handler.go            #   Handler 聚合 + Startup/Shutdown
│   │   ├── api_*.go              #   各功能域 handler
│   │   └── fileserver.go         #   本地受管文件 HTTP 服务（/files/**）
│   ├── service/                  # ③ 业务编排层（事务边界；不写 SQL；不引 gin/Wails）
│   ├── repo/                     # ④ 持久层（GORM；不引上层）
│   ├── domain/                   # ⑤ 域模型（一个聚合根一个文件，DO/DTO/REQ/VO/RESP 同居一处）
│   ├── core/                     # ⑥ Agent 内核（ReAct 循环 + 5 件护栏链 + 压缩 + 检查点 + 事件 + 上下文装配）
│   ├── llm/                      # ⑦ LLM 适配（provider + openai/anthropic/ollama/registry/toolcall）
│   ├── tool/                     # ⑧ 工具系统（registry + exec/file/http/websearch/webfetch/...）
│   ├── skill/                    # ⑨ Skill（parser/registry/builtin）
│   ├── mcp/                      # ⑩ MCP stdio 客户端（client/adapter/manager）
│   ├── capability/               # 能力接入：Preload（上下文注入）/ Tools（模型调用）/ Capture（run 后沉淀）三通道
│   ├── memory/                   # ⑪ 长期记忆（单一 MEMORY.md + FTS5 派生索引）
│   ├── rag/                      # ⑫ 知识库（loader/chunker/indexer/retriever）
│   ├── pet/                      # ⑬ 桌宠（controller/service/sprite）
│   ├── runtime/                  # ⑭ 运行时设施（paths/runtimes/archive）
│   ├── config/                   # ⑮ 配置（Viper）
│   ├── event/                    # ⑯ 应用内事件总线
│   ├── db/                       # ⑰ SQLite 打开 + 迁移
│   ├── pkg/                      # ⑱ 自研底层工具（叶子：无业务语义，AppError/ID/日志/路径/加密/HTTP 规则）
│   ├── tray/                     # ⑲ 系统托盘（Windows Win32 自研 + 非 Windows 空实现）
│   ├── singleinstance/           # ⑳ 单实例保护（命名互斥 + 本地 TCP IPC；非 Windows 空实现）
│
├── frontend/                     # Vue 3 工程
├── assets/                       # 内置 Skill（embed）/ 图标 / 默认 sprite
├── build/                        # 平台资源与产物
└── doc/                          # 设计文档
```

**禁止**：
- ❌ 在 `service / repo / api` 包内定义实体 / DTO / VO / 枚举（必须放 `domain/`）
- ❌ 错误码 / 工具类分散到各业务包（统一 `internal/pkg/`）
- ❌ 在 `main` 包内写业务代码（业务逻辑一律 `internal/`）
- ❌ `internal/pkg/` 内出现业务词汇（session / provider / workflow 等）
- ❌ `internal/pkg/` 依赖任何其他 `internal/` 业务包（叶子工具包铁律）
- ❌ `core/` import wails / api / service / server / gin

### 2.2 依赖方向（强制）

```
internal/server ──► internal/api ──► internal/service ──► 能力域（core/llm/tool/memory/rag/...）
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
| **internal/pkg** | 标准库 / golang.org/x / 第三方工具 | **任何其他 internal 业务包（叶子铁律）** |
| core | llm / tool / memory / pkg | api / service / server |

**双主机说明**：业务 API 走 gin HTTP，Wails 绑定只保留系统能力（对话框/剪贴板/托盘/窗口）。前端事件走 SSE（server/sse.go 桥接 event.Bus）。端口注入：`app:ready` 携带 `serverPort`。

### 2.3 聚合根文件组织（domain 单文件多形态）

一个聚合根一个文件，该实体所有形态同居一处：

```go
// internal/domain/ai_provider.go
package domain

type ProviderKind string                                  // 枚举
const ( ProviderKindOpenAI ProviderKind = "openai" /* ... */ )

var ErrProviderNotReady = pkg.New(3001, "provider 未就绪", "")  // 包级错误变量

type AiProviderDO struct { ... }      // 持久化实体——标配
type AiProviderDTO struct { ... }     // 域间传输——按需
type AiProviderREQ struct { ... }     // API 入参——按需
type AiProviderVO struct { ... }      // 视图对象——按需
type AiProviderRESP struct { ... }    // API 出参——按需
```

| 后缀 | 语义 | 标配 |
|---|---|---|
| DO | Domain Object，映射数据库表 | 必有 |
| DTO | 能力域间传输载体 | 按需 |
| REQ | 前端 → 后端入参 | 按需 |
| VO | 组装后的视图对象 | 按需 |
| RESP | 后端 → 前端出参 | 按需 |

### 2.3.1 内联结构体边界（强制）

`server / api / service / repo` 四层禁止就地声明入参/出参/实体结构体，必须引用 `domain.XxxREQ/DTO/VO/RESP/DO`。

例外：`internal/tool/*` 内的快速参数解析允许内联（须加注释 `// 内联快速解析；不暴露为领域类型`）；`internal/repo/*` 内 GORM 临时查询条件允许内联（禁止作为返回值或跨方法传递）。

### 2.4 错误处理（强制 AppError）

```go
// internal/pkg/apperror.go
func New(code int, message, details string) *AppError
func Wrap(code int, message string, err error) *AppError
```

```go
// ✅ 业务错误
if path == "" { return nil, pkg.New(1001, "路径不能为空", "") }
// ✅ Wrap 底层错误
if err := os.WriteFile(p, data, 0o644); err != nil { return pkg.Wrap(1002, "写文件失败", err) }
// ✅ 包级错误变量（domain 文件顶部集中声明）
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
| 5000–5999 | Agent / Harness（含会话与消息编排） |
| 6000–6999 | Memory |
| 7000–7999 | Knowledge / RAG |
| 8000–8999 | Skill / MCP |
| 9000–9999 | Pet |

### 2.5 命名规范

| 类别 | 风格 | 示例 |
|---|---|---|
| 文件名 | snake_case.go | ai_provider.go |
| 包名 | 全小写无下划线 | agent / tool / memory |
| 类型 / 接口 | PascalCase（不加 I 前缀） | AiProviderDO / Provider |
| 函数 | 导出 PascalCase / 私有 camelCase | ListProviders / resolveOne |
| 变量 | camelCase | runID / compressRatio |
| 错误变量 | ErrXxx | ErrSessionNotFound |
| JSON tag | **snake_case（下划线小驼峰）** | json:"user_message_id" / json:"message_count" |
| GORM tag | snake_case | gorm:"primaryKey;size:64" |
| ID 前缀 | SCREAMING_SNAKE_CASE | PROVIDER / SESSION |

**禁止**：接口加 I 前缀；`ToolInfo` 类冗余后缀；匈牙利命名；`xxxId` 与 `xxxID` 混排。

**JSON 契约铁律（前后端）**：所有 HTTP 交互字段一律 snake_case。多单词字段用下划线分隔：`message_count` / `user_message_id` / `last_message_at` / `api_key_masked`。前端 `types/api.ts`、store、组件消费的字段名必须与后端 RESP 的 json tag 完全一致。后端 domain 结构体禁止出现 `json:"userMessageID"` 这类 camelCase tag。

### 2.5.1 LLM 协议 DTO 例外

`internal/llm/openai|anthropic|ollama/client.go` 是上游 API 响应反序列化 DTO，json tag 必须忠实上游字段名（上游本身为 snake_case，如 `json:"tool_calls"`）。归一化层（`llm/message.go` 的 Message / TokenUsage）同样使用 snake_case json tag，与业务契约保持一致。

### 2.6 注释规范（强制精简）

- 包：1 行职责；类型：2~4 行概要；方法：签名级；字段：仅在不显然时 1 行；
- 文件头注释（含 Vue/TS 的 JSDoc）不超过 5 行；端点 / 事件清单一律引 doc/16，不在注释里重复；
- 行内只解释「为什么」；
- 禁止：过程性内容、长篇 HTML 注释、TODO 历史、实现细节；
- 注释只描述**最终设计与实现**：禁改动过程（「修复」「原…」「新增」叙事）、里程碑标记、外部参考项目名与「参考/借鉴」字样。

### 2.6.1 文档规范（doc/）

- 每篇只写现状：定位 → 设计 → 核心契约（表/字段/端点/事件与代码一致）→ 关键流程 → 约束；
- 表格与短段落优先；不写 TODO 与未来计划；README 索引与文件清单保持同步。

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
| 日志 | slog 仅 info/warn/error 三等级；warn、error 单独落文件（workbaby-warn.log / workbaby-error.log）+ 轮转；ctx 注入 sessionID/runID |
| 上下文压缩 | MicroCompressor（确定性折叠，不调 LLM） |
| 并发控制 | 有界 goroutine 池 |
| 事件总线 | event.Bus + server/sse 桥接 |
| Token 计量 | 每次 LLM 调用一行 token_usages（core TurnUsage → service.persistUsage），仪表盘三线图唯一数据源 |
| 配置 | Viper + system_settings KV |
| 加密 | AES-256-GCM（Provider API Key、MCP env） |

禁止引入：OpenTelemetry / Prometheus 客户端；AOP 框架；全局可变状态。

### 2.11 数据库迁移

- v1：GORM AutoMigrate；新增字段只增不删、零值兜底；
- v2：golang-migrate；PR 必带 up/down sql。

FTS5 虚拟表与触发器用 raw SQL 启动期单独创建。

### 2.12 HTTP 与 API 路径约定（强制）

| 维度 | 规则 |
|---|---|
| HTTP 方法白名单 | 业务 / 工具 / 工作流节点仅允许 GET 与 POST |
| 路径参数位置 | 变量参数放路径末尾（`/xxx/:id/delete` 风格） |
| 删除语义 | 删除走 `POST .../delete` |
| 流式事件 | `GET /api/v1/events?scope=chat&runId={runId}` SSE |

**路径风格唯一标准**：`/api/v1/{resource}/:id/{action}`，示例：
- 中断会话正在跑的 run：`POST /api/v1/chat/sessions/:id/cancel`（`:id` = sessionID）
- 从检查点续跑：`POST /api/v1/chat/runs/:id/resume`（`:id` = runID）
- 删除：`POST /api/v1/chat/sessions/:id/delete`
- 切换模型：`POST /api/v1/chat/sessions/:id/model`
- 切换权限模式：`POST /api/v1/chat/sessions/:id/permission`

**前端调用必须与后端路由完全一致**：禁止前端出现 `/xxx/delete/:id` 这类与后端 `/:id/delete` 不一致的路径。前端 API 层收敛在 `frontend/src/src/api/`，路径白名单与后端 router.go 一一对应，杜绝 404。

白名单逻辑收口在 `internal/pkg/httprules.go`（`AllowMethod` / `NormalizeMethod`），禁止在 internal 内重复定义。

**例外**：llm Provider 适配层调上游 API 可用全部 method；mcp 的 JSON-RPC Method 字段是 RPC 方法名。

---

## 3. 测试规范

**原则**：
- 测试函数 `func TestXxx(t *testing.T)`；子测试用 t.Run；测试名不带里程碑 / 版本号标记；
- 重点覆盖复杂流程 / 长链路 / 跨模块集成 / 安全护栏 / 协议归一化；
- 简单 CRUD / 字段映射 / 纯字符串函数 / 工具冒烟不保留测试；
- 文件按能力域聚合：一个模块 / 一类关注点放一个 `_test.go`，禁止把同一件事（如 runner 的 ReAct 行为）拆成多份文件；mock / fixture 同包共享；
- 依赖真实外网的用例 `testing.Short()` 跳过；
- service 层测试不引入 gin / Wails；server 层用 httptest。

**保留判据（决定一个测试写不写）**——写之前先问：这个测试失败时，是否意味着某个**跨模块/跨轮次/跨协议**的行为坏了？是 → 写；否 → 不写。

| 保留 | 删除 |
|---|---|
| ReAct 多轮循环 / 检查点续跑 / 子 Agent 隔离等**多轮次链路** | 单个纯函数（解析器、格式化、getter）的输入输出 |
| 压缩不拆散 assistant+tool 对等**协议硬约束** | 简单 CRUD、字段映射、枚举转换 |
| 审批 / 信任 / 注入防护 / 路径穿越等**安全护栏** | 幂等的 setter 覆盖、构造函数冒烟 |
| SSE 断线重放 / 文件变更快照回滚等**集成编排** | 同一行为在不同文件里的重复断言 |

**规模约束**：单个测试文件 ≤ 6 个测试函数；同类行为用 table-driven 合并进一个函数（子测试 `t.Run` 区分场景），禁止为每个边界单开一个 `TestXxx`。环境依赖（系统 shell、真实网络）用 `exec.LookPath` / `testing.Short()` 守卫后跳过，不得让整个包变红。

```go
func TestRunnerNormalReactLoop(t *testing.T) {
    mockLLM := &mockLLM{responses: []llm.ChatResponse{
        {ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{"msg":"hi"}`)}}},
        {Message: llm.AssistantMsg("done")},
    }}
    r := core.New(mockLLM, registry, core.Config{MaxTurns: 5}).
        WithSink(core.FuncSink(func(e core.Event) { /* 断言 */ }))
    out, err := r.Run(context.Background(), []*llm.Message{llm.UserMessage("hello")})
    require.NoError(t, err)
    require.Equal(t, core.ReasonEndTurn, out.Reason)
    assert.Equal(t, 2, mockLLM.callCount)
}
```

---

## 4. 架构门禁（scripts/check-boundaries.ps1）

- domain-no-upward / repo-no-upward / service-no-http / server-no-downward / core-no-upward / core-no-http / pkg-no-internal

```bash
powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1
powershell -ExecutionPolicy Bypass -File scripts/check-contract.ps1   # 契约门禁：cancel/resume 拆分 + 前后端路径段一致
```

---

## 5. 工作流

- 优先用 codegraph 做代码理解；单次失败立即降级 grep + read；
- 最小改动：外科手术原则；改动前后跑测试；
- 验证优先：修 Bug 先写复现测试；
- YAGNI：禁止过早抽象、单实现接口；
- 不要猜测：模糊需求先明确假设与边界。

---

## 6. 验收命令

```bash
go build ./...                                     # 全量编译
go test ./internal/...                             # 后端测试
go vet ./internal/...                              # 静态检查
cd frontend && npm run typecheck && npm test        # 前端类型检查 + 测试
powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1   # 架构门禁
wails dev                                          # 开发模式
wails build -nsis -ldflags "-s -w" -trimpath       # 生产构建（NSIS 安装包）
```

---

## 7. 关键架构决策

- 单一聚合根文件：domain/{name}.go 含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量；
- internal/pkg 为叶子工具包：各层可依赖，自身不依赖任何其他 internal 业务包；
- 双主机通信：业务 API 走 gin HTTP（POST/GET + {code,message,data}），流式走 SSE；
- 端口注入：gin 监听 127.0.0.1:0，app:ready 携带 serverPort；
- Wails 绑定仅系统能力；
- 存储语义：DB 存元数据 + 内容；文件作配置源/快照/导出；model.json / mcp.json 是配置真相源（DB 同步）；
- API Key 与 MCP env 用 AES-256-GCM 加密，主密钥首启随机生成并写入 config.yaml；
- thinking 与 content 严格分离；
- 网络搜索 DuckDuckGo 免 Key；
- 本地单用户，gin 只绑定 127.0.0.1 回环 + 随机端口；
- **HTTP 交互字段一律 snake_case（下划线小驼峰）**；
- **前端 API 路径与后端路由严格一一对应**，杜绝 404。

---

## 8. 反模式

- ❌ 大杂烩式改动 / 错误提前抽象 / 隐形架构决策 / 只覆盖乐观路径 / 臆造 API / 代码风格漂移 / 失控式连锁重构 / internal/pkg 依赖其他 internal 业务包
