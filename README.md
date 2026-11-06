# WorkBaby

Windows 桌面单机 AI 助手：Go + Wails3 壳 + 内嵌 HTTP 服务 + pi 引擎驱动。
把 Node.js、pi 编码智能体、插件、技能、MCP 与 Git / Python 运行时装进一个沙箱，
安装即用，不依赖、不污染用户环境。

## 它是什么

- **安装即用**：内置 Node.js 24.21 / PortableGit / Python 3.13 随安装包分发，
  首次启动后台解压到 `%LOCALAPPDATA%\WorkBaby`；用户没装 node、Git、Python 也能跑。
- **pi 由壳全权管理**：pi 编码智能体经内置 npm 装进沙箱（「应用」页一键安装 / 升级 / 卸载），
  以 RPC 模式常驻子进程，会话、模型、思考等级、压缩全部由 GUI 驱动；
  引擎意外退出自动按退避重启，杀进程先等真退出再动磁盘。
- **插件生态兼容**：内置 15 个 pi 扩展包（联网访问、待办、问卷、记忆、模糊搜索、LSP、
  计划评审、自省、上下文用量、footer、Chrome 配置、Plan 模式、多 Agent、Web UI、
  上下文压缩）首启由内置 npm 自动装进沙箱；
  插件走 pi 原生的 `settings.json` → `packages` 机制，可启停 / 升级 / 卸载 / 重试，
  也能查 npm 最新版后手动装其他插件。
- **技能随包分发**：内置 22 个策展技能包首启落位沙箱，按分组展示、可开关 / 编辑 / 重置 / 新建 /
  导入 / 删除；带外部依赖的技能在界面上点明需要什么（如 `gh` CLI、.NET SDK）。
- **MCP 配置面**：stdio / HTTP 服务增删改查与开关，落在 pi 的 `mcp.json`（用户级 +
  工作区项目级）；一条「测试连接」完成 MCP 握手，连不连得上当场见分晓。
- **对话即工作**：流式输出、思考折叠、工具卡片（含差异视图）、插话 / 排队 / 停止、
  上下文水位与统计、一键压缩、待办面板（todo 工具调用派生）——引擎侧事件全部实时可见。
- **会话树可回放**：pi 的 JSONL 会话文件是唯一真相；列表、改名、删除、隐藏单轮、
  导出 HTML 都落在会话文件本身或壳的台账里，重启不丢。
- **git 面板**：状态 / 差异 / 历史 / 分支 / 提交（含修补）/ stash / 推拉全部经内置 git，
  凭证走 Windows 凭据管理器，用户没装 git 也能用。
- **看得见也点得动**：引擎提问（确认 / 选择 / 输入 / 编辑器）直接弹卡回答，
  用户与模型改的是同一份状态。

## 沙箱布局

```
%LOCALAPPDATA%\WorkBaby\
├── workbaby.db            # 设置 KV、开关台账
├── cache\npm\             # npm 缓存与全局前缀（锁在沙箱）
├── logs\                  # 按天滚动日志
├── runtime\               # 内置 node / PortableGit / python（首启解压）
├── pi-app\                # pi 本体（内置 npm --prefix 安装）
└── pi\                    # PI_CODING_AGENT_DIR（pi 的家）
    ├── settings.json      # 模型 + packages 声明 + npmCommand
    ├── models.json / mcp.json / auth.json
    ├── npm\               # 插件安装根
    ├── sessions\          # 会话 JSONL（按工作区编码分目录）
    └── skills\ skills_disabled\   # 技能与停用区
```

子进程 PATH 内置运行时在前、系统在后；agent 的 bash / python 一律先命中内置副本。
**内置运行时缺失时直接报错，绝不回退系统同名命令**——沙箱的意义就在这里。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3（WebView2，无边框 + 前端自绘标题栏） |
| 后端 | Go 1.26 · gin · gorm · `glebarez/sqlite`（纯 Go，**无 cgo**） |
| 前端 | Vue 3.4 · Vite 5 · Element Plus 2.9 · Pinia · Vue Router · Axios · UnoCSS · ECharts |
| 引擎 | pi（`@earendil-works/pi-coding-agent`，RPC 子进程） |

后端分层 `api → service → repo → domain`，`pkg/` 放可测叶子工具；
详细约束见 `AGENTS.md`。

## 开发

```bash
go mod download
cd frontend && npm install && cd ..

task dev      # wails3 dev：后端热重载 + Vite 热更
task build    # 产出 bin\WorkBaby.exe
task check    # gofmt 检查 + go vet + go test
task package  # NSIS 安装包
```

纯浏览器调试前端：把开发态的端口与 `X-Desktop-Token` 令牌写进 localStorage 即可直连后端。

## 文档

| 文档 | 内容 |
|---|---|
| `AGENTS.md` | 工程规范（分包 / 依赖方向 / 编码与测试纪律，唯一权威） |
| `DESIGN.md` | 视觉语言与设计令牌 |
| `docs/ARCHITECTURE.md` | 壳架构：进程拓扑、装配、沙箱布局、引擎生命周期 |
| `docs/API.md` | `/api/v1` 端点契约（信封 / 鉴权 / SSE 事件词汇） |
| `docs/DATA-MODEL.md` | 数据模型：SQLite、设置键、会话 JSONL 回放规则 |
| `docs/MODULES.md` | 模块职责与依赖取舍 |
| `docs/ERROR-CODES.md` | 业务码段位与码表 |
| `docs/DEV.md` | 开发流程、测试策略与门禁 |
| `docs/DEPLOY.md` | 打包形态、版本规则与 CI/CD 发布 |

## 优势与局限

**优势**：零环境依赖的 Windows AI 工作台；pi 生态全兼容；GUI 完全接管引擎管理；
会话与配置都在一个可备份的目录里。

**局限**：只支持 Windows；引擎能力以 pi 为准（无子代理面板）；服务商一律走 API Key，
不提供 OAuth 登录；插件升级依赖 npm 源可达。
