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

不需要预装 Python / Rust / 任何 C 编译链——**全项目无 cgo**，
壳的持久化就是标准库读写一个 JSON 文件。

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
复述框架行为）不写——它们的维护成本高于收益。**一文件一个被测对象、一文件一个
测试入口**：改哪个模块就看哪个文件，一处找全。

七个测试入口 + 一个 live 入口，全部跑完约 1 秒：

| 文件 | 入口 | 断言什么 |
|---|---|---|
| `tests/chain_test.go` | `TestChain` | 回环 HTTP 全链路：鉴权硬门（401 且不带业务信封）、CORS 预检绕过鉴权门并带允许头（WebView2 虚拟源跨源调回环的前提）、信封同构（成功 / 业务失败 / 未知路由 / 坏请求体全为 200 + `code`）、写入落盘（HTTP 写 → 直查存储文件 → HTTP 读回）、SSE 通道（查询参数鉴权、先补 `stream:open`、广播到达、seq 单调） |
| `tests/frames_test.go` | `TestFrames` | 流式翻译器的块表状态机：增量按块累加、工具块从出现到收尾、message_end 权威覆盖但按 id 保留工具读数、user 话术更新、用量透传 turn_end、stopReason=error 收尾带原话、settled 透传 abort、未知事件静默 no-op |
| `tests/session_test.go` | `TestSession` | 会话 JSONL 回放与写入纪律：分支只走当前分支、残行不毁整份历史、环状 parentId 有界退出、工具结果回填宿主轮、孤立结果单独成轮、压缩标记、标题来源；隐藏轮只记台账不改文件、改名只末尾追加、引擎正打开时拒绝 |
| `tests/sandbox_test.go` | `TestSandbox` | 沙箱三层契约：内置清单与磁盘资产互相咬合（18 个插件逐条对版本、22 个技能双向无死条目、frontmatter 块标量不破）、子进程环境把 HOME / USERPROFILE / npm 与 pip 缓存钉进沙箱、技能落位/开关/重置/删除/自愈整条生命周期 |
| `tests/plugin_test.go` | `TestPlugins` | 插件治理：来源串规范（git 源就是 URL 本身，不带 `git:` 前缀）、停用写 `autoload:false` 启用写裸串、开关保留用户配的资源过滤、重试不静默打开停用的插件、卸载撤声明删包目录、版本漂移只碰「在且版本不对」的包 |
| `tests/mcp_test.go` | `TestMcpConfig` | 12 路并发写 mcp.json 一条不丢（读-改-写必须有锁）；删项目级条目不连带删被它遮蔽的用户级条目 |
| `tests/harness_test.go` | — | 公共装具：全后端 harness（真 JSON 存储 + 真 gin 中间件栈）与轻量 harness、HTTP 调用与断言辅助 |
| `tests/sandbox_live_test.go` | `TestSandboxChain` | 真实链路：解压三个运行时 → 内置 npm 装 pi → RPC `get_state` / `get_commands` / `get_session_stats` → 优雅退出。带 `live` 构建标签，默认不跑（需联网，约 2 分钟） |

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

## 打包与发布

### 产物形态

安装态是**一个目录**，不是单文件：

```
WorkBaby\
├── WorkBaby.exe          # Go 编译的壳（前端已 embed 进 exe）
├── assets\
│   ├── runtime\          # node / PortableGit / Python 三个 zip
│   └── skills\           # 内置技能包（22 个目录）
└── 卸载程序等 NSIS 产物
```

前端产物经 `//go:embed all:frontend/dist` 编进 exe，**不额外分发静态文件**。
运行时 zip 走 `assets\runtime\`，首启解压到数据目录——不预解压进安装包，
否则安装体积翻倍，且用户要重装才能升级运行时。

体积基线：`assets\runtime` 约 110MB、`assets\skills` 约 8MB、exe 约 32MB。
安装包体积随 NSIS 压缩率浮动，**只有十几 MB 一定是 assets 没打进去**。

### 版本号

```bash
go build -ldflags "-X main.version=1.2.3" -o bin/WorkBaby.exe .
```

版本跟着 git tag 走；不注入时是 `dev`，本地跑出来的就是开发版，
不会误当成正式版。前端从 `Endpoint().version` 读到它显示在关于页。

### 打包命令与安装器要点

```bash
task build      # 只产 exe，用于本机验证
task package    # NSIS 安装包，per-user 安装、可选安装目录
```

`task package` 内部走 `build/windows/Taskfile.yml`，前置是 `makensis` 在 PATH 上。

- **per-user 安装**，不要求管理员权限（数据目录本来就在 `%LOCALAPPDATA%`）
- 允许用户改安装目录
- **必须把 `assets/` 一起装进去**：`build/windows/nsis/project.nsi` 是手维护文件，
  打包段里有 `File /r assets`；少装 assets 等于装了一堆不能跑的文件。
  该文件只能写 ASCII（NSIS 按 ANSI 代码页读脚本），详见 `build/windows/nsis/README.md`
- 卸载不删数据目录——用户的会话与配置在 `%LOCALAPPDATA%\WorkBaby`

### CI

| 工作流 | 触发 | 做什么 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR → main | gofmt 检查 → `npm ci` → 前端类型检查与产物 → `go vet` → `go test ./tests` → `go build ./...` |
| `.github/workflows/release.yml` | push tag `v*` | 装 wails3 与 NSIS → 校验 tag 与 HEAD 一致 → `npm ci` → 前端产物 → `go test` → `wails3 package` → 上传安装包到 Release |

两个工作流都需要 `windows-latest`（Wails3 的 WebView2 胶水只在 Windows 上编），
且**前端产物必须先于任何 `package main` 的编译**——`main.go` 的 `go:embed`
在全新 checkout 下指向不存在的目录，`vet` / `test` / `build` 都会失败。

发布流需要两样在 `windows-latest` 上不预装的东西：**wails3**（`go install` 到
`GOPATH\bin`，要手动加进 PATH）与 **NSIS**（`choco install nsis` 后把安装目录加进 PATH）。

重新打同一个 tag 时，Release 已存在，工作流会 `gh release upload --clobber`
替换资产而不是报错。

### 发布前自查

```bash
gofmt -l backend main.go tests   # 应无输出
go vet ./...                     # 应无输出
go test ./tests -count=1         # 全绿
task build                       # exe 产出并可启动
```

安装后验证清单：

1. 首次启动窗口按时出现（不黑屏、不假死）
2. 概览页进度条走完：运行时 → 技能 → 插件
3. 设置页诊断：node / git / python / pi 四个版本号都有值
4. 对话页发一句话能流式回来
5. 插件页能停用一个、再启用回来
6. 技能页能停用一个、再启用回来（`skills\` 与 `skills_disabled\` 之间搬移）
7. 关窗收托盘、托盘菜单能唤回
8. 卸载后 `%LOCALAPPDATA%\WorkBaby` 仍在（数据未丢）
