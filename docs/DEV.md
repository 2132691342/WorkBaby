# 开发

## 环境

| 需要 | 版本 | 说明 |
|---|---|---|
| Go | 1.26.x | 编译后端与壳 |
| Node.js | ≥ 20 | **只用于构建前端**；运行时不需要 node |
| wails3 CLI | 3.1.x | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | 3.x | 任务入口（可选，也可直接敲 go 命令） |
| NSIS | 3.x | 只在打安装包时需要（`choco install nsis`） |
| WebView2 Runtime | Win10 1803+ 自带 | Wails 渲染层 |

不需要预装 Python / Rust / 任何 C 编译链——**全项目无 cgo**，
SQLite 走的是纯 Go 驱动。

## 常用命令

```bash
go mod download
cd frontend && npm install && cd ..

task dev        # wails3 dev：后端热重载 + Vite 热更
task build      # 产出 bin\WorkBaby.exe
task run        # 直接跑构建产物
task test       # 场景测试（秒级）
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
复述框架行为）不写——它们的维护成本高于收益。**一文件一个被测对象**：
改哪个模块就看哪个文件，一处找全。

十五个文件、二十三个用例，全部跑完约 5 秒：

| 文件 | 入口 | 断言什么 |
|---|---|---|
| `tests/agent_test.go` | `TestEngineAssemblesStreamFragments` / `TestEngineToolBoundary` | 引擎核心链路：正文分片拼成完整回复、工具入参分片拼成合法 JSON、思考分片不丢、落盘与内存一致；工具的工作区边界（越界读不到）与文件工具往返（写 / 读 / 改 / 差异回传） |
| `tests/session_test.go` | `TestSessionReplay` / `TestSessionWriteDiscipline` / `TestSessionFormatContract` | 会话回放的健壮性（分支只走活动分支、残行不毁整份历史、环状父指针有界退出）与组装规则（工具结果回填宿主轮、孤立结果单独成轮、压缩标记、标题来源、计划模式只认最后一条）；写入纪律（隐藏只记台账不动文件、改名只追加、引擎正打开时拒绝且不动文件）；引擎落盘格式与回放层对齐（含会话目录编码一致） |
| `tests/chain_test.go` | `TestChain` | 回环 HTTP 全链路：鉴权硬门（401 且不带业务信封）、CORS 预检绕过鉴权门并带允许头、信封同构（成功 / 业务失败 / 未知路由 / 坏请求体全为 200 + `code`）、写入落盘（HTTP 写 → 直查存储文件 → HTTP 读回）、SSE 通道（查询参数鉴权、先补 `stream:open`、广播到达、seq 单调） |
| `tests/builtin_test.go` | `TestTodoRoundTrip` / `TestPlanModeGate` / `TestTidyFold` | 引擎内置能力：待办清单整份替换且随结果落盘、回放层能复原；计划模式闸门（写工具被拦、读工具放行、未登记工具默认按写处理、系统提示写明被拦的是哪些、退出后恢复自由）；工具输出折叠（短输出原样、长输出折成头尾加标记、策略关掉后原样） |
| `tests/stream_test.go` | `TestStreamBlockTable` | 流式翻译器的块表状态机：同类增量续写、异类另开一块、工具块从开始到收尾（输出 / 耗时 / 状态）、落盘消息权威覆盖但保留工具读数、用量帧与收尾帧、失败时清 ANSI 码并解除流式态 |
| `tests/skills_test.go` | `TestSkills` | 内置清单与磁盘资产互相咬合（少一边就是死条目、SKILL.md 头能解析出描述）；子进程环境不遮蔽 HOME；技能全生命周期（不自动落位、重复安装拒绝、不覆盖用户改动、开关 = 搬目录 + 记账、重置后仍是可读文件、删除清整个目录且目录路径拒绝、项目技能不受壳管辖） |
| `tests/governance_test.go` | `TestMcpConfig` | `mcp.json` 的并发读写纪律（12 路并发读-改-写一条不丢）与作用域遮蔽（删项目级条目不连带删被遮蔽的用户级条目） |
| `tests/fork_test.go` | `TestSessionFork` | 会话分叉的三条纪律：原会话一个字节不动、新会话只带到分叉点为止（分叉点之后的内容属于另一条思路）、不存在的起点与空起点直接拒 |
| `tests/guards_test.go` | `TestModelDefaultGuards` / `TestProviderKeyMasked` | 写入口的边界：默认模型的非法组合先拒（服务商不存在 / 模型不在该服务商下 / 半截组合 / 未知思考档）且被拒时不落盘、不动台账里的其他键；服务商密钥读出来恒为掩码，空串语义是「保留原值」 |
| `tests/index_test.go` | `TestIndexBuildFindsFilesAndSymbols` / `TestIndexSearchRanksSymbolFirst` | 预索引的收录边界（源文件与符号进、二进制与依赖目录不进）与排序规则（符号命中排在文件命中之前、常用近用的排更前） |
| `tests/web_test.go` | `TestWebAddressGuard` / `TestWebExtractReadable` / `TestWebEndpoints` | 联网能力：内网与本机地址被拦（环回 / 私网 / 链路本地 / CGNAT / 非 http 协议）；正文抽取留下内容、去掉脚本与导航、还原实体、跳转链接还原成真实地址；接口链路接上且接口层也挡住内网地址、设置能持久化、不认识的服务商被拒 |
| `tests/git_test.go` | `TestConflictResolve` | 用真 git 造一次真冲突，断言三个口径各自落成什么：`ours` 留当前分支、`theirs` 留合进来那方、两者都不许残留冲突标记且必须已暂存；空路径与未知口径直接拒。本机没 git 时跳过 |
| `tests/files_test.go` | `TestFilePreview` | 文件预览的三条分类边界：文本回正文、可预览二进制回 base64、认不出的二进制只给占位 |
| `tests/templates_test.go` | `TestTemplates` | 提示词模板：落盘回读正文不混 YAML 头、名字清洗成小写、斜杠命令带 prompt 来源与正文、删除幂等、清洗后为空的名拒保存 |
| `tests/harness_test.go` | — | 公共装具：满装后端 harness（真 JSON 存储 + 真 SQLite + 真 gin 中间件栈）与轻量 catalog harness、HTTP 调用与断言辅助 |

约定：

- 位置在仓库根 `tests/`，一文件一个被测对象
- 每个测试用 `t.TempDir()` 造独立布局，不碰真实数据目录与仓库资产
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`
- 需要真实网络或真实上游的路径不写假绿用例

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
| 发消息没反应 | 模型页有没有选服务商与模型；密钥是否填了；概览页的运行时状态 |
| git 面板说不可用 | `git --version` 能跑吗；壳用系统装的 git，不随包 |
| 技能不出现 | 是否在 `data\skills\`（不是 `skills_disabled\`）；SKILL.md 头是否合法 |
| 联网搜索报不通 | 设置页选的搜索服务有没有填密钥；免密钥的 DuckDuckGo 是默认 |
| 打安装包报 makensis 找不到 | 装 NSIS 并把安装目录加进 PATH |
| 前端改动没生效 | `task dev` 会并行跑两次构建；先停掉旧的 dev 进程再起 |

## 打包与发布

### 产物形态

安装态是**一个目录**，不是单文件：

```
WorkBaby\
├── WorkBaby.exe          # Go 编译的壳（前端已 embed 进 exe）
├── assets\
│   └── skills\           # 推荐技能包（目录）
└── 卸载程序等 NSIS 产物
```

前端产物经 `//go:embed all:frontend/dist` 编进 exe，**不额外分发静态文件**。
运行时不随包发：git / node / python 全部用系统已装的。

