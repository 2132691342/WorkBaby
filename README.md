# WorkBaby

> **能干活的个人 AI 助手** —— Windows 本地优先的桌面 Agent 客户端。
> 数据全部留在本机：SQLite 落库、文件落盘、密钥 AES-GCM 加密，gin 只绑定 127.0.0.1 随机端口。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 对话 | 多轮 ReAct Agent：流式输出、思考与正文分离、上下文自动压缩、断点幂等续跑、中途插话与自动续接 |
| 干活 | 内置工具 40+：命令执行、文件读写、联网搜索、文档解析、知识库检索……危险操作走审批门 + 目录信任 + 命令白名单 |
| 记住 | 三层记忆（短期窗口 / 长期 MEMORY.md / 情景库）：自动形成、按输入召回，模型也可用 `memory_write` 主动记 |
| 私有资料 | 知识库 RAG：上传文档自动索引（FTS5），对话自动召回相关片段，`knowledge_search` 深度检索 |
| 流程 | 可视化 DAG 工作流（LLM / 工具 / 条件 / HTTP / 人工输入 / 通知节点），定时触发，对话里 `run_workflow` 直接调 |
| 扩展 | MCP Server 接入外部工具；Skill 注入方法论与脚本；子 Agent 委派（上下文/预算/工具三重隔离） |
| 陪伴 | 桌宠与聊天同形象同状态；邮件 / Webhook 通知通道；后台任务中心 |
| 工作面板 | 右栏 Git 面板三视图：变更（暂存区/工作区拆行 + 行内暂存/丢弃 + 单文件 diff + 提交）、历史（分页 + 提交详情 + 单次提交内文件 diff）、图谱（`log --graph` 只读）；分支切换/新建/删除与 ahead-behind |
| 仓库导读 | 扫描工作区生成确定性导读：语言统计、入口文件、目录树（每文件一句话摘要）、目录页与文件页 |
| 生命周期钩子 | run_start / before_tool / after_tool / run_end 触发用户命令子进程（stdin 收 JSON 载荷、stdout 出 JSON 决策），`before_tool` 的 deny 拦截工具调用 |

## 技术栈

**Go 1.25** · Wails v2（WebView2 壳）· gin · GORM + SQLite（WAL + FTS5）· Viper · slog · gorilla/websocket（原生 CDP）
**Vue 3 + TypeScript + Vite** · Element Plus · Pinia · Tailwind 4 设计 token

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
  api ─► service ─► 能力域（harness / llm / tool / capability /
                    memory / rag / workflow / mcp / pet …）─► repo ─► SQLite
```

- 业务全部走 HTTP（`{code,message,data}` 统一响应），前端可脱离壳独立调试；
- Agent 内核 `harness` 不依赖任何上层：LLM/工具/记忆/钩子全接口注入，ReAct 循环 + 可插拔钩子（LoopHooks）+ 检查点唯一出口；
- 能力接入走统一契约 `capability`：Preload（上下文注入）/ Tools（模型调用）/ Capture（run 后沉淀）三通道，新增能力注册一行即接入。

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
  server/ api/ service/ repo/ domain/   HTTP 四层 + 域模型
  harness/ llm/ tool/ skill/ mcp/       Agent 内核与能力
  capability/                           能力接入契约（三通道）
  memory/ rag/ workflow/ pet/           记忆/知识库/工作流/桌宠
  cron/ channel/ runtime/ pkg/ …        定时/通知/内置运行时/叶子工具
frontend/src/src/       Vue 3 工程（api / stores / components / chat）
assets/                 内置 Skill、图标、用户手册（/api/v1/docs）
doc/                    设计文档（18 篇）
```

## 文档

- 设计与实现：[`doc/README.md`](doc/README.md)（18 篇，架构 → 各模块 → API 契约 → 构建分发）；
- 工程规范：[`CLAUDE.md`](CLAUDE.md)（编码前必读，冲突以它为准）；
- 用户手册：[`assets/docs/`](assets/docs/)，应用内经 `GET /api/v1/docs` 查看；
- 架构门禁：`powershell -ExecutionPolicy Bypass -File scripts/check-boundaries.ps1`。
