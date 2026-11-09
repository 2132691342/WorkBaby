# WorkBaby

Windows 单机 AI 工作台：安装即用，内置运行时，GUI 完全接管引擎。

把 Node.js、pi 编码智能体、插件、技能、MCP 与 Git / Python 装进一个私有沙箱，
用户不需要预装任何环境，自己的环境也不会被改动。

## 为什么需要它

命令行 AI 助手对多数人有两个门槛：

1. **环境**：要先装 Node、配 PATH、装 CLI 工具。装歪了、多版本冲突、公司网络代理，
   每一步都能卡住人。
2. **操作**：模型、插件、技能、MCP 要靠改 JSON 和记命令。看不清当前状态，
   也不知道哪一步失败了。

WorkBaby 把这两件事收进一个 GUI：运行时随包分发，首次启动自己解压；
引擎、插件、技能、MCP、模型全部在界面上增删改查。**壳不对外提供 CLI，
也不读用户自己装的 pi**——两者互不干扰。

## 能力

**零环境依赖**。内置 Node.js 24.21、PortableGit 2.55、Python 3.13（压缩后约 110MB），
首启后台解压到 `%LOCALAPPDATA%\WorkBaby\runtime`。子进程 PATH 内置副本在前、
系统在后；内置副本缺失时直接报错，**绝不回退系统同名命令**——回退就等于拿用户环境跑包。

**引擎由壳全权管理**。pi 编码智能体经内置 npm 装进沙箱，以 RPC 模式常驻子进程。
会话、模型、思考档、上下文压缩、插话与排队全部由 GUI 驱动。进程意外退出按指数退避
自动拉起，连崩 5 次停手交给用户。

**打开即用，不等引擎**。装配收尾就后台预热引擎，插件冷加载发生在你还在看概览页的
时候；对话页的历史与待办直接从会话文件回放，引擎就绪后自动切到实况——
打开应用立刻能看到上次的对话并开始打字。

**20 个内置插件**，首启自动装齐，可启停 / 升级 / 卸载 / 重试：

| 插件 | 用途 | 默认 |
|---|---|---|
| `pi-web-access` | 联网搜索、抓取 URL、克隆仓库、PDF 与视频理解 | 开 |
| `@juicesharp/rpiv-todo` | 实时待办清单，跨压缩存活 | 开 |
| `@juicesharp/rpiv-ask-user-question` | 结构化问卷，模型拿不准时先问你 | 开 |
| `pi-hermes-memory` | 持久记忆、会话检索、密钥扫描 | 开 |
| `@ff-labs/pi-fff` | 模糊文件名与内容搜索 | 开 |
| `@narumitw/pi-lsp` | 语言无关的 LSP 工具 | 开 |
| `@plannotator/pi-extension` | 计划评审、标注、PR 审查 | 开 |
| `pi-chrome` | 显式授权后复用已登录的 Chrome 配置 | 开 |
| `pi-plan-mode` | 只读探索 + 计划文件编辑 | 开 |
| `pi-lens` | LSP、lint、类型检查与符号结构化搜索 | 开 |
| `pi-tidy` | 工具调用输出折叠：多次工具调用收成一行 | 开 |
| `pi-subagents` | 单 Agent 委派与脚本化多 Agent 流程 | 开 |
| `pi-smart-compact` | 可恢复的上下文清理与压缩 | 开 |
| `pi-openui` | 终端内交互式 dashboard、图表与地图 | 开 |
| `pi-extension-doctor` | 只读诊断扩展冲突与过期 API | 开 |
| `pi-browser-harness` | 浏览器自动化 | 开 |
| `pi-inspect` | 自省：工具、命令、技能、注入的提示 | 关 |
| `pi-context-view` | 可视化上下文用量 | 关 |
| `pi-footer` | 多行状态栏 | 关 |
| `pi-web` | pi 的 Web UI | 关 |

后四个默认关闭，因为它们的职责与本壳 GUI 重叠；装上是为了让用户随时能开。

**10 个内置技能**，随包分发、首启落位，可开关 / 编辑 / 重置 / 新建 / 导入 / 删除。
筛选口径是「沙箱里跑得起来 + 日常用得上」——技能一旦落位就进系统提示，
纯噪音技能是负资产，依赖沙箱外程序（`.NET SDK`、`gh CLI`、`playwright`）的
一律不收；需要纯 Python 依赖的（`reportlab`、`openpyxl`）靠内置 pip 自装，
重依赖的那个默认停用，界面上会标出缺什么。

