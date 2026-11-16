# 开发手册

架构与取舍见 `docs/ARCHITECTURE.md`；按功能排障见 `docs/MODULES.md`；契约见 `docs/API.md`；数据模型见 `docs/DATA-MODEL.md`。
编码规范、依赖方向、门禁口径以 `AGENTS.md` 为唯一权威，本文只讲怎么跑、怎么测、坏了怎么查。

## 1. 环境

| 需要 | 版本 | 说明 |
|---|---|---|
| Go | 1.26.x（`go.mod` 声明 `go 1.26.0`） | 编译后端与壳 |
| Node.js | ≥ 18（Vite 5 的最低要求） | **只用于构建前端**；运行时不随包分发 |
| wails3 CLI | 3.1.x | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | 3.x | 任务入口（必装：`build/` 下的构建链依赖它） |
| NSIS | 3.x | 只在 `task package` 时需要（`choco install nsis` / `winget install NSIS.NSIS`），并把安装目录加进 PATH |
| WebView2 Runtime | Win10 1803+ 自带 | 渲染层；安装包里带引导程序，缺了会静默拉取 |

**全项目无 cgo**：SQLite 走纯 Go 驱动，`CGO_ENABLED=0` 是构建任务的默认值，静态构建与交叉编译是交付前提。
运行时不需要机器上装 node / python / git——它们只是被探测并按需调用的外部工具。

## 2. 命令

任务全部收口在根 `Taskfile.yml`（include `common: ./build/Taskfile.yml` 与 `windows: ./build/windows/Taskfile.yml`）：

| 任务 | 作用 |
|---|---|
| `task dev` | wails3 dev：后端热重载 + Vite 热更（端口 9245，`WAILS_VITE_PORT` 可覆盖） |
| `task build` | 产出 `bin\WorkBaby.exe`：go mod tidy → 前端依赖 → 生成绑定 → 前端产物 → 生成 `.syso`（图标 + 版本资源）→ go build → 删 `.syso` |
| `task run` | 跑构建产物 |
| `task package` | 依赖 build，打成 NSIS 安装包（`INSTALL_SCOPE=user` 默认 / `machine`） |
| `task generate:bindings` | 生成前端 TypeScript 绑定（`-clean` 先清空输出目录，生成物目录跟着后端包位置走） |
| `task icons` | 重生成全套图标（`build/tools/make-icons.ps1`：`build/appicon.png` 与主稿参数 → 四处 ico/png 产物；产物提交进仓库，不进构建流水线） |
| `task test` | `go test ./tests -count=1` |
| `task vet` | `go vet ./...` |
| `task fmt` / `task fmt:check` | gofmt -w / gofmt -l 检查（后者 CI 用） |
| `task i18n` | 前端文案过 vue-i18n 编译器（见 §2.2） |
| `task ts:check` | 前端类型检查 `vue-tsc --noEmit -p tsconfig.json`（要先有生成绑定） |
| `task smoke` | 用发布产物验接线（见 §2.3） |
| `task check` | fmt:check + vet + test + ts:check + i18n，本地门禁一条命令跑完 |

### 2.1 依赖顺序：为什么 build 要先跑

- `main.go` 用 `//go:embed all:frontend/dist` 内嵌前端；`frontend\dist` 与生成绑定 `frontend\bindings` 都不进仓库。
- 一切编译 package main 的命令——`go vet` / `go test` / `go build`——都要求这两者先就位；全新检出先跑 `task build`（或 `task generate:bindings` + 前端 `npm run build`）。
- `vue-tsc` 与 vite 也 import 生成绑定，所以 `task ts:check` 同样排在绑定之后。
- `.github/workflows/ci.yml` 的步骤顺序就是这个道理：生成绑定 → `npm ci` → 前端类型检查与产物 → `go vet` → `go test` → 主机构建。

门禁口径见 `AGENTS.md` §4：四道门禁一条命令跑完，就是 `task check`。

### 2.2 文案门禁

`npm --prefix frontend run check:i18n`（即 `task i18n`）把 `src/locales/**/*.ts` 里每条字面量交给
`@intlify/message-compiler` 的 `baseCompile`，任一失败即退出码 1，脚本在末尾打印当前条数。

- `@` `%` `|` 是保留字，裸写会让整条消息编译失败——**界面不报错、只是静默少一段**，正确写法是 `{'@'}`。
- 门禁只覆盖 locale 文件里的字面量行；模板串与拼接串不在 locale 文件里。
- `npm run build` 串起三层：`vue-tsc --noEmit` → `node scripts/check-i18n.mjs` → `vite build`；只想验文案时用 `npm run check:i18n`。

