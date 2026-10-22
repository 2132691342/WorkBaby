# WorkBaby

**PI 的桌面工作台。** 它不自己实现 AI——它做一件更实际的事：把只会在终端里跑的
coding agent 引擎，变成一个能看、能点、能管的工作台。

```
PI 负责运行 Agent，WorkBaby 负责让你看见并控制它是怎么运行的。
```

## 你会得到什么

| | 能力 |
|---|---|
| 对话 | 流式正文、思考段、工具卡（参数 / 输出 / 耗时 / 就地看改动）按**原始顺序**内联；插话、排队、停止、自动重试提示 |
| 命令 | 输入 `/` 开菜单：内置动作（压缩上下文 / 导出 / 复制 / 重载 / 换模型 …）+ 引擎报上来的扩展命令与提示模板 + 技能 |
| 会话 | 读 PI 的会话目录：终端里聊过的这里也看得到，点开接着聊 |
| 提问 | PI 的扩展要确认 / 单选 / 填空时，界面出卡，回答写回内核 |
| 模型 | 读写 PI 的 `models.json`：这里加的服务商，`pi` 命令立刻能用 |
| 技能 | 列出 PI 真正会加载的技能（用户目录 / 工作区 / 插件包），启停靠移目录 |
| 插件 | 包走 `pi install/remove`，扩展走 `extensions/` + 登记表；安装进度流式打到面板 |
| 工具 | 清单来自 PI 写的工具声明（内置 / 插件 / MCP），开关写 `defaultTools` |
| MCP | 配置原文编辑 + 在线状态与工具清单，支持 OAuth 登录 |
| 工作区 | 改动 / 提交 / 历史 / 分支：助手动了什么，在这里对账并收成一次提交；对话里也能直接看单个文件的改动 |
| 仪表盘 | token 与估算花费（按天 / 模型 / 会话），全部来自会话文件 |
| 环境 | 复用系统已有的 Node / Python / PI；缺了才用内置运行时兜底 |

「能看见」的落点是工具卡、提问卡、工作区差异与用量仪表盘；
「能控制」的落点是每一个落盘配置的开关与 PI 自己的命令行。

## 它不做什么

- 不重写 Agent 内核：工具、压缩、重试、缓存、编辑、MCP、扩展、子代理全由 PI 演进；
- 不另装一道审批闸门、不改 PI 的默认行为（默认不打断是 PI 的设计，要闸门就装一个扩展）；
- 不再存一份会话与用量：那是两本账，两本账迟早分叉。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.24（单 module） |
| 桌面壳 | Wails v2（Frameless，前端自绘标题栏） |
| 业务 API | gin（回环 + 随机端口，只允许 GET / POST） |
| 实时通信 | SSE（`/api/v1/events`） |
| 存储 | SQLite（WAL，pure-Go 无 CGO）：**只存界面设置 KV** |
| 配置 | Viper（工作目录等固定项） |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + Vue Router（hash） |
| UI | Element Plus（基元）+ UnoCSS（原子类，刻度接 `--wb-*` 令牌） |
| 样式 | CSS 设计令牌：`themes.css` + `ep-theme.css` + `base.css` + `content.css` |
| 图表 | ECharts（按需注册，色值取 `--wb-ch-*`） |
| 渲染 | marked + DOMPurify + highlight.js（唯一出口 `utils/md.ts`） |
| 编辑 | CodeMirror 6（markdown / json） |
| 国际化 | Vue I18n（zh-CN 默认 + en） |
| 运行时 | 内置 Node 24 / Python 3.12（`go:embed` + 首次解压，可被系统版替代） |

## 目录

