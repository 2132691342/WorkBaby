# WorkBaby

> **能干活的个人 AI 助手** —— Windows 本地优先的桌面 Agent 客户端。
> 数据全部留在本机：SQLite 落库、文件落盘、密钥 AES-GCM 加密，HTTP 只绑定 127.0.0.1 随机端口。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 对话 | 多轮 ReAct Agent：流式输出、思考与正文分离、上下文自动压缩、断点幂等续跑、中途插话与自动续接 |
| 干活 | 50 个内置工具：20 个业务工具（exec / 文件×6 / webfetch / websearch / http / doc / archive / delegate / memory_write / todo / 计划模式×2 / request_input / run_skill_script / knowledge_search）+ 30 个纯函数（functools）：命令执行、文件读写、联网搜索、文档解析、知识库检索。危险操作走审批门 + 目录信任 + 命令白名单 |
| 记住 | 长期记忆（单一 MEMORY.md + FTS5 派生索引）：按输入召回，模型可主动用 `memory_write` 写入 |
| 私有资料 | 知识库 RAG：文档自动索引（FTS5 trigram），对话自动召回片段，`knowledge_search` 深度检索（支持 PDF/Word/Excel 前端预览） |
| 扩展 | MCP Server 接入外部工具；Skill 注入方法论与脚本；子 Agent 委派（上下文/预算/工具/正文四重隔离） |
| 目标模式 | 设定目标后逐轮推进，每轮按实据校验，未达成自动续跑 |
| 后台任务 | 异步任务中心（队列 + 2 worker + 状态机 + 跨重启可恢复） |
| 工作面板 | 右栏工作区文件树、文件变更快照（unified diff 与回滚）、辅助对话（审批卡住时只读）、任务中心 |

## 技术栈

**Go 1.24+** · Wails v2（WebView2 壳）· gin · GORM + SQLite（WAL + FTS5）· Viper · slog · SSE
**Vue 3 + TypeScript + Vite** · Element Plus · Pinia · Tailwind 4 设计令牌（叠加自研 `wb-ui.css`）

## 架构：单进程双主机

```text
┌──────────────────────────────────────────────────────────────┐
│ Vue 3 前端（WebView2）                                         │
│  ├─ fetch       ──► http://127.0.0.1:{port}/api/v1/**         │
│  ├─ EventSource ──► /api/v1/events    (SSE 流式/推送/断线重放)  │
│  └─ Wails runtime ─► 系统能力：文件对话框 / 剪贴板 / 托盘 / 窗口  │
└──────────────────────────────────────────────────────────────┘
        │ gin（业务 API + 统一响应 + SSE）        ▲
        ▼                                        │ app:ready 注入端口
  api ─► service ─► 能力域（agent / llm / tool / skill / mcp /
                    capability / memory / rag …）─► repo ─► SQLite
```

- 业务全部走 HTTP（`{code,message,data}` 统一响应），前端可脱离壳独立调试
- Agent 内核 `internal/agent` 不依赖任何上层：LLM / 工具 / 记忆 / 审批全部接口注入，主循环只做「请求 → 执行 → 回填」，安全与持久化全在护栏中间件链与钩子里
- 能力接入走统一契约 `internal/capability`：Preload（上下文注入）/ Tools（模型调用）/ Capture（run 后沉淀）三通道，新增能力注册一行即接入

## 快速开始

```bash
git clone <repo> && cd WorkBaby
cd frontend && npm install && cd ..
go mod tidy
wails doctor     # 体检 Go / Node / WebView2 环境
wails dev        # 开发模式（前端 HMR + 后端热重启）
```

首次启动：设置 → 模型 → 添加 Provider（OpenAI 兼容 / Anthropic / Ollama 任一）→ 测试连通 → 回聊天页开聊。

发布构建：

```bash
wails build -nsis -ldflags "-s -w" -trimpath
```

## 目录速览

