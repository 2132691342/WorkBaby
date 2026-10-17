# 开发指南

## 环境要求

- Go 1.24+
- Node.js 20+（前端）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`（与 CI 同一个 pin，避免本地 CLI 版本漂移）
- Windows 10/11（唯一目标平台）

## 常用命令

```powershell
# 验证入口：一个脚本收口，别在本地与 CI 各写一套（分档说明见 AGENTS.md §3.3）
pwsh scripts/check.ps1                                      # 快门：格式 → vet → 边界 → 核心链路四包 → 前端类型
pwsh scripts/check.ps1 -All                                 # 全档：再加所有包的短测试
pwsh scripts/check.ps1 -Full                                # 提交档：全量测试 + 前端类型 + 前端构建
pwsh scripts/check.ps1 -Pkg backend/service                 # 只跑一个包

# 单独跑某一步时才手敲
go test -short -count=1 ./...                               # 所有包短测试：跳过真 git 链路与归档真实解压
go test -count=1 ./...                                      # 全量：13 个入口，两条慢链路都跑到
go test -run 'TestServiceChain/预算与用量口径' ./backend/service   # 定点一个分支
go test -race ./backend/...                                 # 竞态检测，改并发相关代码时用（需 CGO 与 gcc）

# 依赖方向门禁（改包结构或调整依赖后必跑）
go run ./tools/check-boundaries

# 前端类型检查 / 构建
cd frontend
npm install
npm run build        # vue-tsc + vite build，产物 frontend/dist

# 桌面应用（重新生成 Wails 绑定 + 编译 Go + 构建前端）
wails build          # 产物 build/bin/WorkBaby.exe