体积基线：`assets\skills` 约 0.8MB、exe 约 32MB。安装包体积应该在十几 MB 量级；
出现 100MB+ 说明有人把运行时 zip 又拷回了 `assets\`。

### 图标

全套图标由 `build\tools\make-icons.ps1` 生成（品牌色渐变砖 + W 字标 + 火花），
**用 Windows PowerShell 5.1 跑**（依赖 System.Drawing，pwsh 没有；文件带 BOM）：

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File build\tools\make-icons.ps1
```

一次产出四处，改设计只改脚本再跑一遍：

| 产物 | 用途 |
|---|---|
| `build\appicon.png` | 1024 主稿 |
| `build\windows\icon.ico` | exe 与安装器图标（16–256 多尺寸） |
| `backend\shell\res\icon.ico` | 托盘图标，`//go:embed` 进 exe，与上一份必须同步 |
| `frontend\public\favicon.ico` | 前端 favicon |

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
  该文件带中文文案，保存为 **UTF-8 with BOM**（NSIS 靠 BOM 识别编码），
  详见 `build/windows/nsis/README.md`
- 卸载时先问一句**是否连数据目录一起删**，默认保留——用户的会话与配置在
  `%LOCALAPPDATA%\WorkBaby`，删不删必须由用户自己决定

### CI

| 工作流 | 触发 | 做什么 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR → main | gofmt 检查 → `npm ci` → 生成前端绑定 → 前端类型检查与产物 → `go vet` → `go test ./tests` → `go build ./...` |
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
2. 模型页能新增一个服务商并保存
3. 对话页发一句话能流式回来
4. 技能页能停用一个、再启用回来（`skills\` 与 `skills_disabled\` 之间搬移）
5. 工作区页能看见文件与 git 状态
6. 关窗收托盘、托盘菜单能唤回
7. 卸载弹窗选「否」后 `%LOCALAPPDATA%\WorkBaby` 仍在；选「是」则连数据一起清空
