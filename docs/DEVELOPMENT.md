# 开发指南

## 环境要求

- Go 1.24+
- Node.js 20+（前端）
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Windows 10/11（唯一目标平台）

## 常用命令

```powershell
# 后端测试（全量 < 10s）
go test ./internal/...

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

这一版界面「歪」的根因几乎全是布局，不是配色：

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
| `internal/agent` | 内核循环（纯逻辑，无 IO 依赖，不引 gin/wails） |
| `internal/service` | 编排层：会话/对话/审批/配置/系统提示 |
| `internal/tool` | 工具契约 + 注册表 + 11 个内置工具 |
| `internal/server/routes.go` | 路由注册唯一入口 |
| `internal/server/sse.go` | SSE Hub：分发 / 重放 / 慢客户端策略 |
| `frontend/src/src` | 前端源码（注意双层 src） |
| `frontend/src/src/themes.css` | 色值与字体的唯一定义处 |
| `frontend/src/src/wb-ui.css` | 组件基元唯一实现 |

## 数据目录（运行时）

```
%APPDATA%/WorkBaby/            # = C:\Users\<你>\AppData\Roaming\WorkBaby
├── workbaby.db                # SQLite（WAL）
├── config.yaml                # Viper 配置（MasterKey 等）
├── logs/                      # info/warn/error 分文件
├── runtime/python/            # 内置 Python 解压后
├── skills/                    # 用户全局技能
└── tmp/                       # 工具输出落盘、超限截断的全文
```

`WORKBABY_HOME` 环境变量可覆盖根目录，便于便携版与测试隔离。
工作目录默认用户主目录，可放 `.workbaby/skills/` 提供工作区级技能。

## 测试怎么写

- 只写「失败意味着真实链路坏了」的测试（判据见 `AGENTS.md §3.1`）
- 按链路组织：一个测试讲一件事，同类断言用 `t.Run` 归到同一个 Test 下，
  这样失败时从输出就能看出是哪条链路、哪个分支坏了
- 多轮对话用 `internal/llm/llmtest` 的脚本替身驱动，配合
  `factory.SetOverride("test", ...)` 注入，**绝不真联网**
- 文件名首行写导航注释，说明覆盖什么
- 临时数据用 `t.TempDir()`，不要写进真实数据目录

测试索引见 `AGENTS.md §3.3`。

## 排错

| 现象 | 检查 |
|---|---|
| 启动即闪退 | `%APPDATA%/WorkBaby/logs/error.log` |
| 前端连不上后端 | 是否走 `wails dev`（端口靠 `app:ready` 注入）；单独 `npm run dev` 调不到后端，但能看界面 |
| FTS 检索无结果 | `db.go` 里 ftsStatements 是否全部执行成功（逐条 Exec） |
| 模型 400 | 多半是 assistant/tool 配对被破坏；先看 `agent.CleanForProtocol` |
| 工具参数总是不合法 | `registry.go` 注册期编译 Schema 失败会让启动直接失败，能跑到运行期说明 Schema 是好的 |
| 样式改动没生效 | 硬编码颜色与裸像素字号会绕过 `themes.css` / `wb-ui.css`；用 `grep` 查 hex 与 `font-size: Npx` |
