# AGENTS.md · WorkBaby 工程规范

> 本文件是代码风格 / 分包 / 依赖方向 / 工作流的唯一权威。
> 设计说明见 `docs/` 与 `specs/`；冲突时以本文件为准。

---

## 1. 技术栈（强制锁定）

| 层 | 选型 | 版本 |
|---|---|---|
| 语言 | Go | 1.24.x（单 module `WorkBaby`） |
| 桌面壳 | Wails | v2.12.x（Frameless，前端自绘标题栏） |
| HTTP 服务 | gin | ^1.10.x（业务 API + SSE 全部走 HTTP） |
| 前端 | Vue 3 + TypeScript + Vite | 3.5 / 5.6 / 6 |
| 状态管理 | Pinia | ^2.3.x（Setup Store） |
| 路由 | Vue Router | ^4.5.x（hash 模式） |
| HTTP 客户端 | Axios | ^1.7.x |
| 样式 | 原生 CSS 设计令牌（`themes.css` + `wb-ui.css`） | 无 CSS 引擎依赖；组件全自绘，无 UI 组件库 |
| 渲染 | marked · dompurify · highlight.js | markdown 唯一出口 `utils/md.ts` |
| ORM | GORM + glebarez/sqlite | ^1.30.x（pure-Go，无 CGO） |
| 数据库 | SQLite | WAL + FTS5 trigram |
| 配置 | Viper | ^1.19.x（YAML + ENV；运行时配置走 KV 表） |
| Schema 校验 | santhosh-tekuri/jsonschema/v6 | ^6.0.x |
| 日志 | log/slog（标准库） | info/warn/error 分文件落盘 |
| 实时通信 | SSE | 业务实时通信全部走 SSE（`/api/v1/events`），不引入 WebSocket |
| 测试 | testing（标准库） | 全量（含归档解压）约 10s；`-short` 秒级 |
| ID | oklog/ulid/v2 | 业务 ULID 带前缀 |
| 加密 | AES-256-GCM（标准库） | Provider API Key |
| 托盘 | getlantern/systray | 关闭到托盘 |
| 内置运行时 | Python + PowerShell | `backend/runtime/bundled/`（go:embed 进 exe，归档经 LFS 托管），其余一律不内置 |

### 1.1 依赖原则

标准库 → `golang.org/x/...` → 生态库。新增依赖必须说明理由与替代方案。

**明确不引入**：UI 组件库（Element Plus 等）、工作流引擎（BPMN）、微前端（Qiankun）、
富文本编辑器、UnoCSS、ECharts/AntV X6、Vue I18n、通用工具库——当前功能面用不到，
引入只增维护成本。

---

## 2. 强制编码规范

### 2.1 包结构

```
WorkBaby/
├── main.go                       # 入口：embed + wails.Run + 单实例守卫
├── app.go                        # App 结构体（嵌入 *api.Handler，生命周期委托）
│
├── backend/
│   ├── server/                   # ① HTTP 层（gin）：路由 / SSE hub / 统一响应 / 方法白名单
│   ├── api/                      # ② 业务 handler（薄）+ 系统能力绑定（唯一允许 import wails 的包）
│   ├── service/                  # ③ 业务编排层（事务边界；不写 SQL；不引 gin/Wails）
│   ├── repo/                     # ④ 持久层（GORM；不引上层）
│   ├── domain/                   # ⑤ 域模型（一个聚合根一个文件，DO/DTO/REQ/VO/RESP 同居一处）
│   ├── agent/                    # ⑥ Agent 内核（单层流式循环 + 上下文清洗 + 队列）
│   ├── llm/                      # ⑦ LLM 适配（llm.go 归一化 + retry + factory/ + openai/ anthropic/ ollama/）
│   ├── tool/                     # ⑧ 工具系统（registry + files/exec/python/web/search + runtime）
│   ├── skill/                    # ⑨ Skill（parser/registry/loader）
│   ├── knowledge/                # ⑩ 知识库（loader/chunker/service + FTS5 检索）
│   ├── runtime/                  # ⑪ 路径解析 + 内置 Python / PowerShell 运行时（bundled/ 归档 go:embed + 解压）
│   │   └── runtimetest/          #    测试夹具：预置运行时标记，装配类测试跳过解压（只被测试引用）
│   ├── config/                   # ⑫ 配置（Viper YAML + MasterKey）
│   ├── db/                       # ⑬ SQLite 打开 + 迁移 + FTS5 虚表
│   ├── pkg/                      # ⑭ 叶子工具包（AppError/ID/日志/加密/httprules/fsutil）
│   ├── tray/                     # ⑮ 系统托盘
│   └── singleinstance/           # ⑯ 单实例保护（文件锁 + 本地 TCP IPC）
│
├── frontend/                     # Vue 3 工程（源码在 frontend/src/src）
├── assets/                       # 内置 Skill（embed）
├── build/                        # 平台资源与产物
├── docs/                         # 项目级文档：架构 / 数据模型 / API 契约 / 页面 / 开发 / 部署
├── specs/                        # 子系统规格（01-14）
└── tools/                        # Go 写的独立门禁工具（check-boundaries）
```

