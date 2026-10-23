# 开发指南

## 环境要求

- Go 1.24+
- Node.js 24（前端构建；与内置 Node 同一大版本）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`（与 CI 同一个 pin）
- Windows 10/11（唯一目标平台）
- 可选：`pi`（调试真实内核时用；不装也能开发，`pi/pitest` 提供假进程）

## 常用命令

验证只有一个入口：`scripts/check.ps1`。分档规则、保留判据与全部入口索引都在
AGENTS.md §3，这里只留平时手边那几条：

```powershell
pwsh scripts/check.ps1                        # 日常快门（格式 → 编译 → 全部短测试）
pwsh scripts/check.ps1 -Pkg backend/service   # 明确只碰了一个包
pwsh scripts/check.ps1 -Run 冲突               # 只跑名字命中的子测试（不必知道它挂在哪个入口下）
pwsh scripts/check.ps1 -List                  # 入口与子测试名的索引
pwsh scripts/check.ps1 -Full                  # 提交前 / CI：前端构建 + vet + 依赖门禁 + 全量测试

cd frontend
npm ci                                        # 首次（锁文件优先）
npm run build                                 # vue-tsc + vite build → frontend/dist
```

定位测试的标准动作：先 `-List` 拿索引，再 `-Run <关键字>` 落到一段，不翻测试文件。
入口一律叫 `Test<Domain>Chain`，一个包一个入口，一个子测试只讲一种坏法；
保留判据与全部索引在 AGENTS.md §3。

单独跑某一步时才手敲：

```powershell
go build ./...                                  # 比测试更早暴露类型错误，覆盖全部包
go test -short -count=1 ./backend/service       # 只测一个包
go test -short -count=1 -run 'TestServiceChain/会话索引' -v ./backend/service   # 定点到一段
go run ./tools/check-boundaries                 # 依赖方向门禁
```

验证脚本在 `-Full` 档把测试输出落到 `.check/test-output.log`；失败时自动打印失败摘要——
先抓 `--- FAIL` / `panic` / `文件:行号`，抽不到再退回末尾若干行。
本地直接看屏幕，CI 里同一份内容追加进 job summary，不必下载 artifact。

## 调试真实 PI

界面里的 PI 进程由 `backend/pi` 拉起。想让它用某一份特定的 `pi`：

```powershell
$env:WORKBABY_PI_EXE = 'C:\path\to\pi.cmd'
```

想隔离数据（不碰你真实的 `~/.pi/agent`）：

```powershell
$env:WORKBABY_HOME = "$env:TEMP\wb-dev"
$env:PI_CODING_AGENT_DIR = "$env:TEMP\wb-dev\agent"
```

**两个变量都必须设**：`WORKBABY_HOME` 只管数据目录，`PI_CODING_AGENT_DIR` 才管 PI 的配置与会话。
只设前者会读写你真实的 PI 状态。

## 数据落在哪

```
%APPDATA%\WorkBaby\            （或 WORKBABY_HOME）
├── workbaby.db                SQLite（WAL）：只有界面设置 KV
├── config.yaml                工作目录等固定项
├── logs\                      app.log（全量）/ warn.log（warn 与 error）
├── bin\pi.cmd · pi            终端里的 pi 入口（cmd/PowerShell 用 .cmd，Git Bash 用 pi；目录挂在用户 PATH 上）
├── runtime\node\              内置 Node（首次用时解压）
├── runtime\gitbash\           内置 Git Bash（本机没有可用 bash 时才解压）
├── runtime\npm-global\        WorkBaby 代装 PI 的 npm 前缀
├── runtime\shell.json         命令执行环境的自愈记录（写过什么才敢撤回什么）
├── runtime\npm-cache\         npm 缓存
├── tmp\                       临时解压与中转（启动时清扫）
├── background\                自定义背景图副本
└── persona.md                 附加系统提示（--append-system-prompt 传给它）

