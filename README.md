# WorkBaby

**PI 的桌面工作台。** 它不自己实现 AI——它在 PI 之上做一件更实际的事：
把那个只会在终端里跑 coding agent 的引擎，变成一个能看、能点、能管的工作台。

```
PI 负责运行 Agent，WorkBaby 负责让你看见并控制它是怎么运行的。
```

## 你会得到什么

| | 能力 |
|---|---|
| 对话 | 流式正文、思考过程、工具卡（参数 / 输出 / 耗时）、插话、排队、停止、自动重试提示 |
| 会话 | 直接读 PI 的会话目录：终端里聊过的，这里也看得到，点开接着聊 |
| 模型 | 读写 PI 的 `models.json`：这里加的服务商，`pi` 命令也立刻能用 |
| 技能 | 列出 PI 真正会加载的技能（用户目录 / 工作区 / 插件包），启停靠移目录 |
| 插件 | 包走 `pi install/remove`，扩展走 `extensions/` + 登记表；进度流式打印 |
| 工具 | 清单来自 PI 写的工具声明（内置 / 插件 / MCP），开关写 `defaultTools` |
| MCP | 配置原文编辑 + `pi mcp list` 的在线状态与工具清单，支持 OAuth 登录 |
| 工作区 | 改动 / 提交 / 历史 / 分支：助手动了什么，在这里对账并收成一次提交 |
| 仪表盘 | token 与花费（按天 / 模型 / 会话），全部来自会话文件 |
| 环境 | 复用系统已有的 Node / Python / PI；缺了才用内置运行时兜底 |

「能看见」的落点是工具卡、提问卡、工作区差异与用量仪表盘；
「能控制」的落点是每一个落盘配置的开关与 PI 自己的命令行。

## 它不做什么

- 不重写 Agent 内核（工具、压缩、重试、缓存、MCP、扩展、子代理都由 PI 演进）；
- 不装审批闸门、不改 PI 的默认行为（默认不打断是 PI 的设计，想要闸门就装一个扩展）；
- 不另存一份会话与用量（那会得到两本账，而两本账迟早分叉）。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.24（单 module） |
| 桌面壳 | Wails v2（Frameless，前端自绘标题栏） |
| 业务 API | gin（回环 + 随机端口；只允许 GET / POST） |
| 实时通信 | SSE（`/api/v1/events`） |
| 存储 | SQLite（WAL，pure-Go 无 CGO）：**只存界面设置 KV** |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + Vue Router（hash） |
| 样式 | 原生 CSS 设计令牌（`themes.css` + `wb-ui.css`），无 UI 组件库 |
| 渲染 | marked · dompurify · highlight.js（唯一出口 `utils/md.ts`） |
| 运行时 | 内置 Node 24 / Python 3.12（`go:embed` + 首次解压，可被系统版本替代） |

## 目录

```
WorkBaby/
├── main.go                入口：embed + wails.Run + 单实例
├── app.go                 App（嵌入 *api.Handler，生命周期委托）
├── backend/
│   ├── server/            gin 路由 / SSE hub / 统一响应 / 方法白名单
│   ├── api/               薄 handler + 系统能力绑定（唯一允许 import wails 的包）
│   ├── pi/                能力域：子进程监督 / RPC 编解码 / 事件翻译 / 会话文件索引
│   │   ├── env/           运行时复用、PI 安装、models/settings/mcp 互通
│   │   └── pitest/        假 PI 进程（测试夹具）
│   ├── service/           编排：会话 / 对话 / 模型 / 技能 / 插件 / 工具 / MCP / 统计 / 设置
│   ├── repo/              GORM（只有设置 KV）
│   ├── domain/            域模型（一个聚合根一个文件）
│   ├── skill/             技能解析与注册表（目录即 PI 的技能目录）
│   ├── git/               工作区版本视图（读 + 白名单六动作）
│   ├── runtime/           路径解析 + 内置运行时
│   ├── config/ db/ pkg/   配置 / 迁移 / 叶子工具包
│   └── tray/ singleinstance/
├── frontend/src/          api / components / composables / stores / types / utils / views
├── assets/docs/           内置帮助手册
├── docs/ specs/           项目文档与子系统规格
└── tools/check-boundaries 依赖方向门禁
```

## 开发

```bash
wails dev                      # 热重载（前端改完自动刷新）
pwsh scripts/check.ps1         # 格式 + 编译 + 短测试（日常）
pwsh scripts/check.ps1 -Full   # vet + 依赖边界 + 全量测试 + 前端构建（发版前 / CI）
wails build                    # 产物 build/bin/WorkBaby.exe
```

跑起来需要 Go 1.24+、Node 20+、wails CLI。PI 与模型不预装也行：环境向导会带用户装。

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范（唯一权威：分包 / 依赖方向 / 错误码 / 测试） |
| [DESIGN.md](DESIGN.md) | 视觉语言与交互原则 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 分层、两条真相源、一次对话的时序 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 哪些数据属于 PI、哪些属于 WorkBaby |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | HTTP 与 SSE 契约 |
| [docs/PAGE-STRUCTURE.md](docs/PAGE-STRUCTURE.md) | 页面与组件地图 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 错误码与前端分流 |
| [specs/](specs) | 子系统规格（运行时 / 会话 / 技能 / 模型 / 用量 / git / MCP / 监督 / 引导 / 插件） |

## 许可

内部项目，未公开发布。
