# WorkBaby

> **能干活的个人 AI 助手** —— Windows 本地优先的桌面 Agent 客户端。
> 数据全部留在本机：SQLite 落库、文件落盘、密钥 AES-GCM 加密；HTTP 只绑定 127.0.0.1 随机端口。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 对话 | 多轮 ReAct Agent：流式输出、思考与正文分离、上下文自动压缩、断点幂等续跑、中途插话与自动续接 |
| 干活 | 50 个内置工具（20 个业务工具 + 30 个纯函数）：命令执行、文件读写、联网搜索、文档解析、知识库检索、记忆写入。危险操作走审批门 + 目录信任 + 命令白名单 |
| 记住 | 长期记忆（单一 MEMORY.md + FTS5 派生索引）：按输入召回，模型可主动用 `memory_write` 写入 |
| 私有资料 | 知识库 RAG：文档自动索引（FTS5 trigram），对话自动召回片段，`knowledge_search` 深度检索（支持 PDF/Word/Excel 前端预览） |
| 扩展 | MCP Server 接入外部工具；Skill 注入方法论与脚本；子 Agent 委派（上下文/预算/工具/正文四重隔离） |
| 目标模式 | 设定目标后逐轮推进，每轮按实据校验，未达成自动续跑 |
| 后台任务 | 异步任务中心（队列 + 2 worker + 状态机 + 跨重启可恢复） |
| 工作面板 | 右栏工作区文件树、文件变更快照（unified diff 与回滚）、辅助对话（审批卡住时只读）、任务中心 |

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 / 桌面壳 | Go 1.24 · Wails v2（WebView2 壳） |
| HTTP / 实时 | gin · SSE（业务 API + 事件流全部走 HTTP） |
| 存储 | GORM + SQLite（WAL + FTS5 trigram，pure-Go 无 CGO） |
| 配置 / 日志 / ID | Viper · log/slog（标准库）· ULID + UUID |
| Agent / LLM 适配 | 自研 agent 内核（ReAct 循环 + 护栏中间件链）· 自研协议适配（OpenAI / Anthropic / Ollama） |
| MCP | 自研 stdio 客户端（spec 2025-06-18） |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus + Pinia + Tailwind 4（设计令牌经 `themes.css` 落地） |
| 测试 | testing + testify |

完整锁定版本见 [AGENTS.md §1](AGENTS.md)。

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

## 模块速览

```text
main.go / app.go            入口装配（embed 前端、托盘、单实例）
internal/
  server/  api/  service/   HTTP 三层 + 业务编排
  repo/  domain/            持久层与聚合根（DO/DTO/REQ/VO/RESP 同居）
  agent/  llm/  tool/       Agent 内核、LLM 协议适配、工具系统
  skill/  mcp/              Skill 与 MCP stdio 客户端
  capability/               能力接入契约（三通道）
  memory/  rag/             长期记忆与知识库
  config/  event/  db/      配置 / 事件 / 存储
  bootstrap/  runtime/      装配根与运行时设施（仅内置 python）
  pkg/  tray/  singleinstance/  叶子工具 / 托盘 / 单实例保护
frontend/src/src/           Vue 3 工程（api / stores / components / chat）
assets/                     内置 Skill、图标、用户手册（embed）
docs/                       项目级文档（架构/规范/页面/契约/部署）
specs/             功能规格（按业务域分组）
```

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

## 设计语言

视觉与交互规范见 [DESIGN.md](DESIGN.md)。色值与尺寸只在 `frontend/src/src/themes.css` 定义；组件语法唯一实现在 `frontend/src/src/wb-ui.css`。一句话：**纯色 + 镂空 + 1px 中性描边**。

## 文档

| 文档 | 内容 |
|---|---|
| [`AGENTS.md`](AGENTS.md) | 工程规范（编码 / 分包 / 依赖方向 / 测试 / 门禁），编码前必读 |
| [`DESIGN.md`](DESIGN.md) | 视觉与交互设计规范 |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 模块拓扑、依赖方向、数据流、关键设计决策 |
| [`docs/API-CONTRACT.md`](docs/API-CONTRACT.md) | 端点 / 事件 / 错误码契约总表 |
| [`docs/PAGE-STRUCTURE.md`](docs/PAGE-STRUCTURE.md) | 前端页面信息架构 |
| [`docs/COMPONENT-GUIDELINES.md`](docs/COMPONENT-GUIDELINES.md) | 前端组件编写规范 |
| [`docs/PROJECT-SPEC.md`](docs/PROJECT-SPEC.md) | 产品定位、用户旅程、功能清单 |
| [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) | 开发流程、命令与测试体系 |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | 构建、打包与分发 |
| [`specs/`](specs/) | 功能规格（Agent 内核 / 工具 / 会话 / 能力 / 系统） |
| [`CHANGELOG.md`](CHANGELOG.md) | 版本变更摘要 |
| [`assets/docs/`](assets/docs/) | 应用内用户手册（`GET /api/v1/docs` 查看） |
