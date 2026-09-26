# docs/DEVELOPMENT.md · 开发流程与测试体系

## 1. 常用命令

| 目的 | 命令 |
|---|---|
| 开发运行 | `wails dev`（前端 HMR + 后端热重启） |
| 全量编译 | `go build ./...` |
| 后端全量测试 | `go test ./internal/...` |
| 单包 / 单测试 | `go test ./internal/service -run TestChatApproval -v` |
| 静态检查 | `go vet ./internal/...` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 前端测试 | `cd frontend && npm test` |
| 前端构建 | `cd frontend && npm run build` |

## 2. 门禁脚本

改完代码跑一遍即可发现越界与漂移：

| 脚本 | 检查什么 |
|---|---|
| `scripts/check-boundaries.ps1` | 分层依赖 9 项（`pkg` 不依赖 internal、`domain`/`repo` 不反向、`api` 不直接 import repo、`service` 不含 HTTP、`agent` 不依赖上层也不含 HTTP）+ 前后端契约版本号一致 |
| `scripts/check-contract.ps1` | 路由约定（cancel 与 resume 必须分路径）+ 前端 `client.ts` 路径白名单与后端 `routes.go` 注册前缀一致 |
| `scripts/i18n-sync.mjs` | zh / en 字典键集合与顺序一致；`dead` 子命令找未被引用的死键 |
| `scripts/copy-runtimes.ps1` | 构建后钩子：内置运行时归档必须齐全 |

`scripts/test.ps1` 是分层测试入口：`-Pkg service`、`-Run TestXxx`、`-NoFront`、`-Full`。
`-Full` 跑全部 Go 包 + 分层门禁 + i18n 校验 + 前端类型检查与测试。

## 3. 测试体系

### 3.1 定位

测试只守护**跨模块 / 跨轮次 / 跨协议**的行为：一个测试失败，必须意味着某条真实链路坏了。

保留判据与删除清单见 [`AGENTS.md §3`](../AGENTS.md)。规模约束：单文件 ≤ 6 个 `Test`；全量 10 秒内跑完。

### 3.2 分层与职责

| 包 | 测什么 |
|---|---|
| `internal/agent` | ReAct 多轮循环、护栏链裁决、压缩不拆散 assistant+tool 对、历史清洗 |
| `internal/service` | 编排链路：装配接线、事件映射、审批与跨重启续跑、检查点、集成编排（文件变更/反幻觉） |
| `internal/llm` | 跨 Provider 归一化契约（错误分类、重试退避）+ 三家线协议流解析 |
| `internal/tool` | 工具落点与沙箱、写前必读护栏、计划模式硬拦、exec 解析 |
| `internal/server` | SSE 可靠性：帧格式、断线重放、缓冲 gap、会话级订阅隔离、慢客户端 |
| `internal/mcp` | 子进程协议：工具结果回填、死进程归一为可辨识错误、启停与注册表同步 |
| `internal/rag` `memory` `runtime` | 检索算法（分词/兜底/重建替换）、解压安全护栏 |
| `frontend/src/src/chat/__tests__/` | 流事件管线、消息对账、块分组 |

完整测试索引（哪个文件覆盖什么）见 [`AGENTS.md §3.3`](../AGENTS.md)。

### 3.3 按改动面选测试

| 改动面 | 跑这个 |
|---|---|
| 内核循环 / 护栏 | `go test ./internal/agent/` |
| 聊天编排 / 审批 / 恢复 | `go test ./internal/service/ -run 'TestChat\|TestApproval\|TestDurable\|TestCheckpoint'` |
| 事件契约 | `go test ./internal/service/ -run 'TestCoreEventMapper\|TestEmitterContract'` |
| SSE | `go test ./internal/server/` |
| 模型接入 | `go test ./internal/llm/...` |
| 工具沙箱 | `go test ./internal/tool/...` |
| 前端渲染 / 流管线 | `cd frontend && npm test` |

### 3.4 关键设计

**按改动面选测试**：先看要改的能力落在哪个包（见上表），再决定跑哪个目标；按需 `-run TestXxx` 定位单个用例。

**真实依赖优先，只在必要处用替身**

| 依赖 | 选择 | 原因 |
|---|---|---|
| 数据库 | 真实 SQLite（内存模式 + 真实迁移） | 迁移与 FTS5 是高频故障源 |
| LLM | `stubProvider`（按调用序返回预设响应） | 跨进程且不可控 |
| MCP | 真实子进程 fixture（测试二进制 re-exec 成 server） | 要测的就是进程协议 |

**守卫不稳定的外部依赖**：依赖系统 shell / 真实网络的用例先 `exec.LookPath` 或 `testing.Short()` 判断后跳过，
不得让整个包变红。

## 4. 改动规范

分层、错误码、命名、注释与文档要求见 [`AGENTS.md`](../AGENTS.md)。典型改动路径：

| 改动 | 步骤 |
|---|---|
| 新增后端端点 | `internal/server/routes.go` 注册 + `internal/api/`（`api_chat.go` / `api_provider.go` / `api_handlers.go` 按域归入）透传 + 业务进 `service`；同步 `docs/API-CONTRACT.md` 与前端 `client.ts` 白名单 |
| 新增表 | `domain` DO → `repo` → 登记 `db/migrate.go` → 更新 `docs/ARCHITECTURE.md` 表清单 |
| 新增工具 | 实现 `tool.Tool` → 在 `api.Handler.Startup` 注册 → 更新 `specs/05-08 (tools)/` |
| 新增前端文案 | zh / en 字典同步加键，跑 `i18n-sync.mjs check` |
| 新增前端组件 | 遵守 `docs/COMPONENT-GUIDELINES.md`；外观语法回填 `wb-ui.css` |
| 文档 | 与代码同时更新；只描述现状，不记录改动过程 |

## 5. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 门禁脚本作为提交前必经 | 架构约束不靠自觉，破坏即失败 | 改动契约时需同步更新脚本与前端路径白名单 |
| 测试只覆盖复杂链路 | 全量 10 秒内跑完，改功能验证快 | 简单函数的回归靠编译期与集成测试兜底 |
| 文档与代码同 PR 更新 | 文档始终反映现状 | 每次改动都要评估文档影响面 |
| 单文件 ≤ 6 个 `Test` | 改动时定位快；避免单文件膨胀 | 同类场景需用 table-driven 合并 |
| 真实 SQLite / 真实子进程 | 测的是真实行为而非 mock 行为 | 单测略慢；子进程用例需 fixture 基础设施 |
