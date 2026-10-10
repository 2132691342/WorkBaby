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
| 看得见 | 对话时实时显示上下文占用；仪表盘按天 / 按模型统计 token 用量与缓存命中 |
| 看得懂 | 应用内「帮助」页：五篇内置手册（上手 / 配模型 / 工具与安全 / 知识库与技能 / 常见问题），离线可查 |
| 开箱即跑 | Python 与 PowerShell 7 已嵌进程序，首次启动自动解开，不用自己装环境 |

打开就用：装好 → 左侧导航栏「设置」里填一个模型服务（没配好时欢迎页有直达按钮与教程）
→ 回到聊天页说人话 → 它自己读文件、改文件、跑脚本、查资料。
需要贴文件时在输入框打 `@` 从工作区文件面板里选；打 `/` 唤出常用命令与技能，选一个技能它就按那套方法干活。

## 为什么这样设计（取舍）

| 取向 | 选择 | 换来什么 | 代价 |
|---|---|---|---|
| 本地优先 | SQLite + 文件 + 加密密钥全在本机，HTTP 只绑 127.0.0.1 随机端口 | 数据不出机器，无账号无云服务 | 单进程单用户，不能协同 |
| 单一通信面 | 业务 API 与事件流全走 HTTP（gin + SSE），Wails 绑定只留系统能力 | 前端在普通浏览器里就能跑通全界面 | SSE 要自建断连、重放、对账 |
| 透明度优先 | 工具每步一张卡、上下文水位实时可见、用量按天归因 | 用户知道它在干什么、花了多少 | 界面元素比纯聊天多 |
| 可控优先 | 写文件 / 跑命令 / 跑脚本走审批卡；重复调用第 3 次强判失败 | 不可逆操作有闸门，模型打转能自停 | 每步 ask 会打断节奏（可设自动放行） |
| 确定性优先 | 上下文压缩是确定性裁剪 + 按 token 预算找切点，不调 LLM | 压缩可预测、可重放、零额外成本 | 裁掉的早期细节不进模型（历史仍在库里） |
| 开箱即用 | Python / PowerShell 归档 `go:embed` 进 exe，首启解压 | 新手装上即用，不碰环境变量 | exe 约 130MB+ |
| 界面自绘 | 无 UI 组件库，控件全部自绘 + 语义 CSS 令牌 | 视觉与主题完全可控 | 控件生命周期自己维护 |

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 / 桌面壳 | Go 1.24 · Wails v2（Frameless，前端自绘标题栏） |
| HTTP / 实时 | gin · SSE（业务 API + 事件流全部走 HTTP） |
| 存储 | GORM + SQLite（WAL + FTS5 trigram，pure-Go 无 CGO） |
| 配置 / 日志 / ID | Viper · log/slog · ULID |
| Agent / LLM | 自研内核（单层流式循环）· 自研协议适配（OpenAI 兼容 / Anthropic / Ollama） |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + 原生 CSS 设计令牌（无 UI 框架） |
| 测试 | testing（14 个 Test / 32 个子测试全为跨模块链路；日常 `go test -short ./...`、定点 `go test -short -run <名字> ./<包>`；LLM 用脚本替身注入，不联网） |

## 快速开始

```powershell
# 内置运行时归档（Python 3.12.8 + PowerShell 7）随仓库托管（Git LFS）：
# 克隆后先 git lfs pull 把真文件拉下来，否则构建会以内嵌 LFS 指针报错。

# 开发
go test -short ./...          # 后端链路测试（去掉 -short 为全量，含归档真实解压）
go run ./tools/check-boundaries   # 依赖方向门禁（改包结构后必跑）
cd frontend && npm install && npm run build
wails dev                     # 开发模式

# 发布
wails build                   # 产物 build/bin/WorkBaby.exe
wails build -nsis             # 另出 Windows 安装器 build/bin/WorkBaby-amd64-installer.exe
```

## 目录

```
backend/
  agent/      内核：单层流式循环 + 工具调度 + 上下文压缩（不依赖 IO 与桌面壳）
  llm/        协议适配（openai / anthropic / ollama + retry + factory + 假实现）
  tool/       工具注册表 + 11 个内置工具的执行与安全护栏
  service/    业务编排（会话 / 对话 / 审批 / 模型服务 / 技能 / 工作区文件 / 设置 / 用量）
  server/     gin 路由 + SSE hub（合流 / 重放 / 慢客户端）
  api/        启动装配 + 业务 handler + 对话框 / 自启等系统能力实现
  repo/       GORM 持久层
  domain/     域模型（一个聚合根一个文件，DO / REQ / VO / RESP 同居）
  knowledge/  知识库（解析 + 切分 + FTS5 检索）
  skill/      Skill 解析、注册与来源优先级
  runtime/    路径解析 + 内置 Python / PowerShell 运行时（归档 go:embed，首启解压）
              runtimetest/  测试夹具：预置运行时标记，装配类测试跳过解压
  config/     Viper 配置（MasterKey 等）
  db/         SQLite 打开 / 迁移 / FTS5 虚表
  pkg/        叶子工具包（错误 / ID / 日志 / 加密 / 路径）
  tray/       系统托盘        singleinstance/  单实例与二次启动转交
frontend/src/src/   Vue3 源码（themes.css 是唯一色值与字体来源）
assets/       内置 Skill + 应用内帮助文档（embed 进 exe）
build/windows/ 平台资源与 NSIS 安装器脚本（installer/project.nsi 是可持久定制的那份）
tools/        依赖方向门禁（Go 写的独立程序）
docs/         项目级文档（架构 / 契约 / 数据模型 / 页面 / 开发 / 部署）
specs/        子系统规格 01-14
```

## 文档

先读 `AGENTS.md`（规范）→ `docs/ARCHITECTURE.md`（全貌）→ 按需查 `specs/`。

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范（编码 / 依赖方向 / 错误码 / 测试判据），唯一权威 |
| [DESIGN.md](DESIGN.md) | 视觉语言、设计令牌、交互原则 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 全貌：分层、依赖方向、每个模块的设计 / 实现 / 为什么 / 代价、可靠性对策、已知限制 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 域模型与持久化：表、树状条目链、迁移与 FTS5 |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | HTTP 端点、SSE 事件字典与断线对账五条规则 |
| [docs/PAGE-STRUCTURE.md](docs/PAGE-STRUCTURE.md) | 路由、页面骨架、组件基元、设置键位 |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | 开发环境、布局纪律、数据目录、测试写法、排错 |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | 构建产物、内置运行时、分发注意 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 错误码 1000–8999 的含义与处置动作 |
| [specs/12 用户手册](specs/12-user-manual.md) | 应用内帮助页：内置文档组织、`/docs` 接口与阅读界面 |

子系统规格 `specs/01-14`：
[01 Agent 内核](specs/01-agent-loop.md) ·
[02 上下文压缩](specs/02-context-compaction.md) ·
[03 工具系统](specs/03-tool-system.md) ·
[04 文件工具](specs/04-file-tools.md) ·
[05 命令执行策略](specs/05-exec-policy.md) ·
[06 内置运行时](specs/06-runtime.md) ·
[07 会话模型](specs/07-session.md) ·
[08 Skill 系统](specs/08-skill.md) ·
[09 知识库](specs/09-knowledge.md) ·
[10 LLM 适配层](specs/10-llm-adapter.md) ·
[11 桌面壳](specs/11-desktop-shell.md) ·
[12 用户手册](specs/12-user-manual.md) ·
[13 系统提示与设置](specs/13-prompt-and-settings.md) ·
[14 用量与上下文水位](specs/14-usage-and-context.md)