**对话即工作**。流式输出、思考折叠、工具卡片（含差异视图）、插话 / 排队 / 停止、
上下文水位、一键压缩、待办面板。引擎提问（确认 / 选择 / 输入 / 编辑器）直接弹卡可答。

**会话可回放**。pi 的 JSONL 会话文件是唯一真相；列表、改名、删除、隐藏单轮、
导出 HTML 都落在会话文件本身或壳的台账里，重启不丢。

**工作区**。文件树与预览、git 状态 / 差异 / 历史 / 分支 / 提交 / stash / 推拉，
全部经内置 git；凭证走 Windows 凭据管理器。工作区清单带仓库分支、未提交改动数与会话数。

**MCP**。stdio / HTTP 服务增删改查与开关，落在 `mcp.json`（用户级 + 工作区项目级），
一条「测试连接」完成 MCP 握手。

**模型**。服务商与模型增删改查，写 `models.json` + `auth.json`，密钥读出来恒为掩码。
一律 API Key，不做 OAuth。

## 沙箱布局

```
%LOCALAPPDATA%\WorkBaby\          （开发态加 -dev 后缀）
├── workbaby.json                 # 壳的偏好与台账（单文件 JSON）
├── cache\{npm,npm-global}\       # npm 缓存与全局前缀，锁在沙箱
├── logs\                         # 按天滚动
├── webview\                      # WebView2 用户数据
├── runtime\{node,PortableGit,python}\
├── pi-app\node_modules\          # pi 本体
└── pi\                           # PI_CODING_AGENT_DIR：pi 的家
    ├── settings.json             # 模型 + packages[] + npmCommand
    ├── models.json auth.json mcp.json
    ├── npm\node_modules\         # 插件
    ├── sessions\                 # 会话 JSONL（唯一持久真相）
    ├── skills\ skills_disabled\  # 技能启用 / 停用区
```

整个 pi 的家由 `PI_CODING_AGENT_DIR` 重定向。不重定向的话，壳内装的插件会和
用户自己装的 pi 混在同一目录，卸载壳就删了用户的东西。

卸载程序会先问一句**是否连数据目录一起删**，默认保留：会话与配置是用户的工作
成果，删不删由用户自己决定；WebView2 缓存无论选什么都清。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3.1（WebView2，无边框 + 前端自绘标题栏） |
| 语言 | Go 1.26 |
| 业务 API | gin 1.12（回环 HTTP，127.0.0.1 随机端口） |
| 存储 | 标准库 JSON 文件（`workbaby.json`，原子整写，**无 cgo**） |
| 前端 | Vue 3.4 + Vite 5 + Element Plus 2.9 + Pinia + Vue Router + Axios + UnoCSS + ECharts |
| 引擎 | pi（`@earendil-works/pi-coding-agent`，RPC 子进程） |

## 开发

```bash
go mod download
cd frontend && npm install && cd ..

task dev       # wails3 dev：后端热重载 + Vite 热更
task build     # 产出 bin\WorkBaby.exe
task check     # gofmt 检查 + go vet + 场景测试
task test:live # 真实沙箱链路（解压运行时 + 装 pi + RPC 握手，需联网）
task package   # NSIS 安装包（需要 makensis）
```

四道门禁（提交前必过）：

```bash
go build ./...
go vet ./...
cd frontend && npx vue-tsc --noEmit -p tsconfig.json
go test ./tests -count=1
```

## 文档

| 文档 | 内容 |
|---|---|
| `AGENTS.md` | 工程规范：分层 / 命名 / 错误码 / 测试纪律（唯一权威） |
| `DESIGN.md` | 视觉语言、设计令牌、按钮五态 |
| `docs/ARCHITECTURE.md` | 进程拓扑、装配顺序、引擎生命周期、稳定性约束 |
| `docs/MODULES.md` | 每个包与目录的职责、依赖方向、设计取舍 |
| `docs/API.md` | `/api/v1` 端点契约、SSE 事件词汇、错误码表 |
| `docs/DATA-MODEL.md` | 壳存储文件、设置键、会话 JSONL 回放规则 |
| `docs/DEV.md` | 开发环境、测试策略、门禁、打包发布、排错 |

## 局限

- 只支持 Windows x64
- 引擎能力以 pi 为准
- 插件安装依赖 npm 源可达；纯离线只能用到随包的 pi 本体与技能
- 服务商一律 API Key，不做 OAuth