### 2.3 开发态脚本

`scripts/` 只有三个脚本，不堆一次性探针：

| 脚本 | 干什么 |
|---|---|
| `backend.ps1` | dev.ps1 与 smoke.ps1 点源共用的起停助手：`Start-WorkBabyBackend` 起 `--hidden` 纯后端实例，端口是随机的，从按天日志「本次启动之后新增的行」里抓；`Stop-WorkBabyBackend` / `Stop-ProcessTree` 连根带子收尾（npm 会把 node 挂在 cmd 下面，只杀最外层会留孤儿） |
| `dev.ps1` | 起后端 + Vite，打印端口与令牌，供纯浏览器调试（见 §2.4） |
| `smoke.ps1` | 默认用 `bin\WorkBaby.exe` 起纯后端，只打免参数的只读接口并验无令牌回 401——验发布产物的接线，不验业务；带路径或 ID 的接口留给 tests 覆盖 |

三个脚本都是 UTF-8 **带 BOM**：`powershell`（5.1）读不到 BOM 会把中文按 ANSI 误读、整份解析失败，`pwsh`（7）两种都吃——改脚本保持带 BOM 保存（规则见 `AGENTS.md` §2.5）。已实测：`pwsh scripts/smoke.ps1` 全接口通，`pwsh scripts/dev.ps1 -BackendOnly` 能打出端口与令牌。

### 2.4 纯浏览器调试前端

1. 先 `task build`（脚本默认用 `bin\WorkBaby.exe`），再 `pwsh scripts/dev.ps1`
2. 浏览器控制台写入脚本打印的两行：

```js
localStorage.setItem('wb.dev.port', '<脚本打印的端口>')
localStorage.setItem('wb.dev.token', '<脚本打印的令牌>')
```

重启前端后 `api/wails.ts` 的 `devOverride()` 读这两个键直连 HTTP，不依赖 Wails 绑定。
后端装配失败时绑定调用同样抛错，所以这条覆盖必须显式写 localStorage，不靠「调用失败」去猜。

**令牌与 `WB_DEV_TOKEN`**：`scripts/backend.ps1` 起后端时把脚本的 `-Token`（`dev.ps1` 默认 `devtoken123`）
写进环境变量 `WB_DEV_TOKEN`，令牌在调试期间是固定值，不用每次贴随机串。它**只在开发态布局生效**
（`backend/shell/app.go` 的 `newToken`）：发布版一律每次启动随机 24 字节令牌，环境变量读都不读；
熵源不可用时直接拒启（panic），不会退化成可预测的令牌。接口鉴权头是 `X-Desktop-Token`，
只有 `/events` 一条路由收 `?token=` 查询参数（`backend/api/server.go` 的 `authMiddleWare`）。

安全注意：固定令牌等于把「每次启动随机」这道门换成常量——**任何知道值的本机进程都能打 API**。
只在自己的开发机上用，别把 `devtoken123` 这类值带进共享环境；调试完随手收掉这个环境变量。
开发态的会话、设置、密钥都落在 `WorkBaby-dev` 数据目录（`%LOCALAPPDATA%\WorkBaby-dev`，见 §6），
与安装态完全隔离——在开发态调试不会碰到真实用户数据，反之亦然。

## 3. 目录与测试清单

```
main.go                    装配根：内嵌前端产物 + 版本号，交给 shell.Run
backend/{api,agent,domain,pkg,repo,service,shell,sse,web,index}
frontend/src/{api,components,composables,locales,router,stores,types,utils,views}
frontend/scripts/          前端构建期脚本（i18n 文案门禁）
assets/skills/             随包分发的推荐技能
build/{Taskfile.yml,config.yml,tools/make-icons.ps1,windows/{Taskfile.yml,nsis,icon.ico,info.json,wails.exe.manifest}}
scripts/{backend.ps1,dev.ps1,smoke.ps1}   开发态起停与冒烟
tests/                     Go 场景测试，一文件一主题
```

`tests/` 现状：13 个文件、12 个入口函数（另有一个 `TestMain` 在 `mcp_fake_server_test.go`），全量数秒跑完。一文件一个主题，主题内的子契约用 `t.Run` 分层：