```
WorkBaby/
├── main.go                  入口：embed + wails.Run + 单实例守卫
├── app.go                   App（持有 *api.Handler，生命周期委托给它；不嵌入，见 specs/04）
├── backend/
│   ├── server/              gin 路由 / SSE hub / 统一响应 / 方法白名单
│   ├── api/                 薄 handler + 系统能力绑定（唯一允许 import wails 的包）
│   ├── pi/                  能力域：子进程监督 / RPC 编解码 / 事件归一 / 会话文件索引
│   │   ├── env/             运行时复用、PI 安装与探测、models/settings/mcp 互通
│   │   ├── pitest/          假 PI 进程（测试夹具：走真管道的 RPC 替身）
│   │   └── piptest/         编译假 PI、定位仓库根（测试夹具）
│   ├── service/             编排：会话 / 对话 / 模型 / 技能 / 插件 / 工具 / MCP / 统计 / 设置
│   ├── repo/                GORM（只有设置 KV）
│   ├── domain/              域模型（一个聚合根一个文件）
│   ├── skill/               技能解析与注册表（目录即 PI 的技能目录）
│   ├── git/                 工作区版本视图（读全开放 + 写白名单动作；冲突处置与分级强制推送）
│   ├── runtime/             路径解析 + 内置 Node / Python 运行时
│   ├── config/ db/ pkg/     配置 / 迁移 / 叶子工具包
│   └── tray/ singleinstance/
├── frontend/src/            api / components / composables / locales / stores / types / utils / views + 四个 CSS
├── assets/docs/             内置帮助手册
├── docs/ specs/             项目级文档与子系统规格
└── tools/check-boundaries   依赖方向门禁
```

## 开发

```bash
wails dev                      # 热重载（前端改完自动刷新）
pwsh scripts/check.ps1         # 快门：格式 + 编译 + 短测试（日常，约 10s）
pwsh scripts/check.ps1 -Pkg backend/pi   # 只验一个包（只碰一个能力域时，约 5s）
pwsh scripts/check.ps1 -Chain 提问   # 跨包只跑命中名字的链路（只编命中的包）
pwsh scripts/check.ps1 -List   # 列全部测试入口与子测试名
pwsh scripts/check.ps1 -Full   # vet + 依赖边界 + 全量测试 + 前端构建（发版前 / CI）
wails build                    # 产物 build/bin/WorkBaby.exe
```

跑起来要 Go 1.24+、Node 24+、Wails CLI。PI 与模型不预装也行：环境向导会带用户装。

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范（唯一权威：分包 / 依赖方向 / 错误码 / 测试 / 命名） |
| [DESIGN.md](DESIGN.md) | 视觉语言与交互原则 |
| [docs/OVERVIEW.md](docs/OVERVIEW.md) | **项目全景**：能力清单、模块地图、两个真相源、一次对话的链路、关键取舍 |
| [docs/MODULES.md](docs/MODULES.md) | **模块设计**：每个模块是什么、怎么实现、为什么、优势与代价 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 分层、启动时序、事件出口、消息顺序、前端状态归属 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 哪些数据属于 PI、哪些属于 WorkBaby、会话文件里读什么 |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | HTTP 与 SSE 契约（路径表 + 事件表） |
| [docs/PAGE-STRUCTURE.md](docs/PAGE-STRUCTURE.md) | 页面与组件地图 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 错误码段位与前端分流 |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | 构建、安装器、CI 与 Release |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | 环境要求、验证入口、数据位置、改代码时看什么 |

### 子系统规格（specs）

一个子系统一篇，每篇都是「定位 → 设计 → 契约 → 关键流程 → 约束 → 取舍」。

| # | 子系统 | # | 子系统 |
|---|---|---|---|
| [01](specs/01-runtime.md) | 运行时与路径 | [08](specs/08-workspace-git.md) | 工作区版本视图 |
| [02](specs/02-session.md) | 会话 | [09](specs/09-mcp.md) | MCP 外部工具 |
| [03](specs/03-skill.md) | 技能 | [10](specs/10-pi-supervisor.md) | PI 监督 |
| [04](specs/04-desktop-shell.md) | 桌面壳 | [11](specs/11-pi-bootstrap.md) | PI 引导 |
| [05](specs/05-user-manual.md) | 应用内手册 | [12](specs/12-workspace-files.md) | 工作区文件面板 |
| [06](specs/06-prompt-and-settings.md) | 模型、提示与人设 | [13](specs/13-extensions.md) | 插件（扩展与包） |
| [07](specs/07-usage-and-context.md) | 用量与上下文 | | |

应用内置手册面向用户（`assets/docs/`），specs 与 docs 面向开发，两者不重复维护同一份话术。

内部项目，未公开发布。