# 开发模式（前端热更 + Go 重编译）
wails dev
```

## 改界面时：先在浏览器里看

`wails dev` 每次都要开窗、重编 Go，慢。而界面是纯 HTTP + 事件驱动的，
所以 `frontend/src/dev/` 备了一套假后端：`npm run dev` 直接在普通浏览器里
跑出成品界面，改完刷新就看到，不必每次 `wails build`。

```powershell
cd frontend
npm.cmd run dev    # 打开 http://127.0.0.1:5173
```

- 假数据在 `dev/fixtures.ts`，会话/技能/文档支持增删，能真的点「新建技能」
- `?empty=1` 打开空态，用来检查「什么都没有」时的界面
- 生产构建下 `import.meta.env.DEV` 为 false，整段被摇掉，不进包

> PowerShell 下必须写 `npm.cmd`，`npm` 会被执行策略拦下。

**局限**：这只验证界面，不代表后端联调通过。真联调仍要 `wails dev`。

## 布局纪律（改界面前必读）

界面「歪」的根因几乎全是布局，不是配色：

| 症状 | 病因 |
|---|---|
| 页面横向撑破、右侧被裁 | flex 子项默认 `min-width:auto`，不写 `min-width:0` 就会退回内容宽度 |
| 一行文字竖着排成几列 | 容器是 `flex` 但漏了 `flex-direction: column` |
| 读数被压成「3.9 / 秒」两行 | `flex:1` 等分在窄窗口宽度不够，改 `auto-fit + minmax()` 自行换行 |
| 按钮被挤出可视区 | 行内长文本不设 `min-width:0` + 省略号 |

窗口最窄 960px，所有布局都要在这个宽度下成立。

## 目录速查

| 路径 | 内容 |
|---|---|
| `backend/pi` | PI 子进程监督：进程 / RPC 编解码 / 事件翻译（不引 gin/wails，只能靠 pkg 与 runtime） |
| `backend/pi/env` | 环境引导：内置 Node、PI 的发现与安装、models.json / settings.json / mcp.json 互通 |
| `backend/service` | 编排层：会话/对话/审批桥/配置 |
| `backend/service/memory.go` | 记忆：跨对话的长期事实，注入 APPEND_SYSTEM.md（不单独成包，见 `specs/22`） |
| `backend/history` | 通用会话历史：只读扫描本机其它 AI 客户端的会话文件 |
| `backend/archmap` | 项目架构分析：import 解析 → 分层图数据 |
| `backend/server/routes.go` | 路由注册唯一入口 |
| `backend/server/sse.go` | SSE Hub：分发 / 重放 / 慢客户端策略 |
| `tools/check-boundaries` | 依赖方向门禁实现（独立 Go 程序，`go run ./tools/check-boundaries`） |
| `frontend/src` | 前端源码（唯一一层；`api / components / stores / views` 等平级） |
| `frontend/src/themes.css` | 色值与字体的唯一定义处 |
| `frontend/src/wb-ui.css` | 组件基元唯一实现 |
| `frontend/wailsjs` | Wails 生成的绑定（`wails.json` 的 `wailsjsdir`），不手改、不进 `src` |

## 数据目录（运行时）

```
%APPDATA%/WorkBaby/            # = C:\Users\<你>\AppData\Roaming\WorkBaby
├── workbaby.db                # SQLite（WAL）：会话元数据 / 用量 / 运行记录 / 技能 / 知识库 / 记忆
├── config.yaml                # Viper 配置（MasterKey 等）
├── logs/                      # app.log（全量）+ warn.log（warn 与 error）
├── runtime/node/              # 内置 Node 解压后（PI 靠它跑）
├── runtime/npm-global/        # npm 全局前缀，PI 装在这里
├── runtime/python/            # 内置 Python 解压后
├── pi-agent/                  # PI 的 agent 目录（PI_CODING_AGENT_DIR 指向它）
│   ├── settings.json          #   默认模型 / 思考档 / defaultTools / 压缩参数
│   ├── models.json            #   服务商与模型（WorkBaby 是唯一写入者）
│   ├── mcp.json               #   MCP server（含内置的 workbaby-knowledge）
│   ├── APPEND_SYSTEM.md       #   人设 + 记忆 + 技能清单（受控标记块）
│   ├── workbaby-policy.json   #   审批档（由审批扩展读）
│   ├── extensions/            #   workbaby.ts：审批闸门 + 记忆写入工具
│   ├── skills/                #   PI 的技能目录（= WorkBaby 的用户技能目录）
│   └── sessions/              #   PI 的会话 JSONL：对话真身在这里
├── builtin-skills/            # 内置技能落盘处（模型按路径读取）
├── site-packages/             # 内置 Python 装的第三方包（跨运行时版本保留）
└── tmp/                       # 超限输出的全文
```

**两处存储**：`workbaby.db` 存 WorkBaby 自己的账（会话元数据 / 用量 / 运行记录），
`pi-agent/sessions/` 存对话真身（PI 写的 JSONL）。WorkBaby **绝不写**后者。

`site-packages` 不放进 `runtime/python/`：后者在内置 Python 换版本时整个清场，
装过的包会跟着一起没。它同时被写进 `python312._pth`（embeddable 版只按这个文件
决定 `sys.path`，不认 `PYTHONPATH`），否则表现是「包装上了但 import 不到」。

`npm-global` 单独一层是必要的：系统 npm 全局前缀通常需要管理员权限，
写失败时 npm 报的是权限错，用户完全看不懂。

`WORKBABY_HOME` 环境变量可覆盖根目录，便于便携版与测试隔离。
`WORKBABY_PI_EXE` 可指定一份特定的 PI（测试与高级用户用）。
工作目录默认用户主目录，可放 `.pi/skills/` 提供工作区级技能。

## 测试怎么写

**只保留「失败即意味着某条真实链路坏了」的测试**。判据：这条断言失败时，是否有某个跨模块 /
跨轮次 / 跨协议的行为出了问题？纯函数的输入输出、字段映射、构造冒烟一律不写——它们不缩短排错时间，
只增加每次改动后的阅读成本与上下文负担。

- **一个包一个入口**：入口统一叫 `Test<Domain>Chain`，同包内其余函数是 `check*` 辅助
  （不是 `Test*`），由入口调起。多入口会让「这个包测了什么」分散在若干个函数里，
  失败时也不知道该看哪一个
- **一条链路一个入口**：同类分支用 `t.Run` 归到同一个入口下，
  失败时从输出直接看出是哪条链路、哪个分支坏了，
  **只有一个分支的 `t.Run` 不要写**——它只多一层缩进，不提供任何定位信息
- **重成本低层级只准备一次**：起 HTTP 服务、解压归档这类昂贵 setup 放在父测试里，
  子测试共享；否则每次加分支都在给总时长做乘法
- **装配类测试不真解压归档**：`backend/runtime/runtimetest.SeedMarkers` 预置
  「已解压 + 版本标记」，装配只跑配置 / DB / 服务 / 工具注册的真实链路。
  真解压只在 `runtime` 包的 `TestBundledRuntimeChain` 做一次——那是全量测试
  唯一的慢点，日常用 `-short` 跳过，是刻意的
- 多轮对话用 `backend/pi/pitest` 的**假 PI 进程**驱动：它是真实子进程，走真实
  stdin/stdout 协议，所以测的是编解码与生命周期本身，不是 mock。真 PI 只在 `-short`
  外的集成检查里跑一次。
- 环境依赖（系统 shell、真实网络、内置运行时归档、真 git）必须先探测再决定跳过，
  而不是喂假数据让测试「绿着跑」
- 首行注释写清楚这条链路覆盖什么、坏了的表现是什么；文件里不留「曾经坏过」这类历史叙述
- 临时数据用 `t.TempDir()`，不要写进真实数据目录

保留哪几条链路、每条覆盖什么，见 `AGENTS.md §3.4` 的测试索引。

## 验证命令

分档与命令的唯一权威是 [`AGENTS.md` §3.3](../AGENTS.md)（本节不重复表格，避免两处各写一套后走样）。
取舍只有一句：**改哪里跑哪里**——默认档只跑核心链路四包，独立子系统的护栏用 `-All`，
真 git 与归档解压只在 `-Full`（本地提交前与 CI）跑到。

单独跑某一步时才手敲原生命令：

| 目的 | 命令 |
|---|---|
| 单个包 | `go test -count=1 ./backend/pi` |
| 定点一条链路 | `go test -run 'TestPiChain/未知记录忽略不报错' ./backend/pi` |
| 依赖方向门禁 | `go run ./tools/check-boundaries` |
| 竞态检测 | `go test -race ./backend/...`（需 CGO 与 gcc，分钟级） |

耗时大头是**编译**（20 个包 + wails/gin/gorm 依赖），测试本体多在亚秒级——
`-Pkg` 省的是编译，不是断言。CI（`.github/workflows/ci.yml`）跑的就是 `-Full` 那档。

运行时归档（`backend/runtime/bundled/*.zip`）直接随仓库托管（Git LFS），
升级时手工替换 zip 并同步版本常量与 SHA 常量，没有取包脚本。

## 排错

| 现象 | 检查 |
|---|---|
| 启动即闪退 | `%APPDATA%/WorkBaby/logs/warn.log`（warn 与 error 共落此文件） |
| 前端连不上后端 | 是否走 `wails dev`（端口靠 `app:ready` 注入）；单独 `npm run dev` 调不到后端，但能看界面 |
| FTS 检索无结果 | `db.go` 里 ftsStatements 是否全部执行成功（逐条 Exec） |
| 界面一直转圈 | PI 进程没吐 `agent_settled`：看 `pi-agent/` 是否存在、Node 是否就绪；`/env` 给闸门快照 |
| 声称完成但还在跑 | `chat:done` 是否错挂在 `agent_end` 上（应只在 `agent_settled`） |
| 审批卡点了没反应 | `workbaby.ts` 扩展是否在 `settings.json` 的 `extensions` 里；决策是否带了正确的 `session_id` |
| 模型 400 | 多半是 PI 侧的协议配对；看 PI 的 stderr 尾部（`Client.Stderr()`） |
| 改了 `mcp.json` 没生效 | PI 在启动期读它；必须走重载（重启进程）才能生效 |
| 样式改动没生效 | 硬编码颜色与裸像素字号会绕过 `themes.css` / `wb-ui.css`；用 `grep` 查 hex 与 `font-size: Npx` |