| 文件 | 入口 | 子契约 |
|---|---|---|
| `harness_test.go` | —（装具） | 真 JSON 台账 + 真临时布局 + 真 gin 中间件栈的隔离后端；按脚本吐分片的假上游；SSE 帧捕获；`catalogHarness`；`mustSetWorkspace` / `waitIndexBuilt` / `writeSession` 等跨主题助手 |
| `mcp_fake_server_test.go` | TestMain | 测试二进制带 `--wb-mcp-server` 时切入假 MCP 服务器模式（stdin 收 JSON-RPC、stdout 一行一帧），自起自己当上游 |
| `engine_test.go` | TestEngine / TestEngineGuard | TestEngine：分片拼装 / 上游错误原话透传 / 工具入参容错 / 工具轮次闭环 / 工具作用域 / 待办往返 / 计划模式闸门。TestEngineGuard（回归防线，同一文件）：分片重复头字段不清入参 / name 迟到最后一片参数不丢 / length 截断整批拒绝执行 / 落盘全文与上下文折叠分离 / 裁窗后孤儿配对修复 / 瞬时失败退避后成功 / 鉴权失败一票否决不重试 / 流内 error 帧限流重试 |
| `chat_test.go` | TestChatFlow / TestChatContext | TestChatFlow：队列语义 / 思考档接线 / 模型参数接线 / 中止交回排队话术 / 发送失败面 / 提问卡链路。TestChatContext（同一文件）：上下文读数 / 上下文压缩 / 流式块表 |
| `session_test.go` | TestSession | 回放 / 写入纪律 / 格式契约 / 分叉 |
| `models_test.go` | TestModel | 清单对账 / 默认模型写入口 / 密钥掩码 |
| `mcp_test.go` | TestMcp | 客户端 / 运行时装配 / 配置治理 |
| `skills_test.go` | TestSkills | 清单咬合 / 提示里的技能索引 / 生命周期 |
| `workspace_test.go` | TestWorkspace | 索引收录边界 / 索引排序 / 联网地址拦截 / 正文抽取 / 联网接口链路 |
| `index_lifecycle_test.go` | TestIndexLifecycle | Frecency 并发记账不丢不炸 / 检索出口并发命中 / 构建中检索回引擎忙 / 构建可取消（预取消透传 + 中途取消及时返回） / HTTP 重建后台化且立即受理 |
| `process_test.go` | TestProcess | 长驻进程收尾 / 子进程环境 / 冲突解决 |
| `backend_hardening_test.go` | TestBackendHardening | 设置写入口不接受工作区键 / 密钥不回显 / 当前会话删除先解绑再删 / 模板保存回清洗后的名字（含非法名拒绝与出参形状） / 路径守卫挡符号链接 / 请求体超限即拒 / SSE 慢订户被摘掉 / 排队出参恒为数组 |
| `http_test.go` | TestHTTPContract | 鉴权门 / CORS 预检 / 信封同构 / 写入落盘 / SSE 通道 |

定位方法：

```bash
go test ./tests -list '.*'                 # 列出全部入口函数
go test ./tests -run 'TestSession/分叉' -v  # 只跑一个子契约；-v 能看到主题→子契约的树
go test ./tests -count=1                   # 全量
```

- `-run` 用 `/` 分层下钻：`TestChatFlow/队列语义` 跑该组及其子层；子契约名可用中文。
- 失败输出的第一行就是 `--- FAIL: TestSession/分叉`，名字即定位。

## 4. 测试策略

每条都是可判定的取舍规则：满足「保留」任一条才写，满足「不写」任一条就不写。

保留——断点在任意一层都表现为「界面坏了」或「能力丢了」：

| 类别 | 判定 |
|---|---|
| 跨层链路 | 接口 → 编排 → 引擎 → 落盘 → 回放整条贯通（鉴权门、信封、SSE 通道、写入落盘） |
| 状态机 | 会话回放 / 分支 / 隐藏轮、流式块表——顺序是契约，坏了界面就错 |
| 能力边界 | 分片拼装 / 工具准入 / 输出折叠——模型能做什么、看到什么，取决于这些边界 |
| 清单与磁盘咬合 | 技能目录搬移与生命周期、MCP 注册表重建、模型整表对账——两边理解一旦分叉就是静默丢数据 |
| 并发与收尾 | Kill 幂等、超时预算、并发重建不打死注册表 |
| 安全边界 | 路径作用域、联网地址拦截、git 冲突口径、密钥掩码、鉴权门 |

不写：

