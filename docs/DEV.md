# 开发

## 环境

| 需要 | 版本 | 说明 |
|---|---|---|
| Go | 1.26.x | 编译后端与壳 |
| Node.js | ≥ 20 | **只用于构建前端**；运行时的 node 是随包分发的内置副本 |
| wails3 CLI | 3.1.x | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | 3.x | 任务入口（可选，也可直接敲 go 命令） |
| WebView2 Runtime | Win10 1803+ 自带 | Wails 渲染层 |

不需要预装 Python / Rust / 任何 C 编译链——`glebarez/sqlite` 是纯 Go 实现，
**全项目无 cgo**。

## 常用命令

```bash
go mod download

task dev      # wails3 dev：后端热重载 + Vite 热更
task build    # 产出 bin\WorkBaby.exe
task run      # 直接跑（不打包）
task check    # gofmt 检查 + go vet + go test
task package  # NSIS 安装包（发布前）
```

没有 Task 时等价命令：

```bash
wails3 dev -config ./build/config.yml -port 9245
go build -o bin/WorkBaby.exe .
gofmt -l backend main.go && go vet ./backend/... . && go test ./backend/... -count=1
```

## 纯浏览器调试前端

壳内运行的 WebView 通过 Wails 绑定拿端口与令牌。想让浏览器直连后端：

1. 用固定令牌起壳或后端：`WB_DEV_TOKEN=devtoken task dev`
2. 浏览器控制台写入：

```js
localStorage.setItem('wb.dev.port', '9245')
localStorage.setItem('wb.dev.token', 'devtoken')
```

重启前端即走 HTTP 直连，不依赖 Wails 绑定——前端 90% 的调试不必起窗。

## 数据目录隔离

开发态的 `Layout.Dev` 为 `true`，数据目录是 `%LOCALAPPDATA%\WorkBaby-dev`，
与安装态完全隔离。删掉这个目录等于恢复出厂。

## 测试策略

测试的价值在「改这里会坏那里」的地方；平凡断言不写。

| 文件 | 断言什么 |
|---|---|
| `tests/e2e_server_test.go` | 端到端长链路（3 个场景）：鉴权硬门（HTTP 头 + SSE 查询参数）、信封契约（未知路由 / 坏请求体 / bootstrap 成功同构）、设置链路（KV 落 SQLite、工作区去重、关窗值归一） |
| `tests/session_replay_test.go` | 会话回放管线（5 个场景）：活动分支、工具结果挂接、压缩与改名条目、隐藏台账、残行与环安全 |
| `tests/builtin_catalog_test.go` | 内置清单（6 个场景）：清单自洽与来源串往返、清单与资产对齐、技能开关搬目录、默认停用落位、插件启停与卸载生命周期、版本漂移判定 |

约定：

- 位置在仓库根 `tests/`，一文件一个被测对象，`go test ./...` 十几秒内跑完
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`
- 每个测试用 `t.TempDir()` 造独立布局，不碰真实数据目录
- 需要真实引擎或网络的路径不写悬空用例——测不了就不测，别留假绿

## 门禁

```bash
go build ./...                                  # 1 全包可编译
go vet ./...                                    # 2 静态检查
cd frontend && npx vue-tsc --noEmit -p tsconfig.json   # 3 前端类型检查
go test ./... -count=1                          # 4 测试全绿
```

只动后端跑 1 / 2 / 4；只动前端跑 3。**发布前另跑 `task build` 与 `task package`**。

## 排错

| 现象 | 先看哪里 |
|---|---|
| 窗口起来但一片空 | 设置页 → 诊断 → 日志；`Endpoint()` 的错误会在前端渲染成诊断页 |
| 内置 node 未就绪 | 运行时压缩包是否在 `assets\runtime\`；首启解压日志 |
| pi 装不上 | 网络能否到 npm；`settings.json` 的 `npmCommand` 是否指向内置 node |
| 插件装了没生效 | `pi\settings.json` 的 `packages[]` 是否真有该条目且未 `autoload:false` |
| 技能不出现 | 是否在 `pi\skills\`（不是 `skills_disabled\`）；SKILL.md 头是否合法 |