**禁止**：

- 在 `service / repo / api` 包内定义实体 / DTO / VO / 枚举（必须放 `domain/`）
- 错误码 / 工具类分散到各业务包（统一 `backend/pkg/`）
- 在 `main` 包内写业务代码
- `backend/pkg/` 内出现业务词汇（session / provider / chat 等）
- `backend/pkg/` 依赖任何其他 `backend/` 业务包（叶子工具包铁律）
- `agent/` import wails / api / service / server / gin

### 2.2 依赖方向（强制）

```
backend/server ──► backend/api ──► backend/service ──► 能力域（agent/llm/tool/knowledge/skill）
                                          │
                                          ▼
                                     backend/repo ──► backend/domain
                                          │
                                          ▼
                              backend/pkg（叶子工具包：各层可依赖，自身不依赖任何 backend 业务包）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| server | api / domain / pkg / gin | service / repo / 能力域 |
| api | service / domain / pkg / gin / wails | repo / 能力域 |
| service | repo / domain / pkg / 能力域 | api / server / gin / wails |
| 能力域 | repo / domain / pkg / 其他能力域（经接口） | api / service / server / wails |
| repo | domain / pkg / gorm | api / service / 能力域 |
| domain | pkg | 任何上层 |
| **backend/pkg** | 标准库 / golang.org/x / 第三方工具 | **任何其他 backend 业务包** |
| agent | llm / tool / pkg | api / service / server / repo |

**双主机**：业务 API 走 gin HTTP，Wails 绑定只保留系统能力（对话框/剪贴板/窗口）。
前端事件走 SSE。端口注入：`app:ready` 携带 `server_port`。

### 2.3 聚合根文件组织（domain 单文件多形态）

一个聚合根一个文件，该实体所有形态同居一处：

| 后缀 | 语义 | 标配 |
|---|---|---|
| DO | Domain Object，映射数据库表 | 必有（必须显式 `TableName()`，防 GORM 把 DO 复数化成 `_dos`） |
| DTO | 能力域间传输载体 | 按需 |
| REQ | 前端 → 后端入参 | 按需 |
| VO | 组装后的视图对象 | 按需 |
| RESP | 后端 → 前端出参 | 按需 |

包级错误变量集中在文件顶部声明（`var ErrXxx = pkg.New(...)`）。

### 2.3.1 内联结构体边界（强制）

`server / api / service / repo` 四层禁止就地声明入参/出参/实体结构体，必须引用 `domain.XxxREQ/DTO/VO/RESP/DO`。

例外：`backend/tool/*` 内的快速参数解析允许内联（须加注释）；`backend/repo/*` 内 GORM 临时查询条件允许内联；
上游第三方接口的响应解析载体允许就地声明（与 §2.5.1 同理——它不是我们的入参 / 出参，
字段名还要忠实对方，目前只有 `service/provider.go` 解析 `/models` 列表那一处）。

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
// ❌ 禁止
return errors.New("provider not ready")   // 丢 code，前端无法分流
```

**错误码段位**：

| 段位 | 域 |
|---|---|
| 1000–1999 | 通用 / 文件 / 路径 |
| 2000–2999 | 配置 / 持久化 / DB |
| 3000–3999 | LLM / Provider |
| 4000–4999 | 工具 / 审批 |
| 5000–5999 | Agent / 内核 |
| 6000–6999 | Knowledge |
| 7000–7999 | Runtime（内置运行时） |
| 8000–8999 | Skill |
| 9000–9999 | 保留 |

每个码的完整含义与处置动作见 [`docs/ERROR-CODES.md`](docs/ERROR-CODES.md)。

### 2.5 命名规范

| 类别 | 风格 | 示例 |
|---|---|---|
| 文件名 | snake_case.go | ai_provider.go |
| 包名 | 全小写无下划线 | agent / tool / knowledge |
| 类型 / 接口 | PascalCase（不加 I 前缀） | ProviderDO / Streamer |
| 函数 | 导出 PascalCase / 私有 camelCase | ListProviders / resolveOne |
| 变量 | camelCase | runID / compressRatio |
| 错误变量 | ErrXxx | ErrSessionNotFound |
| JSON tag | **snake_case** | json:"user_message_id" |
| GORM tag | snake_case | gorm:"primaryKey;size:64" |
| ID 前缀 | SCREAMING_SNAKE_CASE | PROVIDER / SESSION |

**禁止**：接口加 I 前缀；匈牙利命名；`xxxId` 与 `xxxID` 混排。

**JSON 契约铁律**：所有 HTTP 交互字段一律 snake_case。前端 `types/api.ts`
消费的字段名必须与后端 RESP 的 json tag 完全一致。

### 2.5.1 LLM 协议 DTO 例外

`backend/llm/openai|anthropic|ollama` 下的 `*.go` 是上游 API 响应反序列化 DTO，
json tag 必须忠实上游字段名。归一化层（`llm/llm.go`）使用 snake_case。

### 2.6 注释规范（强制精简）

注释的读者是「半年后要改这段代码的人」。他要的是**这个东西是什么、边界在哪**，
不是作者的思考过程。

| 位置 | 要求 |
|---|---|
| 文件头 | 1 行说清这个文件的职责（最多 2 行） |
| 包 | 1 行 |
| 类型 | 1 行说它是什么；契约类类型可到 2 行说清边界 |
| 方法 | 1 行签名级说明（干什么 / 返回什么），不写实现步骤 |
| 字段 | 名字不自解释时写 1 行；枚举与哨兵值必须写清取值含义 |
| 行内 | 只写「为什么」，1 行 |

- **最低要求就这三样**：文件有头、导出方法有签名说明、字段有注释。够了就停——注释不是设计文档
- **连续注释块 ≤ 2 行**；方法注释的第二行只在「不这么做会出事」时才留
- 注释只写**结论与边界**，不写论证：「为什么这么设计、当初踩过什么坑」属于 `docs/` 与 `specs/`
- 禁止：过程性叙述、TODO 历史、实现细节叙事、外部项目名与「参考 / 借鉴」字样

### 2.6.1 文档规范

文档只回答三件事：**这个项目是什么、每个模块怎么设计实现、为什么这么选型**。

- 根级：`README.md`（入口与能力全景）/ `AGENTS.md`（工程规范，唯一权威）/ `DESIGN.md`（视觉语言）
- `docs/`：项目级说明（架构 / 数据模型 / API 契约 / 页面 / 开发 / 部署）
- `specs/`：子系统规格，编号 01-14，一个子系统一个文件
- 每篇只写现状：定位 → 设计 → 契约 → 关键流程 → 约束 → 取舍。**优势与代价必须写出来**
- 表格与短段落优先；能用表就不用列表
- **不写改动过程**：不建 CHANGELOG，不记录「本次改了什么」「之前坏在哪」，
  设计变更直接改进对应文档；历史由 Git 承载
- 不写外部项目名与「参考 / 借鉴 / 类似 X」——只讲自己怎么设计、为什么这么设计
- 文档必须与代码同步：改行为时同时改对着这篇行为的文档与注释，不同步视为未完成

### 2.7 全局 ID：ULID 大写 + 领域前缀

```go
func NewID(prefix string) string   // "SESSION_01ARZ3NDEKTSV4RRFFQ69G5FAV"
```

前缀常量集中在 `backend/domain/id.go`。本机用户 id 固定 `"local"`。

### 2.8 时间戳统一毫秒整数

```go
CreatedAt int64 `gorm:"autoCreateTime:milli" json:"created_at"`
```

禁止 `time.Time` / `time.Duration` 出现在跨边界 struct。

### 2.9 平台特定代码（build tag）

```go
//go:build windows
```

禁止一个文件里 `runtime.GOOS` 分支处理所有平台（托盘 / 剪贴板等系统能力必须分文件）。

### 2.10 横切关注点

| 关注点 | 方案 |
|---|---|
| 统一错误 | AppError + 错误码分段 |
| 日志 | slog 仅 info/warn/error；分文件落盘；ctx 注入 sessionID/runID |
| 实时通信 | SSE only（256 缓冲 / 慢客户端先挤 delta 保关键事件 / 重连带 `last_event_id` 重放对账） |
| 压缩 | 确定性清洗 + 按 token 预算找切点，不调 LLM（见 spec 02） |
| 工具输出 | 双通道：`Content` 回模型、`Detail` 给 UI；2000 行 / 50KB 双上限截断，超出落临时文件并回报丢弃量；命令 / 脚本类保留结尾（`CutTail`），其余保留开头 |
| 事件出口 | `service.Emitter` 唯一出口（注入归属 + 分配 seq + sink 注入） |
| Token 计量 | 每次 LLM 调用一行 token_usages；`GET /stats` 按天/模型/会话聚合 |
| 配置 | Viper + settings KV 表 |
| 加密 | AES-256-GCM（Provider API Key） |
| 单实例 | 文件锁 + 本地 TCP IPC 转交文件路径 |

禁止引入：OpenTelemetry / Prometheus 客户端；AOP 框架；全局可变状态。

### 2.10.1 前端设计令牌（强制）

视觉语言见 `DESIGN.md`。核心约束：

- 色值只在 `themes.css` 定义（`--wb-*` 语义令牌 + `[data-theme]` 块）
- 组件禁止写死颜色：用 `var(--wb-*)`，不用 hex
- 分层只靠「1px 中性描边 + 极轻阴影」：禁渐变、禁彩色描边、禁 backdrop-blur 叠层
- 控件高度四档：`--wb-ctl-h-sm`(26px) / `--wb-ctl-h`(34px) / `--wb-ctl-h-lg`(38px) / `--wb-ctl-h-xl`(44px)，另有图标钮 `--wb-ctl-icon`(32px)；禁止裸像素高度
- 装饰性图标瓦片用 `--wb-tile-sm`(30px) / `--wb-tile`(40px) / `--wb-tile-lg`(52px)：它们不是控件，但同一组尺寸反复出现，散成裸像素就会各自漂
- 按钮五态必须齐全（默认 / 悬停放大 / 按下缩小 / 禁用 / 加载）：每个可点元素都要给「点到了」的反馈
- 危险按钮用中性描边 + 危险色文字：彩色描边违反分层纪律，语义交给文字与底色
- 字号用刻度 `--wb-fs-*`；禁止任意像素值
- 字体只有三个语义变量：`--font-sans` / `--font-mono` / `--font-display`，全部在 `themes.css` 定义；`html, body` 必须显式声明 `font-family` 与 `font-size`
- 空态与骨架统一用 `EmptyState` / `PageState`；禁止 `el-empty` / `el-skeleton`
- 按钮胶囊镂空：`wb-ui.css` 的 `.btn` 族是唯一实现
- 图标统一走内联 SVG（`.ic` 类已按 24 网格写好 `stroke-width`）；禁止用 Unicode 字符或 Emoji 冒充图标
- 事件 data 的字段名必须与后端 json tag 一致（snake_case）；禁止 `as never` 之类的强转绕过类型检查

### 2.11 数据库迁移

- GORM AutoMigrate：新增字段只增不删、零值兜底；废弃字段留在表里（改语义靠代码兼容）
- FTS5 虚拟表与触发器用 raw SQL 启动期逐条创建（多语句一次 Exec 不可靠）

### 2.12 HTTP 与 API 路径约定（强制）

| 维度 | 规则 |
|---|---|
| HTTP 方法白名单 | 业务仅允许 GET 与 POST（`backend/pkg/httprules.go` 收口） |
| 路径参数位置 | 变量参数放路径末尾（`/xxx/:id/delete` 风格） |
| 删除语义 | 删除走 `POST .../delete` |
| 流式事件 | `GET /api/v1/events?session_id={sid}` SSE，Last-Event-ID 重放 |

**路径风格唯一标准**：`/api/v1/{resource}/:id/{action}`。前端 API 层收敛在
`frontend/src/src/api/index.ts`，路径与 `backend/server/routes.go` 一一对应。

**例外**：llm Provider 适配层调上游 API 可用全部 method。

### 2.13 Agent 内核契约（强制）

`backend/agent` 是整个工程最核心的子系统。

#### 2.13.1 主循环形态（单层流式）

```text
Run(ctx):
  for turn = 1..MaxTurns:
    msgs = Compact(msgs, Budget)          # 超预算才动手，纯函数
    pending = drain(steering)            # 轮间插话：并进下一轮上下文
    append(pending)
    msg    = streamTurn(ctx, msgs)       # 流式调模型，边收边发事件；预算 = Config.MaxTokens
                                         # 上游拒绝时降级重试（压缩 / 收预算），最多两次
    if stop in {error, aborted}: 收尾退出
    if stop == length: drop(msg.tool_calls)  # 半个 JSON 不执行也不入历史
    append(msg)
    if stop == length: break             # 截断轮的 tool_calls 一律不执行
    if len(calls) == 0: break            # 纯文本回复 = 回合自然结束
    results = executeTools(calls)        # 按声明顺序回填，协议配对完整
    append(results)
    emit turn_end
```

简洁优先：不做双层循环，不做 PrepareNextTurn 挂载点。压缩与模型切换都在
service 层发送前完成。

**为什么是单层而不是「循环停了自动续跑」**：自动续跑是为终端 REPL 设计的——
用户不停敲键，循环不能停。桌面端有明确的「插话 / 排队」按钮，语义由 UI 表达；
内核再套一层自动续跑，只会让「什么时候注入」变成隐式行为。跟进消息与插话
同属一个队列，在轮间注入，效果一致，少一个状态机。

**为什么没有 PrepareNextTurn 挂载点**：压缩是纯函数（清洗 + 按预算找切点），不需要在
轮间回调里做副作用。`Config.Budget` 由 service 层备好，内核在每次发送前
按预算调用 `agent.Compact`，压缩是发请求的必经之路而不是可选钩子。

#### 2.13.2 队列

```go
type Queue struct{ ... }        // 一次 drain 一条（one-at-a-time）
func NewQueue() *Queue
func (q *Queue) Enqueue(m *llm.Message)
func (q *Queue) Drain() []*llm.Message
func (q *Queue) HasItems() bool
```

插话（steer）与排队（followUp）共用一个队列：轮间注入、立刻生效。
队列不区分两种语义——UI 决定叫它「插话」还是「排队」，内核只管轮间取出。

#### 2.13.2.1 工具闸门

审批是内核唯一的回调面，收成一个函数而不是钩子集合：

```go
type Gate func(ctx context.Context, call *llm.ToolCall) (blocked bool, reason string)
```

返回 `blocked=true` 时该调用不执行，直接生成一条 `IsError` 的工具结果，
让模型看到「这个操作没做」并自行换方案。压缩、模型切换都不走回调。

#### 2.13.3 工具 ExecutionMode

```go
type ExecutionMode string
const (
    ExecutionSequential ExecutionMode = "sequential"
    ExecutionParallel   ExecutionMode = "parallel"
)
```

声明 `Parallel` 且调用数 >1 时走信号量并发，结果按调用顺序回填。

#### 2.13.4 不变量

| 不变量 | 锁死原因 |
|---|---|
| 工具结果严格按调用顺序回填 | `assistant(tool_calls)` 与 `tool` 配对完整，错位上游 400 |
| 事件出口必须串行化 | 并行工具各自在 goroutine 里 emit，不锁就是并发改同一份落库位点，assistant 声明撞主键后整条丢失 |
| 被截断轮的 tool_calls 一律不执行 | 半个 JSON 调用执行出去比不执行更危险；留进历史还会被当成真调用回传上游 |
| 输出预算必须显式下发到上游 | 0 在 openai 协议里被 `omitempty` 整个吃掉、在 anthropic 协议里是必填字段，两条协议都要兜到 `llm.DefaultMaxTokens`；推理型模型的思考会吃满小预算，正文与工具调用一起断在 length |
| 输出预算 = 窗口 1/8（目录口径下再与厂商硬上限取小） | 窗口 1/8 保证长回答不被随手截断；窗口来自目录时它的硬上限与窗口同源，一并取小（1M 窗口按 1/8 下发 12.5 万可能超过厂商真实上限）。窗口被用户手填（`model_configs` / `context_window`）时硬上限不参与：它描述的是同名模型在别的服务上的样子，此时按用户声明的窗口 1/8 走 |
| 上下文余量必须容得下输出预算 | 余量缺省就是输出预算本身：余量比实际下发值小，压缩会按虚高的空间往窗口里塞内容，总占用顶破窗口 |
| 上下文预算必须含工具声明与图片 | 工具 schema 每轮都随请求发出去，图片按张占 token：只数消息正文，水位读数偏低、压缩来得太晚（`Budget.ToolsTokens` / `EstimateTokens`） |
| 采样参数只在用户显式配置时下发 | 用一个拍脑袋的默认温度覆盖所有模型，会在推理型与思考型模型上撞上游约束；持久化层用 `SamplingUnset=-1` 表达未设置（0 是合法的确定性取值） |
| 内核不擅自抬高输出预算 | 内核不知道模型能吐多少，抬过头就是把「能看懂的截断」换成「看不懂的 400」；到顶就以 length 收尾，交给界面的「继续」。只有上游明确拒绝这个预算时才降级重试：报错里的上限优先、没写就对折，最多两次、下限 4096 —— 只降不升 |
| 用量口径在 turn_end 之前归一 | 命中量大于输入量时界面会算出超过 100% 的命中率、输入量也偏小（长对话的成本被读低）；`llm.Usage.Normalize` 把两种计费口径补成「Input 含缓存」，读侧再兜一次 100% |
| 上下文读数取「上游实收输入」与本地估算的较大者 | 同一张卡片上的「入」与「上下文」必须是一回事：估算偏保守时用实测值纠正，否则出现「入 9.9K / 上下文 25K」这种自相矛盾的读数 |
| 上游报上下文超限 → 强制压缩重试一次 | 本地估算是粗估，偏乐观时上游拒绝整轮；本轮已吐出过内容则不重试（避免重复正文），第二次仍失败才以错误收尾 |
| 「报错但零产出」轮不算成功 | 否则空 assistant 落库，下一轮直接 400 |
| 工具失败必须带原因（模型 / 界面 / 日志三处） | `Result.Detail` 为空时由 `agent.runOne` 补成 `Content`：只把原因留在给模型的那份里，工具卡展开是空白、日志只剩 `ok=false`，用户与开发者都查不出为什么 |
| CleanForProtocol 输出必须可直发上游 | 空消息/孤儿结果/未配对调用都在这一层兜底 |
| 事件发布永不阻塞内核 | Hub 慢客户端先挤 delta 断连兜底 + `chat:gap` 对账 |
| 同一工具同一参数整个 run 内累计 3 次后拦 | 模型卡在同一个调用上打转，比撞轮数上限更难排查；计数不衰减——中间穿插别的调用不代表它换了方案 |
| 并发工具 goroutine 在起之前取信号量 | 信号量放在 goroutine 里的话 N 个调用先起 N 个 goroutine 再抢锁，「并发度有上界」是假的——一次 30 个工具就铺 30 个 goroutine |
| 工具 panic 在串行与并行两条路径都 recover | panic 说明有 bug，但桌面端不该因为一个工具的 bug 把整进程连同这次对话一起带走；`runOneSafe` 统一兜底转成 `IsError` 结果并打堆栈——只兜并发路径等于没兜，write / edit / powershell / python 都是串行工具 |
| 用户文件写入必须原子替换 | 临时文件 + rename；磁盘写满或进程被杀留下半截文件比写失败更糟，助手写的是用户的真实工程文件 |
| 插话队列必须有上限 | 队列无上限时连点/脚本可无限堆积，每条都要在轮间注入进上下文，内存与 token 双爆；满时丢最旧并回执错误 |
| 端口握手必须早于 `domReady` 广播 | 监听器晚一步端口就为空，前端所有接口 404 |
| 就绪状态用响应式值传，不靠事件通知 | 事件在监听器注册前派发会丢，组件挂载时要读到的是当前值 |
| 启动失败必须走 Wails 事件总线 | 此时 HTTP/SSE 还不存在，走 `Emitter` 等于没发，用户只会看到空窗口 |
| `domReady` 必须等 `OnStartup` 完成 | Wails 把 `OnStartup` 放独立 goroutine，两者无顺序保证；靠「Svc 是不是 nil」判断会误判慢启动 |
| `beforeClose` 必须在退出流程中放行 | 无条件拦截会让托盘「退出」变成空操作，进程留驻后台占住 exe |
| 窗口关闭必须走 `Quit`（触发 `OnBeforeClose`） | 直接 `WindowHide` 绕过判定，把「关闭」一律变成收托盘，关掉驻留设置也退不掉 |
| 托盘退出后必须等消息循环收尾 | 进程抢在 `NIM_DELETE` 之前退出，通知区会留下摘不掉的幽灵图标 |
| 托盘消息循环必须锁定 OS 线程 | `GetMessage` 与托盘窗口创建分属不同线程，菜单点击与退出回调一起失灵 |
| 读锁必须配 `RUnlock` | 配成 `Unlock` 会 panic，且调用点在每轮对话的 `BuildSystem` 里 |

详见 `specs/01-agent-loop.md` / `02-context-compaction.md`。

---

## 3. 测试规范

**原则**：测试失败时必须意味着某条真实链路坏了。

### 3.1 保留判据

写之前先问：**这个测试失败时，是否意味着某个跨模块 / 跨轮次 / 跨协议的行为坏了？**

| 保留 | 删除 |
|---|---|
| 多轮循环 / 工具顺序回填 / 中断收尾 | 单个纯函数的输入输出 |
| CleanForProtocol 等协议硬约束 | 简单 CRUD、字段映射、枚举转换 |
| 审批闭环 / 路径穿越 / 写前必读等安全护栏 | 幂等 setter、构造函数冒烟 |
| FTS 检索与短查询兜底 | 只验证「不 panic」「非 nil」的弱断言 |
| 服务层集成：建会话→发送→落库 | 同一行为在不同文件里的重复断言 |
| 跨进程边界的端到端（HTTP / SSE / IPC） | 结构存在性检查（某张表在不在） |

同一个行为只在一个地方断言。**重复断言是测试的负债**：改一次行为要改 N 处，
而其中任何一处的失败都不会告诉你更多东西。

### 3.2 规模约束

- **一个 `Test` 讲一条链路**：同类断言用 `t.Run` 归到同一个 Test 下，
  失败时从输出就能看出是哪条链路、哪个分支坏了
- 单个测试文件 ≤ 6 个 `Test` 函数
- **昂贵的 setup 只在父测试准备一次**：起 HTTP 服务、装配服务容器、解压内嵌归档都属于这一类。
  每个子测试各起一套，加一条分支就在给总时长做乘法
- **装配类测试不真解压归档**：`runtime/runtimetest.SeedMarkers` 预置
  「已解压 + 版本标记」后，装配只跑配置 / DB / 服务 / 工具注册的真实链路。
  真解压只在 `runtime` 包做一次（唯一的慢点是刻意的）
- **慢链路用标准 `-short` 隔离**：解压归档加了 `testing.Short()` 守卫后，
  日常 `go test -short ./...` 秒级返回，只有真碰运行时归档时才跑全量。
  不要用自定义 tag 或环境变量另造一套开关
- 环境依赖（系统 shell、真实网络、运行时归档）用 `exec.LookPath` / `t.Skip` 守卫后跳过
- 每个测试文件**首行必须有导航注释**：写清楚覆盖哪条链路、坏了的表现是什么
- 多轮对话一律用 `backend/llm/llmtest` 脚本替身 + `factory.SetOverride` 注入，绝不真联网

### 3.3 测试索引

保留判据只有两条：**它坏了会指向一条真实链路**、**必须跨模块 / 跨轮次 / 跨协议**。
全量 **14 个 Test / 13 个文件 / 36 个子测试**（约 2400 行）。同族场景收在同一个 Test 的子测试里，
不按「一个断言一个测试」散开——散开只会在改动时逼你跑一大堆、失败时又定位不到是哪条链。

**改这里 → 只跑这条**（定位与验证都从这张表进）：

| 改动位置 | 命令 | 耗时 |
|---|---|---|
| `agent/`（循环、压缩、工具调度） | `go test -short -run 'TestLoopChain\|TestCompactProtocol' ./backend/agent` | ~1s |
| `service/`（会话、对话、审批、落库） | `go test -short -run TestServiceRunChain ./backend/service` | ~3s |
| `llm/*`（协议适配） | `go test -short ./backend/llm/...` | ~3s |
| `tool/`（文件护栏、执行、web） | `go test -short -run TestFilesGuardrails ./backend/tool` | ~1s |
| `server/`（SSE hub） | `go test -short -run TestHubDeliveryChain ./backend/server` | ~2s |
| `api/` + `server/`（端到端 HTTP + SSE） | `go test -short -run TestHTTPChain ./backend/api` | ~3s |
| `runtime/`（内置运行时） | `go test -short -run TestBundledRuntimeChain ./backend/runtime` | ~1s |
| `skill/` · `knowledge/` | `go test -short ./backend/skill ./backend/knowledge` | ~2s |
| 壳层（`app.go` / 托盘 / 单实例） | `go test -short -run 'TestAppLifecycle\|TestSecondLaunchHandoff' . ./backend/singleinstance` | ~1s |
| 拿不准 | `go test -short ./...` | ~9s |
| 提交前 / CI | `go test -count=1 ./...` + `go run ./tools/check-boundaries` | ~13s（含归档真实解压；冷缓存首次可达 1 分钟） |
| 并发相关改动 | `go test -race ./backend/...` | 分钟级 |

时间大头是 `runtime` 包对两份归档的真实解压（全量唯一的慢点，`-short` 跳过）。
提交前那两条是分开跑的：测试与门禁各自独立可见，失败时不互相掩盖。

`-race` 前置条件：`-race` 需要 cgo，而本项目为了让 SQLite 驱动保持纯 Go 默认关掉了 CGO。
跑这一档必须先装 MinGW（`winget install -e --id BrechtSanders.WinLibs.POSIX.UCRT`，
装完确认 `gcc` 在 PATH 里），再 `CGO_ENABLED=1`。没有 C 编译器时这一档会直接
`build failed`——不是代码问题，别当成回归去查。

**每条链覆盖什么**（括号里是子测试）：

| 文件 | Test | 覆盖的链路 |
|---|---|---|
| `app_test.go` | TestAppLifecycle | 启动等待判据（慢装配不误判 / 失败如实报 / 卡死有上限）；关闭去向（托盘开关 / 退出流程 / 托盘未就绪） |
| `agent/agent_test.go` | TestLoopChain | 顺序回填（串行多轮 + 并行批次）；工具失败带原因；四条收尾路径（取消 / 截断不执行工具且不重试 / 零产出截断 / 上游报错）；输出预算被拒时降级（取上游上限 + 到下限放弃）；重复调用拦到上限且事件成对；事件出口串行化 |
| | TestCompactProtocol | 清洗剔除空 assistant 与孤儿结果；拆散的并行声明合并；压缩不孤儿化结果且切在整轮边界（含预算边界与降级截断） |
| `api/api_test.go` | TestHTTPChain | 启动装配完整且端口握手时序正确；跨源预检放行 POST；真实 HTTP 栈 SSE 送达（seq 递增、data 回带 event）与 Last-Event-ID 补帧。服务只启动一次，子测试共用 |
| `server/sse_test.go` | TestHubDeliveryChain | 缓冲满时挤 delta 保 done；同类 delta 合流保序 |
| `service/chain_test.go` | TestServiceRunChain | 审批闭环（会话级放行）；落库链序（错误轮半成品 / 插话注入）；声明与结果紧邻配对（跨轮 / 同轮并发）；输出预算与水位口径；用量口径（归一落库 + 命中率封顶）；默认模型继承与存量回填。装配只做一次 |
| `service/skill_zip_test.go` | TestSkillZipImport | 技能包导入链：多技能解压落盘注册、条目穿越拒绝且不落盘、没有 SKILL.md 的包拒绝 |
| `tool/files_test.go` | TestFilesGuardrails | 路径护栏（穿越拒绝 + Unicode 变体找回）；覆盖前必读闭环；edit 唯一性、实际改动与行尾 BOM 保持 |
| `knowledge/knowledge_test.go` | TestKnowledgeChain | 建索引 → 检索（短查询子串兜底）→ 删除级联；pptx 按页号抽取且可检索 |
| `skill/skill_test.go` | TestSkillRegistryChain | embed 解析落盘与触发词渲染；同名按来源优先且可回退、切换工作目录换掉工作区技能 |
| `runtime/runtime_test.go` | TestBundledRuntimeChain | 归档 SHA 与常量一致、解压路径穿越拒绝；真实解压平铺到根目录（`-short` 跳过） |
| `singleinstance/singleinstance_test.go` | TestSecondLaunchHandoff | 二次启动转交：文件路径与空路径（只唤起窗口）都原样送达主实例 |
| `llm/openai/openai_test.go` | TestOpenAIAdapter | 输出上限字段按协议家族选择（含网关前缀）；流式解析（usage 帧晚到 + 缓存双口径 + 工具调用单次下发）；断流仍收尾 |
| `llm/anthropic/anthropic_test.go` | TestAnthropicAdapter | 无签名 thinking 不回传；缓冲满不丢事件且停滞经空闲看门狗收尾 |

---

## 4. 架构门禁

```bash
gofmt -l .                           # 必须无输出：Go 源码统一 LF（见 .gitattributes）
go vet ./...
go test -count=1 ./...               # 全量测试（日常用 -short 跳过归档解压，见 §3.3）
go run ./tools/check-boundaries      # 依赖方向门禁
cd frontend && npm run build         # 含 vue-tsc 类型检查
wails build                          # 产物 build/bin/WorkBaby.exe
```

`gofmt -l .` 要能当门禁用，行尾就必须统一：`.gitattributes` 里 `*.go text eol=lf`
（Windows 上 `core.autocrlf=true` 会把检出内容写成 CRLF，而 gofmt 把 CRLF 文件一律
报成「未格式化」）。§2.6 的注释行数上限同样可机械核对——

```bash
rg -U -t go -o "(^\s*//[^\n]*\r?\n){4,}"   # 必须无输出：没有 ≥4 行的连续注释块
```

`tools/check-boundaries` 用 `go list -json` 读编译器视角的真实 import 关系，
逐层比对 §2.2 的允许表，并额外检查能力域不回引上层。它不做文本 grep——注释、
字符串字面量与构建标签分支都会让 grep 得出错误结论。改动包结构或调整依赖后必须跑通。

文档与实现不一致时，**以本文为准**；发现实现跑偏就改实现，不要改本文迁就代码。

---

## 5. 工作流

- 最小改动：外科手术原则；改动前后跑测试
- 验证优先：修 Bug 先写复现测试
- YAGNI：禁止过早抽象、单实现接口