```text
main.go / app.go        入口装配（embed 前端、托盘、单实例）
internal/
  server/ api/ service/ repo/ domain/          HTTP 四层 + 域模型
  agent/ llm/ tool/ skill/ mcp/                Agent 内核与能力
  capability/                                  能力接入契约（三通道）
  memory/ rag/                                 记忆 / 知识库
  config/ event/ db/ bootstrap/ runtime/ pkg/  配置 / 事件 / 存储 / 装配 / 运行时 / 叶子工具
  tray/ singleinstance/                        托盘 / 单实例保护
frontend/src/src/       Vue 3 工程（api / stores / components / chat）
assets/                 内置 Skill、图标、用户手册
docs/                   项目级文档（架构 / 规范 / 流程 / 契约 / 部署）
specs/features/         功能规格（按业务域分组）
```

## 内核形态：PI 形态骨架

`internal/agent` 与 PI `pi-agent-core` 等价：

| PI 形态 | 本工程对应 |
|---|---|
| `Agent` 状态化包装（state + listeners + queues） | `Loop` 结构体 + `WithSink` / `WithHooks` / `WithSteeringQueue` / `WithFollowUpQueue` |
| `agent-loop` 两层纯函数循环 | `runLoop`（outer 等 follow-up 收尾缝，inner 做工具批 + steering） |
| `beforeToolCall` / `afterToolCall` Hook | 护栏中间件链 `Middleware` + `Executor(ExecOptions)` |
| `transformContext` / `convertToLlm` | `Hooks.TransformContext` / `Hooks.ConvertToLlm` |
| `getSteeringMessages` / `getFollowUpMessages` | `SteeringQueue` / `FollowUpQueue`（默认 one-at-a-time） |
| `shouldStopAfterTurn` | `Hooks.ShouldStop` |
| `prepareNextTurnWithContext` | `Loop.PrepareNextTurn`（NextTurnUpdate：切模型 / 思考档 / 注入消息 / 压缩信息） |
| `AgentEvent` 10 类 | 14 个 `EventKind`：RunStart / TurnStart / TurnDelta / TurnThinking / TurnEnd / ToolCall / ToolStart / ToolResult / Checkpoint / Compressed / Error / RunDone + Retry / QueueDrained |
| `Checkpoint` 续跑 | `Checkpoint` + `CheckpointStore`（SQLite 实现） |
| 拒绝 = 回执（`Refused`） | `MetaTerminate` + 摘要错误回执模型 |

是否引入 PI 的 Session 树（JSONL 分支 / fork / tree resume）与实验性 harness 抽象，取决于何时出现多分支会话树类需求。当前形态已能支撑单会话 ReAct；引入新抽象属「包装型重构」零行为变化，待需求驱动。

## 文档

| 文档 | 内容 |
|---|---|
| [`AGENTS.md`](AGENTS.md) | 工程规范（编码 / 分包 / 依赖方向 / 测试 / 门禁），编码前必读 |
| [`DESIGN.md`](DESIGN.md) | 视觉风格、布局与交互设计规范 |
| [`docs/PROJECT-SPEC.md`](docs/PROJECT-SPEC.md) | 项目定位、产品目标与功能范围 |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 架构、目录结构与数据组织 |
| [`docs/API-CONTRACT.md`](docs/API-CONTRACT.md) | 端点 / 事件 / 错误码契约总表 |
| [`docs/COMPONENT-GUIDELINES.md`](docs/COMPONENT-GUIDELINES.md) | 前端组件、样式与依赖规范 |
| [`docs/PAGE-STRUCTURE.md`](docs/PAGE-STRUCTURE.md) | 页面与视图结构 |
| [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) | 开发流程、命令与测试体系 |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | 构建、打包与分发 |
| [`docs/REFERENCE-PROJECTS.md`](docs/REFERENCE-PROJECTS.md) | 参考项目调研与取舍（pi / go-micro / PandaX / gotool 等） |
| [`specs/features/`](specs/features/) | 功能规格（Agent 内核 / 工具 / 会话 / 能力 / 系统） |
| [`TODO.md`](TODO.md) · [`CHANGELOG.md`](CHANGELOG.md) | 进度与版本记录 |
| [`assets/docs/`](assets/docs/) | 应用内用户手册（`GET /api/v1/docs` 查看） |
