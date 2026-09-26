# docs/REFERENCE-PROJECTS.md · 参考项目调研与取舍

> 本工程不依赖下述任何项目（零代码引用）；本文记录调研结论与取舍边界，作为设计与维护的对照基线。
> 内核契约基线：PI `pi-agent-core`（基线 v0.85.0），对齐与偏离见 `AGENTS.md §2.13 / §9.7`。

## 1. pi（HuggingFace `pi-mono`，TypeScript）——内核形态来源

**是什么**：monorepo（`ai` / `agent` / `tui` / `coding-agent` / `chord` 等 11 包），`agent` 包即 `pi-agent-core`。

**已对齐的契约**（`internal/agent` 等价实现）：

| PI | 本工程 |
|---|---|
| 2 层 `runLoop`（inner 工具批 + outer 收尾缝） | `runLoop`（`agent/loop.go`） |
| `PendingMessageQueue` + `QueueMode` | `MessageQueue`（`FollowUpQueue` 为语义别名） |
| 三回调 `transformContext` / `convertToLlm` / `prepareNextTurn` | `Hooks.TransformContext` / `Hooks.ConvertToLlm` / `Loop.PrepareNextTurn` |
| `getApiKey` 每次调用前刷新 | `Loop.WithGetAPIKey` |
| `AgentTool.executionMode`（sequential / parallel） | `Tool.ExecutionMode()` |
| 错误编码为消息（不抛异常）+ 发送前修复 | 「报错但零产出」轮不算成功 + `RebuildHistory` 清洗 |

**未移植（有意）**：Session JSONL 分支树（fork/tree/resume——单会话 ReAct 暂不需要）、extensions 模块（等同插件系统，明令不做）、TUI、chord/protocol 多端附着、内置 8 工具生态（本工程工具面自定）。

## 2. go-micro（Go agent harness）——韧性与计量细节来源

**是什么**：经典微服务框架（registry/server/broker）之上的 agent harness，agent = 内嵌 LLM 的服务。

**取**：

| 借鉴点 | 落地 |
|---|---|
| 错误分类 + 稳定 Kind，只重试 timeout / rate_limited / unavailable，退避封顶 | `llm` 的错误分类与 `DefaultRetryPolicy` |
| 拒绝码结构化（`Refused` 字段直给模型可读文案） | `ToolResult.Refused` + `Meta["refused_reason"]` |
| 确定性记忆压缩（不调 LLM，可换钩子） | `MicroCompressor` 同思路：确定性折叠 |
| checkpoint 工具结果复用（步骤名 = tool+args，resume 不重放副作用） | `MapSteps` + `RepeatGuard`（恢复键复用） |
| 取消永远赢（每次尝试后检查 `ctx.Err()`） | 内核 run 循环的取消语义 |

**不取**：registry/client/broker 微服务设施（桌面单进程函数表即可）、A2A / delegate 联邦与结果缓存、x402 付费工具、MCP gateway（本工程要的是 MCP **client**）、假流式（按词切分最终回复）。

## 3. ERP-AGENT（Python / FastAPI + LangChain deepagents）——反面参照

**是什么**：采购助手，agent 循环整体外包给 deepagents/LangGraph，自研部分只有中间件钩子与流消费。（非 Go 项目，仅作设计参照。）

**取（思路）**：阶段状态进结构化 state 而非正则猜文本；声明式配置（YAML / docstring）降低样板。

**不取的教训**：无真实 token 预算（阈值常量是死配置，截断只在展示层）——本工程以 `MaxInput` + `MicroCompressor` 承担；存储/服务硬耦合云组件——本工程 SQLite + 进程内工具；鲁棒性近零（无重试 / 无断连取消）——本工程重试、检查点、审批闭环齐备。

## 4. PandaX（Go 企业级 web 全栈）——工程实践对照

**取（思路）**：api / service / entity 分层与接口化 Dao；统一响应包裹（本工程为 `{code,message,data}` + AppError 分段错误码）；启动期 AutoMigrate（单机 SQLite 场景）；信号驱动停机。

**不取**：租户体系、Casbin API 权限、组织机构数据权限、Redis / 多数据库 / TDengine、限流与验证码、代码生成器、消息队列与规则引擎——均为多租户服务端才需要的面，单机桌面应用一律剔除。

## 5. gotool（cnlesscode，Go 工具库）——评估不引入

**是什么**：纯工具库（gfs 文件操作 / gZip / request / random / mapCache / gDom 等，根包仅标准库依赖）。

**结论：不引入**。理由：

| 维度 | 事实 |
|---|---|
| 覆盖面 | gfs / gZip / request / random 与 `internal/pkg` 及标准库重叠（archive 工具自带限额，gotool 无） |
| 缺口 | 本项目必需的 GBK/UTF-16 归一化、AES-256-GCM、日志落盘轮转、HTTP 流式（SSE/LLM）它都没有 |
| 依赖面 | gintool / nlp / cloud 子包会拖入 gin、gorm、云 SDK（按包裁剪可避，但约束靠纪律不靠结构） |

其角色已由 `internal/pkg`（叶子工具包，AGENTS.md §2.1 铁律）承担。
