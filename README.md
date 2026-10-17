# WorkBaby

> **能干活的个人 AI 助手** —— Windows 本地优先的桌面 Agent 客户端，给不懂命令行的办公新手用。
> 数据全部留在本机：会话文件与 SQLite 落盘、密钥 AES-GCM 加密；HTTP 只绑定 127.0.0.1 随机端口。
>
> 内核不重写：Agent 循环、工具、压缩、重试、MCP、扩展全部由内置的 [PI](https://pi.dev) 负责。
> WorkBaby 做的是让你看见并控制它怎么跑。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 对话 | 流式输出、思考与正文分离、中途插话与排队、上下文自动压缩 |
| 干活 | PI 自带的工具：读 / 写 / 改文件，看目录、找文件、搜内容，跑命令、跑脚本，搜网页、开网页；另加知识库检索与记忆写入 |
| 安全 | 写文件 / 跑命令 / 跑脚本走审批卡：只这一次 / 本会话内都放行 / 拒绝；另有「只读」模式直接拒绕 |
| 方法论 | Skill：把「怎么做一件事」写成 `SKILL.md`，需要时才展开正文 |
| 私有资料 | 知识库：本地文档（PDF / Word / Excel / Markdown / 文本）建索引，通过 MCP 工具按需检索 |
| 记得住 | 记忆：跨对话保留你的习惯与约定；界面上写、或对话里对助手说「记住…」，之后每轮对话自动带上 |
| 外部工具 | MCP：把本地 MCP server 配进来，由 PI 自己连接并在对话里使用 |
| 分头干活 | 子代理：助手把「读一堆东西只要一个结论」的活派到独立进程去跑，结论回填进当前对话 |
| 干完能复盘 | 每次 run 一行账：花了多少 token、第几轮涨到多少上下文、每个工具动作耗时与成败 |
| 走错能回头 | 检查点：从任意一轮分叉成一条新会话，原对话完整保留 |
| 评估回归 | 一次对话的客观指标 + 主观分；存基线，退步了一眼看见 |
| 历史接着聊 | 在 Claude Code / Codex 里聊过的天，这里只读翻看，可搬到 WorkBaby 接着聊 |
| 项目架构 | 本地静态分析模块与依赖，画成分层架构图（不联网、不调模型） |
| 改了什么看得见 | 工作区：助手在你的文件夹里改了哪些文件、每一处改了什么，逐行对账（只读，不替你提交） |
| 看得见 | 对话时实时显示上下文占用；仪表盘按天 / 按模型统计 token 用量与缓存命中 |
| 看得懂 | 应用内「帮助」页：内置手册，离线可查 |
| 开箱即跑 | Node / Python 已嵌进程序，首次启动自动解开；PI 也由它装好 |
| 顺手 | 两套主题、五档字体、四档字号、自定义背景；关闭收进托盘、开机自启、双击 md/txt 直接唤起 |

打开就用：装好 → 跟着向导走完三步（运行环境 / 助手引擎 / 模型服务）
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
| 开箱即用 | Node / Python 归档 `go:embed` 进 exe，首启解压 | 新手装上即用，不碰环境变量 | exe 约 90MB |
| 界面自绘 | 无 UI 组件库，控件全部自绘 + 语义 CSS 令牌 | 视觉与主题完全可控 | 控件生命周期自己维护 |

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 / 桌面壳 | Go 1.24 · Wails v2（Frameless，前端自绘标题栏） |
| HTTP / 实时 | gin · SSE（业务 API + 事件流全部走 HTTP） |
| 存储 | GORM + SQLite（WAL + FTS5 trigram，pure-Go 无 CGO） |
| 配置 / 日志 / ID | Viper · log/slog · ULID |
| Agent / LLM | 外部内核 PI（`pi --mode rpc`）· 模型与服务商由 PI 自己的 `models.json` 管 |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + 原生 CSS 设计令牌（无 UI 框架） |
| 测试 | testing（12 个包各一个入口 + 应用生命周期一条，全部是跨模块 / 跨协议链路；`pwsh scripts/check.ps1` 分四档收口验证，LLM 用脚本替身注入，不联网） |

## 快速开始

```powershell
# 内置运行时归档（Node 24.21.0 + Python 3.12.8）随仓库托管（Git LFS）：
# 克隆后先 git lfs pull 把真文件拉下来，否则构建会以内嵌 LFS 指针报错。

# 日常验证（一个脚本收口）
pwsh scripts/check.ps1                  # 快门：编译 → 依赖门禁 → 短测试（约 10 秒）
pwsh scripts/check.ps1 -All             # 再加全量测试与前端构建
pwsh scripts/check.ps1 -Full            # 提交前：再加静态检查与前端类型检查
pwsh scripts/check.ps1 -Pkg ./backend/mcp     # 只跑一个包

# 开发
cd frontend && npm install && npm run build
wails dev                         # 开发模式

# 发布
wails build                       # 产物 build/bin/WorkBaby.exe
wails build -nsis                 # 另出 Windows 安装器 build/bin/WorkBaby-amd64-installer.exe
```

## 目录

```
backend/
  pi/         PI 子进程监督：进程生命周期 / RPC 协议编解码 / 事件翻译（不依赖 IO 以外的任何东西）
    env/      环境引导：内置 Node、PI 的发现与安装、models.json / settings.json / mcp.json 互通
  service/    业务编排（会话 / 对话 / 审批桥 / 模型服务 / 技能 / 工作区文件 / 设置 / 用量）
  server/     gin 路由 + SSE hub（合流 / 重放 / 慢客户端）
  api/        启动装配 + 业务 handler + 对话框 / 自启等系统能力实现
  repo/       GORM 持久层
  domain/     域模型（一个聚合根一个文件，DO / REQ / VO / RESP 同居）
  knowledge/  知识库（解析 + 切分 + FTS5 检索；另挂一个 MCP server 出口）
  git/        工作区版本视图（只读 status / diff，走系统 git）
  skill/      Skill 解析、注册与来源优先级
  history/    通用会话历史（只读扫描 + 按来源解析）
  archmap/    项目架构分析（import 解析 + 分层图数据）
  runtime/    路径解析 + 内置 Node / Python 运行时（归档 go:embed，首启解压）
              runtimetest/  测试夹具：预置运行时标记，装配类测试跳过解压
  config/     Viper 配置（MasterKey 等）
  db/         SQLite 打开 / 迁移 / FTS5 虚表
  pkg/        叶子工具包（错误 / ID / 日志 / 加密 / 路径）
  tray/       系统托盘
  singleinstance/  单实例与二次启动转交
frontend/src/   Vue3 源码（themes.css 是唯一色值与字体来源）
assets/       内置 Skill + 应用内帮助文档（embed 进 exe）
build/windows/ 平台资源与 NSIS 安装器脚本（installer/project.nsi 是可持久定制的那份）
scripts/      check.ps1：统一验证入口（快门 / -All / -Full / -Pkg）
tools/        依赖方向门禁（Go 写的独立程序）
docs/         项目级文档（架构 / 契约 / 数据模型 / 页面 / 开发 / 部署 / 错误码）
specs/        子系统规格 01-27（01-05 / 10 已随内核替换下线）
```

运行观测不单独设包：`service/runs.go` + `repo/run.go` + `domain/run.go`。
**内存真的不在本机库里**：会话正文是 PI 的 JSONL，WorkBaby 只读投影。

## 测试：改哪里就跑哪一条

一包一个入口（`TestXxxChain`），内部按链路分段。每条都对着一个**用户可见的故障**写，不是为凑覆盖率。

```powershell
pwsh scripts/check.ps1                       # 快门：编译 + 门禁 + 全部包的短测试（约 10 秒）
pwsh scripts/check.ps1 -Pkg ./backend/service # 只跑一个包（改小功能用这个）
pwsh scripts/check.ps1 -All                  # 加全量测试与前端构建
pwsh scripts/check.ps1 -Full                 # 提交前：加静态检查与前端类型检查
go test ./backend/service/ -run TestServiceChain -v   # 只跑一条链
```

| 改了哪里 | 跑这条 | 断言什么 |
|---|---|---|
| PI 子进程 / 协议 / 翻译 | `backend/pi` `TestPiChain` | 按 LF 分帧（U+2028 不截断）、未知记录忽略、命令按 id 关联、`done` 只在 `agent_settled`、崩溃报错不自动重发、一流核心一进程 |
| 环境引导 / 配置互通 | `backend/pi/env` `TestPiEnvChain` | 闸门阻塞项、PATH 前置注入、`models.json` 形状、`defaultTools` 纯名字、`mcp.json` 未知字段保留 |
| 对话编排 / 审批桥 / 投影 / 用量 | `backend/service` `TestServiceChain` | JSONL 投影（只显示文件里有的）、会话懒创建与删除、策略文件、审批桥、子代理独立进程、配置同步幂等 |
| HTTP 面 / SSE | `backend/api` `TestHTTPChain` | 装配完整与端口握手、跨源预检、环境闸门、SSE 送达与断线重放、运行记录与用量落账 |
| 知识库 | `backend/knowledge` `TestKnowledgeChain` | 加文档→索引→检索→删除级联、Office 页序、重建索引后台任务 |
| Skill | `backend/skill` `TestSkillRegistryChain` | embed 解析落盘渲染、来源优先级与工作区切换 |
| 工作区版本视图 | `backend/git` `TestGitChain` | 真 git 仓库的 status/diff 全链路（行号归属 / 中文路径还原）、非仓库给人话引导 |
| 内置运行时 | `backend/runtime` `TestBundledRuntimeChain` | 内嵌归档与 SHA 一致、解压路径穿越防护、Node 剔层与其余平铺 |
| 桌面壳启动 / 托盘 | `app_test.go` `TestAppChain` | 慢装配不误判、失败如实报、卡死有上限、关闭去向 |

## 文档

读法：`README.md`（这个项目是什么）→ `docs/ARCHITECTURE.md`（全貌与每个模块的设计）→
`specs/06-27`（某个子系统怎么实现）→ `AGENTS.md`（动手改代码时的规范）。
只看设计不看规范的，读到 `ARCHITECTURE.md` 就够了。

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范（编码 / 依赖方向 / 错误码 / 测试判据），唯一权威 |
| [DESIGN.md](DESIGN.md) | 视觉语言、设计令牌、交互原则 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 全貌：分层、依赖方向、每个模块的设计 / 实现 / 为什么 / 代价 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 域模型与持久化：两处存储的分工、JSONL 投影、迁移与 FTS5 |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | HTTP 端点、SSE 事件字典与断线对账五条规则 |
| [docs/PAGE-STRUCTURE.md](docs/PAGE-STRUCTURE.md) | 路由、页面骨架、组件基元、设置键位 |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | 开发环境、布局纪律、数据目录、测试写法、排错 |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | 构建产物、内置运行时、分发注意 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 错误码 1000–9799 的含义与处置动作 |
| [specs/12 用户手册](specs/12-user-manual.md) | 应用内帮助页：内置文档组织、`/docs` 接口与阅读界面 |

子系统规格 `specs/06-27`：
[06 内置运行时](specs/06-runtime.md) ·
[07 会话模型](specs/07-session.md) ·
[08 Skill 系统](specs/08-skill.md) ·
[09 知识库](specs/09-knowledge.md) ·
[11 桌面壳](specs/11-desktop-shell.md) ·
[12 用户手册](specs/12-user-manual.md) ·
[13 系统提示与设置](specs/13-prompt-and-settings.md) ·
[14 用量与上下文水位](specs/14-usage-and-context.md) ·
[15 工作区版本视图](specs/15-workspace-git.md) ·
[16 MCP 外部工具](specs/16-mcp.md) ·
[19 评估与回归](specs/19-eval-regression.md) ·
[20 通用会话历史](specs/20-universal-history.md) ·
[21 项目架构图](specs/21-arch-map.md) ·
[22 记忆](specs/22-memory.md) ·
[23 运行观测与检查点](specs/23-runs-and-checkpoints.md) ·
[24 子代理](specs/24-subagent.md) ·
[25 PI 监督进程](specs/25-pi-supervisor.md) ·
[26 环境引导](specs/26-pi-bootstrap.md) ·
[27 审批与 WorkBaby 扩展](specs/27-approval-bridge.md)
