# WorkBaby

> **能干活的个人 AI 助手** —— Windows 本地优先的桌面 Agent 客户端，给不懂命令行的办公新手用。
> 数据全部留在本机：SQLite 落库、文件落盘、密钥 AES-GCM 加密；HTTP 只绑定 127.0.0.1 随机端口。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 对话 | 流式输出、思考与正文分离、中途插话、上下文自动清洗压缩 |
| 干活 | 11 个内置工具：读 / 写 / 改文件，看目录、找文件、搜内容，跑 PowerShell、跑 Python，搜网页、开网页，查私有资料 |
| 安全 | 写文件 / 跑命令 / 跑脚本走审批卡：只这一次 / 本会话内都放行 / 拒绝 |
| 方法论 | Skill：把「怎么做一件事」写成 `SKILL.md`，需要时才展开正文 |
| 私有资料 | 知识库：本地文档（PDF / Word / Excel / Markdown / 文本）建索引，对话时按需检索 |
| 看得见 | 对话时实时显示上下文占用；仪表盘按天 / 按模型统计 token 用量 |
| Python | 内置便携式 CPython（可选），开箱即用，不用自己装环境 |

打开就用：装好 → 设置里填一个模型服务 → 回到聊天页说人话 → 它自己读文件、改文件、跑脚本、查资料。
需要贴文件时在输入框打 `@` 选文件；`/` 可以唤出常用命令。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 / 桌面壳 | Go 1.24 · Wails v2（Frameless，前端自绘标题栏） |
| HTTP / 实时 | gin · SSE（业务 API + 事件流全部走 HTTP） |
| 存储 | GORM + SQLite（WAL + FTS5 trigram，pure-Go 无 CGO） |
| 配置 / 日志 / ID | Viper · log/slog · ULID |
| Agent / LLM | 自研内核（单层流式循环）· 自研协议适配（OpenAI 兼容 / Anthropic / Ollama） |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + 原生 CSS 设计令牌（无 UI 框架） |
| 测试 | testing（全量 < 10s，LLM 用假实现注入） |

## 快速开始

```powershell
# 开发
go test ./internal/...        # 后端测试
cd frontend && npm install && npm run build
wails dev                     # 开发模式

# 发布
wails build                   # 产物 build/bin/WorkBaby.exe
```

## 目录

```
internal/
  agent/      内核：单层流式循环 + 工具调度 + 上下文压缩
  llm/        协议适配（openai / anthropic / ollama + retry + factory）
  tool/       工具注册表 + 11 个内置工具
  service/    业务编排（会话 / 对话 / 审批 / 配置 / 系统提示）
  server/     gin 路由 + SSE hub
  api/        业务 handler + Wails 系统能力绑定
  repo/       GORM 持久层
  domain/     域模型（一个聚合根一个文件）
  knowledge/  知识库（切分 + FTS5 检索）
  skill/      Skill 解析与注册
  runtime/    路径解析 + 内置 Python
  pkg/        叶子工具包（错误 / ID / 日志 / 加密 / 路径）
frontend/src/src/   Vue3 源码（themes.css 是唯一色值与字体来源）
docs/         架构 / 契约 / 开发 / 部署 / 页面
specs/        功能规格 01-14
```

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范（唯一权威） |
| [DESIGN.md](DESIGN.md) | 视觉语言与交互原则 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架构总览与关键取舍 |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | HTTP / SSE 契约与断线对账 |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | 开发环境、数据目录、排错 |
| [docs/PAGE-STRUCTURE.md](docs/PAGE-STRUCTURE.md) | 页面结构与组件基元 |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | 构建产物与分发 |
| [specs/](specs/) | 14 篇功能规格 |
| [specs/12-user-manual.md](specs/12-user-manual.md) | 用户手册（写给第一次用的人） |
