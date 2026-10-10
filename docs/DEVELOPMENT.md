# 开发指南

## 环境要求

- Go 1.24+
- Node.js 20+（前端）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Windows 10/11（唯一目标平台）

## 常用命令

```powershell
# 后端测试：日常改动用 -short（跳过归档解压，秒级返回）
go test -short ./...                                        # 日常
go test -count=1 ./...                                      # 全量（14 个 Test / 32 个子测试），含归档真实解压
go test -short ./backend/service                            # 只跑一个包
go test -short -run TestServiceRunChain ./backend/service   # 只跑一条链路
go test -race ./backend/...                                 # 竞态检测，改并发相关代码时用

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
所以 `frontend/src/src/dev/` 备了一套假后端：`npm run dev` 直接在普通浏览器里
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
| `backend/agent` | 内核循环（纯逻辑，无 IO 依赖，不引 gin/wails） |
| `backend/service` | 编排层：会话/对话/审批/配置/系统提示 |
| `backend/tool` | 工具契约 + 注册表 + 11 个内置工具 |
| `backend/server/routes.go` | 路由注册唯一入口 |
| `backend/server/sse.go` | SSE Hub：分发 / 重放 / 慢客户端策略 |
| `tools/check-boundaries` | 依赖方向门禁实现（独立 Go 程序，`go run ./tools/check-boundaries`） |
| `frontend/src/src` | 前端源码（注意双层 src） |
| `frontend/src/src/themes.css` | 色值与字体的唯一定义处 |
| `frontend/src/src/wb-ui.css` | 组件基元唯一实现 |

## 数据目录（运行时）

```
%APPDATA%/WorkBaby/            # = C:\Users\<你>\AppData\Roaming\WorkBaby
├── workbaby.db                # SQLite（WAL）
├── config.yaml                # Viper 配置（MasterKey 等）
├── model.json                 # 模型能力缓存的本地覆写
├── logs/                      # app.log（全量）+ warn.log（warn 与 error）
├── runtime/python/            # 内置 Python 解压后
├── runtime/powershell/        # 内置 PowerShell 7 解压后
├── skills/                    # 用户全局技能
├── builtin-skills/            # 内置技能落盘处（模型按路径读取）
└── tmp/                       # 工具输出落盘、超限截断的全文
```

`WORKBABY_HOME` 环境变量可覆盖根目录，便于便携版与测试隔离。
工作目录默认用户主目录，可放 `.workbaby/skills/` 提供工作区级技能。

## 测试怎么写

**只保留「失败即意味着某条真实链路坏了」的测试**。判据：这条断言失败时，是否有某个跨模块 /
跨轮次 / 跨协议的行为出了问题？纯函数的输入输出、字段映射、构造冒烟一律不写——它们不缩短排错时间，
只增加每次改动后的阅读成本与上下文负担。

- **一条链路一个 `Test`**：同类分支用 `t.Run` 归到同一个 `Test` 下，
  失败时从输出直接看出是哪条链路、哪个分支坏了；单个文件 1–3 个 `Test` 就够
- **重成本低层级只准备一次**：起 HTTP 服务、解压归档这类昂贵 setup 放在父测试里，
  子测试共享；否则每次加分支都在给总时长做乘法
- **装配类测试不真解压归档**：`backend/runtime/runtimetest.SeedMarkers` 预置
  「已解压 + 版本标记」，装配只跑配置 / DB / 服务 / 工具注册的真实链路。
  真解压只在 `runtime` 包的 `TestBundledRuntimeChain` 做一次——那是全量测试
  唯一的慢点，日常用 `-short` 跳过，是刻意的
- 多轮对话用 `backend/llm/llmtest` 的脚本替身驱动，配合
  `factory.SetOverride("test", ...)` 注入，**绝不真联网**
- 环境依赖（系统 shell、真实网络、内置运行时归档）必须先探测再决定跳过，
  而不是喂假数据让测试「绿着跑」
- 首行注释写清楚这条链路覆盖什么、坏了的表现是什么；文件里不留「曾经坏过」这类历史叙述
- 临时数据用 `t.TempDir()`，不要写进真实数据目录

保留哪几条链路、每条覆盖什么，见 `AGENTS.md §3.3` 的测试索引。

## 验证命令

没有包装脚本，验证就是几条原生命令（CI 跑的也是它们，本地跑通即 CI 跑通）：

| 目的 | 命令 |
|---|---|
| 日常测试 | `go test -short ./...`——跳过归档真实解压，秒级返回 |
| 提交前全量 | `go test -count=1 ./...` |
| 定点一条链路 | `go test -short -run <正则> ./<包>` |
| 依赖方向门禁 | `go run ./tools/check-boundaries` |
| 竞态检测 | `go test -race ./backend/...`（需 CGO 与 gcc，分钟级） |

运行时归档（`backend/runtime/bundled/*.zip`）直接随仓库托管（Git LFS），
升级时手工替换 zip 并同步版本常量与 SHA 常量，没有取包脚本。

## 排错

| 现象 | 检查 |
|---|---|
| 启动即闪退 | `%APPDATA%/WorkBaby/logs/warn.log`（warn 与 error 共落此文件） |
| 前端连不上后端 | 是否走 `wails dev`（端口靠 `app:ready` 注入）；单独 `npm run dev` 调不到后端，但能看界面 |
| FTS 检索无结果 | `db.go` 里 ftsStatements 是否全部执行成功（逐条 Exec） |
| 模型 400 | 多半是 assistant/tool 配对被破坏；先看 `agent.CleanForProtocol` |
| 工具参数总是不合法 | `registry.go` 注册期编译 Schema 失败会让启动直接失败，能跑到运行期说明 Schema 是好的 |
| 样式改动没生效 | 硬编码颜色与裸像素字号会绕过 `themes.css` / `wb-ui.css`；用 `grep` 查 hex 与 `font-size: Npx` |
