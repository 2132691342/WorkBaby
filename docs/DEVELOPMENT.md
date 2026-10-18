# 开发指南

## 环境要求

- Go 1.24+
- Node.js 20+（前端构建）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`（与 CI 同一个 pin）
- Windows 10/11（唯一目标平台）
- 可选：`pi`（调试真实内核时用；不装也能开发，`pitest` 提供假进程）

## 常用命令

```powershell
# 验证入口：一个脚本收口，别在本地与 CI 各写一套（分档说明见 AGENTS.md §3.3）
pwsh scripts/check.ps1                        # 快门：格式 → 编译 → 所有包短测试
pwsh scripts/check.ps1 -Pkg backend/service   # 只验一个包
pwsh scripts/check.ps1 -Full                  # 提交前 / CI：vet + 依赖门禁 + 全量测试 + 前端构建

# 单独跑某一步时才手敲
go test -short -count=1 ./...                 # 短测试：跳过真 git 链路与归档真实解压
go test -count=1 ./...                        # 全量
go test -run 'TestServiceChain/会话索引' ./backend/service   # 定点链上的一段
go run ./tools/check-boundaries               # 依赖方向门禁

cd frontend
npm install
npm run build                                 # vue-tsc + vite build → frontend/dist
```

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
├── logs\                      app.log / error.log
├── runtime\node|python\       内置运行时（首次用时解压）
├── background\                自定义背景图副本
├── persona.md                 附加系统提示（--append-system-prompt 传给它）
└── config.yaml                工作目录等固定项

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
| `views/` | 五个路由视图；视图内不再自画导航入口 |
| `components/` | 按视图分目录；基元在 `common/` |
| `stores/` | Pinia Setup Store；SSE 只放临时态，权威态一律来自快照 |
| `types/api.ts` | 与后端 RESP 的 json tag 一一对应；改字段必须同步 `ContractVersion` |
| `themes.css` | 色值与字体的唯一来源；组件里禁止写 hex |
| `wb-ui.css` | 组件基元（按钮五态 / 卡片 / chip / 开关 / 表格 / 空态）的唯一实现 |

浏览器里看界面：`npm run dev` 会因为没有后端而显示空态——
要连真后端请用 `wails dev`。

## 常见问题

| 现象 | 先看 |
|---|---|
| 技能 / 插件 / 模型全是空 | `PI_CODING_AGENT_DIR` 是否被覆盖；日志里的「PI 配置目录」是不是你期望的那个 |
| 发了消息没反应 | 环境闸门是否 ready；日志里 `chat: 已发送` 有没有出现 |
| 同一目录第二条对话报 5005 | 这是内核约束（一个目录一个当前会话），等第一条跑完 |
| 提问卡点了没反应 | `POST /chat/answer` 的 5010 说明它已失效；看 PI 是否还在跑这一轮 |
| 仪表盘数字与账单不一致 | 正常：单价是静态的，见 specs/08 |