~/.pi/agent\                   （或 PI_CODING_AGENT_DIR）
├── settings.json              defaultProvider/Model/ThinkingLevel、defaultTools、compaction、packages、extensions
├── models.json                服务商与模型
├── mcp.json                   MCP 服务（全局）
├── sessions\--<cwd>--\*.jsonl 会话（界面读这里）
├── skills\                    用户技能（停用的在 skills.disabled\）
├── extensions\                用户扩展
└── npm\node_modules\          pi install 装下来的包
```

排障第一步永远是看 `logs\app.log` 的启动横幅：它同时打印数据目录与 PI 配置目录。

## 改后端时

| 目录 | 注意 |
|---|---|
| `backend/pi` | 协议编解码与会话索引都在这里；**不要**在这里 import 任何上层包 |
| `backend/pi/env` | 配置文件路径与写入都在这里；写 `settings.json` 只改掌管键 |
| `backend/service` | 编排层：一个关注点一个文件；不写 SQL、不引 gin |
| `backend/api` | 薄 handler：解析参数 → 调 service → `ok/fail`；组合根在 `handler.go` |
| `backend/server` | 路由表与 SSE；路径与前端 `api/index.ts` 一一对应 |
| `backend/domain` | 入参 / 出参 / 实体都放这里，四层不得就地声明结构体 |

新增一个能力时的顺序：`domain` → `repo`（若要落库）→ `service` → `api` → `server/routes.go`
→ `frontend/src/api/index.ts` → `types/api.ts` → 组件。

## 改前端时

| 目录 | 注意 |
|---|---|
| `views/` | 六个路由视图；一级导航只在 `layout/NavRail.vue`，视图内不再自画导航入口 |
| `components/` | 按域分目录：`layout` / `chat` / `dashboard` / `settings` / `workspace` / `common` |
| `stores/` | Pinia Setup Store；SSE 只放临时态，权威态一律来自快照 |
| `types/api.ts` | 与后端 RESP 的 json tag 一一对应；改字段必须同步 `ContractVersion` |
| `locales/` | 中英两套文案，key 按域分层；组件里不写用户可见字符串；文案里的裸 `@` 必须写成 `{'@'}`，否则被 intlify 当 linked 语法报「Invalid linked format」 |
| `themes.css` | 色值与尺寸刻度的唯一来源；组件里禁止写 hex |
| `ep-theme.css` | Element Plus 变量 → `--wb-*` 的桥；按钮五态等细节对齐 |
| `base.css` | 重置、排版基线、滚动条、面与图标基元 |
| `content.css` | EP 没有对应物的自绘区：消息流 / markdown / 代码块 / 工具卡 / 思考块 / diff |
| `uno.config.ts` | 原子类刻度已接到 `--wb-*`：`gap-4` = `var(--wb-sp-4)`，禁止写裸像素 |

浏览器里看界面：`npm run dev` 会因为没有后端而显示空态——
要连真后端请用 `wails dev`。

## 常见问题

| 现象 | 先看 |
|---|---|
| 技能 / 插件 / 模型全是空 | `PI_CODING_AGENT_DIR` 是否被覆盖；日志里的「PI 配置目录」是不是你期望的那个 |
| 助手执行命令报 WSL 的错 | 命令执行环境选中了 WSL 启动器；点「设置 · 助手引擎 → 重新检测并修复」，或装 Git for Windows 后重跑 |
| 终端里敲 `pi` 找不到 | 「设置 · 助手引擎 → 终端命令」生成入口并挂 PATH，**新开**终端才生效 |
| 发了消息没反应 | 环境闸门是否 ready；日志里 `chat: 已发送` 有没有出现 |
| 同一目录第二条对话报 5005 | 这是内核约束（一个目录一个当前会话），等第一条跑完 |
| 提问卡点了没反应 | `POST /chat/answer` 的 5010 说明它已失效；看 PI 是否还在跑这一轮 |
| 仪表盘数字与账单不一致 | 正常：单价是静态的，见 specs/07 |