- 单 getter / 复述框架行为 / 纯函数往返
- 覆盖率驱动的用例
- 依赖真实网络或真实上游的假绿用例（上游一律用按脚本吐分片的假服务）
- 依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`

结构规则——每主题一个入口函数 + `t.Run` 子契约：

- 入口是运行粒度：`go test ./tests -run 'TestModel'` 一次拿到该主题全部结论；全量也就数秒。
- 子契约是定位粒度：失败落在 `TestSession/分叉` 这一级，`-run` 挑着跑，不用在全量输出里翻。
- 装具只有 `harness_test.go` 一份，新主题复用，不在主题文件里各写一遍。
- 断言带失败信息，说清「什么该等于什么，实际是什么」；测试里碰外部程序走 `pkg.LookPath` / `pkg.RunProc`，不裸用 `os/exec`。

## 5. 排错表

按症状的完整排查路径见 `docs/TROUBLESHOOTING.md`（业务码逐个给下一步动作，含 `8003` 这类只存在于前端的码）；
本节只留开发期最常用的几条。

日志按天滚动：安装态 `%LOCALAPPDATA%\WorkBaby\logs\wb-YYYY-MM-DD.log`，开发态 `WorkBaby-dev`；
也可用 `GET /logs?lines=` 在界面里看尾部。发布版没有控制台，`pkg.Warn` / `pkg.Error` 是唯一留痕。

| 现象 | 先查哪 |
|---|---|
| 发送失败 | 日志四个发送点：`聊天发送绑定会话失败` / `聊天发送取模型失败` / `聊天发送排队失败` / `聊天运行失败`；信封 code 对照：5000 未绑定、3000/3001 服务商与模型、5002 引擎占用 |
| 模型下拉残留已删模型 | `GET /providers` 的 `models[]`；`POST /providers/upsert` 的 `models` 是否 `null`（非 nil 才整表对账）；删行是否触发 `DropActiveIf` |
| MCP 配置不生效 | `mcp.json` 条目的 `enabled` 与 `command`（url 型只进配置清单，不进引擎）；日志 `MCP 服务 %s 已装载 %d 个工具` / `装载失败` |
| 上下文水位恒 0 | `GET /chat/state` 的 `context_usage.context_window`（模型没填窗口则为 0，只有读数没有水位）；`usage_tokens` 无 usage 时按字符 /4 估 |
| 界面没反应但后端说发了 | `docs/API.md` §4 的「后端不发 / 前端不接」两列；`stores/events.ts` 的 default 分支会对未接管事件 console.warn 一次 |
| 界面卡在「正在回答」 | 是否收到 `chat:settled` / `chat:done`；`/chat/stop` 是否被拒；`Stream.Finish` 是否被执行 |
| i18n 门禁失败 | 该条文案里的裸 `@` `%` `\|`；改成 `{'@'}` 这类转义后重跑 `task i18n` |
| 子进程挂起 / 关不掉 | `pkg/proc.go`：`Kill` 等 `Wait` 最多 3s（`killSettleBudget`）就放行；仍不退出说明孙进程脱离树，查 `KillTree` |
| 索引搜不到 | `GET /index/status` 的 `index_built`；日志 `索引完成：工作区=… 文档=…`；切工作区后首次搜索会现建（`EnsureIndexed`） |
| git 操作没反应 | 写接口都回最新 `status`，先看 `code`；`1102` 是凭证（走 GCM，落 Windows 凭据管理器），`1101` 看 `detail` 的 stderr |
| 窗口不出现 | `--hidden` 与前端 `Ready()` 是否被调用（首屏画完才调） |
| 装配失败 | 日志 `业务装配失败 [码]`；端点等待 20s 超时也算失败；再查数据目录可写性 |

**诊断工具陈旧**：静态诊断报「某符号未定义」而 `go build` 通过时先信编译器。
三种已知陈旧来源与清理办法见 `AGENTS.md` §4.1；重建命令用项目同版本：
`GOTOOLCHAIN=go1.26.0 go install golang.org/x/tools/gopls@latest`。

## 6. 开发态与安装态

判定在 `backend/shell/app.go::resolveLayout`，按 exe 同级有没有 `assets\skills` 一眼判定：

| | 开发态 | 安装态 |
|---|---|---|
| 判定 | exe 同级没有 `assets\skills`，且从工作目录往上（≤ 8 层）找到同时含 `go.mod` 与 `assets` 的仓库根 | exe 同级有 `assets\skills` |
| 数据目录 | `%LOCALAPPDATA%\WorkBaby-dev` | `%LOCALAPPDATA%\WorkBaby` |
| 资产目录 | 仓库根 `assets\` | exe 同级 `assets\` |
| 前端 | Vite 伺服（端口 9245），产物落 `frontend/dist` 供内嵌 | 内嵌进 exe |
| WebView2 数据 | `data\webview` | 同左 |

两边都定位不到时按安装态兜底（数据目录不带 `-dev`，资产取 exe 同级）；资产缺失不影响启动——它只是内置技能包。

数据目录完全隔离，删掉 `WorkBaby-dev` 等于恢复出厂。目录一览：

| 内容 | 说明 |
|---|---|
| `workbaby.json` | 设置台账（原子整写 + 互斥锁） |
| `workbaby.db` | SQLite：服务商与模型目录（**含 API 密钥原文**），以及派生检索索引 `search_doc`。会话正文不在这里；删掉这个文件等于连带删掉全部服务商密钥，必须重新填写 |
| `workbaby.db-wal` / `workbaby.db-shm` | 同一条 SQLite 连接的 WAL 与共享内存文件；备份与排障时与 `.db` 一起看待，单独挪走 `.db` 会留下无法回放的 WAL |
| `mcp.json` | MCP 服务清单 |
| `logs/` | 按天日志 |
| `sessions/` | 会话 JSONL（唯一持久真相） |
| `skills/` `skills_disabled/` | 启用态与停用态技能区 |
| `templates/` | 提示词模板 |
| `webview/` | WebView2 用户数据（`WebviewUserDataPath`，`shell/app.go`） |

## 7. 发布流程

发布由 push `v*` tag 触发（`.github/workflows/release.yml`），全程在 Windows Runner：

1. `fetch-depth: 0` 全量拉取——版本号来自 git describe，浅克隆拿不到 tag
2. 校验 tag 与 HEAD 一致：`git describe --tags --exact-match HEAD` 必须等于 tag 名，不等就失败（防止在旧提交上重打 tag 发错版本）
3. 装 NSIS 与 wails3 → `npm ci` → 生成绑定 → `npm run build` → `go test ./tests -count=1`
4. `wails3 package` 打 NSIS 安装器，收集 `bin\*installer*.exe`
5. 发布 Release：重打的 tag 撞上已存在的 Release 就 `--clobber` 替换资产，没有就 `gh release create --generate-notes`

本地发布命令：

```bash
task build      # git describe 取版本 → -tags production -trimpath -ldflags "-w -s -H windowsgui -X main.version=<ver>"
task package    # INSTALL_SCOPE=user（默认，装 %LOCALAPPDATA%，免 UAC）或 machine（Program Files）
```

- 版本号来源：`git describe --tags --always --dirty`（`build/windows/Taskfile.yml`）；NSIS 侧另取 `git describe --tags --abbrev=0` 去掉 `v` 前缀。
- `task package` 依赖 `makensis`：缺了直接报「缺 makensis」并给出安装命令，而不是 `executable file not found`。
- 安装器先由 `wails3 generate webview2bootstrapper` 产出 WebView2 引导程序；本机没有运行时则静默拉取。
- `build/windows/info.json` 必须写字面值——模板串会原样进 exe 的版本资源。
- 卸载时安装器会先问是否同时删用户数据（`%LOCALAPPDATA%\WorkBaby`），`data\webview` 则无条件清掉；不问就会像落在 `%APPDATA%\<exe 名>` 的缓存那样变成没人认领的残留。

## 8. 常见改动落点

| 要改的东西 | 落点 |
|---|---|
| 新增一个 HTTP 接口 | `domain` 加 REQ/RESP → `service` 写业务 → `api/routes_*.go` 加 handler 并在 `server.go::mount` 挂上 → `docs/API.md` 与 `docs/MODULES.md` 同步 |
| 新增一个工具 | `agent/tool_*.go` 实现 `Tool` → `NewRegistry` 或 `Add*` 装上 → `agent/policy.go` 登记只读/写分类（未登记按写处理） |
| 新增一个设置键 | `service/settings.go` 加常量（界面类默认值进 `defaultSettings`）→ `docs/DATA-MODEL.md` 同步 |
| 新增一个 SSE 事件 | 后端 `Hub.Broadcast` → `stores/events.ts::routeFrame` 加分支 → `docs/API.md` §4 与 `docs/MODULES.md` 同步 |
| 新增一个路由页 | `frontend/src/router/index.ts` + `views/` + `locales/{zh-CN,en}/` 两套文案（键必须成对） |
| 新增一个错误码 | 先写进 `backend/pkg/codes.go` 与 `docs/API.md` 的错误码表，再在代码里用 |
| 新增一个测试主题 | `tests/` 一文件一主题，装具复用 `harness_test.go`；改任务入口同步 `Taskfile.yml` 与本文 |
