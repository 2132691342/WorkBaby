# 开发

## 环境

| 需要 | 版本 | 说明 |
|---|---|---|
| Go | 1.26.x | 编译后端与壳 |
| Node.js | ≥ 20 | **只用于构建前端**；运行时的 node 是随包分发的内置副本 |
| wails3 CLI | 3.1.x | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | 3.x | 任务入口（可选，也可直接敲 go 命令） |
| NSIS | 3.x | 只在打安装包时需要（`choco install nsis`） |
| WebView2 Runtime | Win10 1803+ 自带 | Wails 渲染层 |

不需要预装 Python / Rust / 任何 C 编译链——`glebarez/sqlite` 是纯 Go 实现，
**全项目无 cgo**。

## 常用命令

```bash
go mod download
cd frontend && npm install && cd ..

task dev        # wails3 dev：后端热重载 + Vite 热更
task build      # 产出 bin\WorkBaby.exe
task run        # 直接跑构建产物
task test       # 场景测试（秒级）
task test:live  # 真实沙箱链路：解压运行时 + 装 pi + RPC 握手（需联网，约 2 分钟）
task check      # gofmt 检查 + go vet + 场景测试
task package    # NSIS 安装包（需要 makensis）
```

没有 Task 时等价命令：

```bash
wails3 dev -config ./build/config.yml -port 9245
go build -o bin/WorkBaby.exe .
gofmt -l backend main.go tests; go vet ./...; go test ./tests -count=1
```

## 纯浏览器调试前端

壳内运行的 WebView 通过 Wails 绑定拿端口与令牌。想让浏览器直连后端：

1. 用固定令牌起壳：`WB_DEV_TOKEN=devtoken task dev`
2. 浏览器控制台写入：

```js
localStorage.setItem('wb.dev.port', '9245')
localStorage.setItem('wb.dev.token', 'devtoken')
```

重启前端即走 HTTP 直连，不依赖 Wails 绑定——前端大部分调试不必起窗。

## 数据目录隔离

开发态把数据目录解析成 `%LOCALAPPDATA%\WorkBaby-dev`，与安装态完全隔离。
删掉这个目录等于恢复出厂。

## 测试策略

只测「改这里会坏那里」的复杂流程与长链路。平凡断言（单 getter、纯函数往返、
复述框架行为）不写——它们的维护成本高于收益，还会把 `go test` 拖成几分钟。

六个默认文件 + 一个 live 文件、九个用例、约 1 秒：

| 文件 | 用例 | 断言什么 |
|---|---|---|
| `tests/http_chain_test.go` | `TestHTTPChain` | 鉴权硬门（401 且不带业务信封）、信封同构（成功 / 业务失败 / 未知路由 / 坏请求体全为 200 + `code`）、写入真的落盘（HTTP 写 → 直连 SQLite 查 → HTTP 读回三个方向对上） |
| | `TestSSEStream` | 查询参数鉴权、连上先补 `stream:open`、广播帧真的到达订阅者、广播必发号（seq 单调） |
| `tests/session_test.go` | `TestSessionReplay` | 分支遍历只走当前分支、残行不毁整份历史、环状 parentId 有界退出、工具结果回填宿主轮、孤立结果单独成轮、压缩标记、标题取自 `session_info` |
| | `TestSessionWriteDiscipline` | 隐藏轮只记壳的台账不改文件、改名只在末尾追加、引擎正打开该会话时拒绝且不动文件 |
| `tests/sandbox_test.go` | `TestBuiltinManifest` | 内置清单与磁盘资产互相咬合（18 个插件逐条对版本、22 个技能双向无死条目）；每个 SKILL.md 的 name / description 都解析得出来（YAML 块标量不破） |
| | `TestSkillLifecycle` | 落位尊重默认值且不覆盖用户改动；开关是搬目录 + 台账记账；重置后 SKILL.md 仍是可读文件；删除清掉整个目录、目录路径一律拒；「两个区各一份」的破损态自愈后清单无重复行 |
| `tests/plugin_test.go` | `TestPluginLifecycle` | 来源串规范（git 源就是 URL 本身，不带 `git:` 前缀）；停用写对象形态 `autoload:false`、启用写裸来源串；开关保留用户配的资源过滤（extensions）；重试安装不把已停用的插件静默打开；卸载撤声明并删包目录 |
| | `TestPluginVersionDrift` | 判据是「在且版本对」，Provision 与 UpgradeAll 都只碰漂移项 |
| `tests/mcp_test.go` | `TestMcpConfig` | 12 路并发写 mcp.json 一条不丢（读-改-写必须有锁）；删项目级条目不连带删掉被它遮蔽的用户级条目 |
| `tests/harness_test.go` | — | 公共装具：全后端 harness（真 SQLite + 真 gin 中间件栈）与轻量 harness、HTTP 调用与断言辅助 |
| `tests/sandbox_live_test.go` | `TestSandboxChain` | 真实链路：解压三个运行时 → 内置 npm 装 pi → RPC `get_state` / `get_commands` / `get_session_stats` → 优雅退出。带 `live` 构建标签，默认不跑 |

约定：

- 位置在仓库根 `tests/`，一文件一个被测对象
- 每个测试用 `t.TempDir()` 造独立布局，不碰真实数据目录与仓库资产
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`
- 需要真实引擎或网络的路径放 `live` 标签后，不留假绿

## 门禁

```bash
go build ./...                                          # 1 全包可编译
go vet ./...                                            # 2 静态检查
cd frontend && npx vue-tsc --noEmit -p tsconfig.json    # 3 前端类型检查
go test ./tests -count=1                                # 4 场景测试全绿
```

只动后端跑 1 / 2 / 4；只动前端跑 3。**发布前另跑 `task build` 与 `task package`**。

## 排错

| 现象 | 先看哪里 |
|---|---|
| 窗口起来但一片空 | 设置页 → 诊断 → 日志；`Endpoint()` 的错误会在前端渲染成诊断页 |
| 内置 node 未就绪 | 运行时压缩包是否在 `assets\runtime\`；首启解压日志 |
| pi 装不上 | 网络能否到 npm；`settings.json` 的 `npmCommand` 是否指向内置 node |
| 插件装了没生效 | `pi\settings.json` 的 `packages[]` 是否真有该条目且未 `autoload:false` |
| 插件换了版本还是旧行为 | 包只在引擎启动时加载：停用再启用，或重启引擎 |
| 技能不出现 | 是否在 `pi\skills\`（不是 `skills_disabled\`）；SKILL.md 头是否合法 |
| 打安装包报 makensis 找不到 | 装 NSIS 并把安装目录加进 PATH |
