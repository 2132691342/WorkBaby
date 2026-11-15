# 开发手册

架构与取舍见 `docs/ARCHITECTURE.md`；按功能排障见 `docs/MODULES.md`；契约见 `docs/API.md`。
编码规范（命名、分包、依赖方向、注释纪律、行数）以 `AGENTS.md` 为唯一权威，本文只讲怎么跑、怎么测、坏了怎么查。

## 1. 环境

| 需要 | 版本 | 说明 |
|---|---|---|
| Go | 1.26.x（`go.mod` 声明 `go 1.26.0`） | 编译后端与壳 |
| Node.js | ≥ 18（Vite 5 的最低要求） | **只用于构建前端**；运行时不随包分发 |
| wails3 CLI | 3.1.x | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |
| Task | 3.x | 任务入口（必装：`build/` 下的构建链依赖它） |
| NSIS | 3.x | 只在 `task package` 时需要（`choco install nsis` / `winget install NSIS.NSIS`），并把安装目录加进 PATH |
| WebView2 Runtime | Win10 1803+ 自带 | 渲染层；安装包里带引导程序，缺了会静默拉取 |

**全项目无 cgo**：SQLite 走 `glebarez/sqlite`，`CGO_ENABLED=0` 是构建任务的默认值，静态构建与交叉编译是交付前提。
运行时不需要机器上装 node / python / git——它们只是被探测并按需调用的外部工具。

## 2. 命令

```bash
go mod download
npm --prefix frontend install

task dev                 # wails3 dev：后端热重载 + Vite 热更（端口 9245）
task build               # 产出 bin\WorkBaby.exe（内部先跑 npm run build）
task run                 # 直接跑构建产物
task package             # NSIS 安装包（默认 INSTALL_SCOPE=user）
task test                # go test ./tests -count=1
task smoke               # 起纯后端实例打一遍全部只读接口（验接线）
task vet                 # go vet ./...
task fmt                 # gofmt -w backend main.go tests
task fmt:check           # gofmt -l 检查（CI 用）
task i18n                # npm --prefix frontend run check:i18n
task check               # fmt:check + vet + test + i18n（一条命令跑完本地门禁）
```

没有 Task 时的等价命令：

```bash
wails3 dev -config ./build/config.yml -port 9245
go build -o bin/WorkBaby.exe .
go build ./... && go vet ./... && go test ./tests -count=1
cd frontend && npx vue-tsc --noEmit -p tsconfig.json && npm run build
```

**提交前的四道门禁**（`AGENTS.md` §4）：`go build ./...`、`go vet ./...`、
`cd frontend && npx vue-tsc --noEmit -p tsconfig.json`、`go test ./tests -count=1`。
`npm run build` 串起三层：`vue-tsc --noEmit` → `node scripts/check-i18n.mjs` → `vite build`；
只想验文案时用 `npm run check:i18n`。

### 2.1 前端类型检查与文案门禁

- `vue-tsc --noEmit -p tsconfig.json`：`types/api*.ts` 与 Go 的 json tag 逐字段对齐，类型报错往往就是契约漂移。
- `check-i18n.mjs`：把 `src/locales/**/*.ts` 里每条字面量交给 `@intlify/message-compiler` 的 `baseCompile`，
  任一失败即退出码 1。当前规模是 1412 条文案（脚本自己在末尾打印条数）。
  `@` `%` `|` 是 ICU 保留字，裸写会让整条消息编译失败——**界面不报错、只是静默少一段**，
  正确写法是 `{'@'}`。门禁只覆盖字面量行，模板串与拼接串不在 locale 文件里。

### 2.2 纯浏览器调试前端

1. 起后端与 Vite：`pwsh scripts/dev.ps1`（脚本会打印实际端口与令牌）
2. 浏览器控制台写入脚本打印的那两行：

```js
localStorage.setItem('wb.dev.port', '<脚本打印的端口>')
localStorage.setItem('wb.dev.token', '<脚本打印的令牌>')
```

重启前端后 `api/wails.ts getEndpoint()` 走 `devOverride()`，直连 HTTP，不依赖 Wails 绑定。
后端装配失败时绑定调用同样抛错，所以这条覆盖必须显式写 localStorage，不靠「调用失败」去猜。

## 3. 目录与测试清单

```
main.go                    装配根：内嵌前端产物 + 版本号，交给 shell.Run
backend/{api,agent,domain,pkg,repo,service,shell,sse,web,index}
frontend/src/{api,components,composables,locales,router,stores,types,utils,views}
frontend/src/{main.ts,App.vue,i18n.ts}  前端入口与根组件
frontend/scripts/          前端构建期脚本（i18n 文案门禁）
assets/skills/             随包分发的推荐技能
build/{Taskfile.yml,config.yml,windows/{Taskfile.yml,nsis,icon.ico,info.json,wails.exe.manifest}}
scripts/                   开发期辅助脚本（冒烟、开发态起停）
tests/                     Go 场景测试，一文件一主题
```

`scripts/` 只放两个可复用的脚本，不堆一次性探针：

| 脚本 | 干什么 |
|---|---|
| `smoke.ps1` | 起一个纯后端实例，把全部只读接口打一遍，报告非 0 业务码；另验无令牌回 401。等价命令：`task smoke` |
| `dev.ps1` | 起后端 + Vite，打印后端端口与令牌，供浏览器直连前端调试 |

`tests/`（18 个文件，`go test ./tests -count=1` 数秒内跑完）：

| 文件 | 一句话 |
|---|---|
| `harness_test.go` | 公共装具：真 JSON 台账 + 真临时布局 + 真 gin 中间件栈的隔离后端 |
| `fixtures_test.go` | 共用脚手架：假上游（按脚本吐 SSE 分片）、帧捕获、文件助手 |
| `http_test.go` | 回环 HTTP 全链路：鉴权门 → CORS 预检 → 信封 → 落盘 → SSE 通道 |
| `chat_test.go` | 聊天运行时：两条队的语义、思考档进请求体、发送失败给可行动错误 |
| `chat_ask_test.go` | 提问卡全链路：模型提问 → 广播 → 应答 → 那一轮继续走 |
| `chat_context_test.go` | 上下文读数（首屏读数 / 水位提示）与压缩（手动 / 自动 / 无模型） |
| `models_test.go` | 模型清单整表对账、活跃指针被删即清、默认模型写入口、密钥掩码 |
| `stream_test.go` | 流式翻译器块表状态机：增量累加、工具块生命周期、权威覆盖、收尾与用量 |
| `engine_stream_test.go` | 引擎流式拼装：正文 / 思考 / 工具入参分片拼回，落盘与内存一致 |
| `engine_tools_test.go` | 引擎工具边界：工具轮次闭环、路径作用域、待办往返、计划模式闸门、输出折叠、占用估算 |
| `session_test.go` | 会话管线：回放健壮性、壳的写入纪律、落盘与回放格式契约、分叉 |
| `skills_test.go` | 推荐清单与磁盘互相咬合、技能生命周期整条可跑 |
| `mcp_test.go` | MCP 能力链路：握手与调用、进程死亡、开关驱动重建、清单并发与作用域遮蔽 |
| `mcp_fake_server_test.go` | 假 MCP 服务器：测试二进制带 `--wb-mcp-server` 时进服务器模式，自己拉起自己 |
| `git_test.go` | 冲突解决的三种结局（`git add` / 保留我方 / 保留对方）不能写反 |
| `index_test.go` | 预索引：收录边界、符号优先命中、频次加权排序 |
| `web_test.go` | 联网：地址安全（SSRF 拦截在接口层与工具层都要挡）、正文抽取、接口链路 |
| `proc_test.go` | 长驻子进程收尾：`Kill` 必回、幂等、已退出时立刻返回、遗言仍可读 |

## 4. 测试策略

**保留**：状态机与解析（会话回放/分支/隐藏轮、流式块表）、端到端长链路（鉴权 → 路由 → service → 落盘 → 信封）、
引擎能力边界（分片拼装、工具准入、输出折叠）、清单与落位（技能目录搬移、MCP 注册）、安全边界（路径、SSRF、git 冲突）。

**不写**：复述框架行为的用例、单 getter、纯函数往返、覆盖率驱动用例、依赖真实网络或真实上游的假绿用例。

依赖 Wails / WebView2 的模块不进测试；可测的纯逻辑抽到 `pkg/`，编排在 `service/`。
新加断言要说清「断言的是什么」，并带失败信息。

## 5. 排错表

日志位置：`%LOCALAPPDATA%\WorkBaby\logs\wb-YYYY-MM-DD.log`（开发态是 `%LOCALAPPDATA%\WorkBaby-dev\logs\`），
按天滚动，也可用 `GET /logs?lines=` 在界面里看尾部。发布版没有控制台，`pkg.Warn`/`pkg.Error` 是唯一留痕。

| 现象 | 先查哪 |
|---|---|
| 发送失败 | 日志 `WARN` 行的四个发送点：`聊天发送绑定会话失败` / `聊天发送取模型失败` / `聊天发送排队失败` / `聊天运行失败`；再对信封 `code`（3000/3001/5000/5002） |
| 模型下拉残留已删模型 | `GET /providers` 的 `models[]`；`POST /providers/upsert` 的 `models` 是否 `null`（非 nil 才整表对账），删行是否触发 `DropActiveIf` |
| MCP 配置不生效 | `mcp.json` 条目的 `enabled` 与 `command`（`url` 型不进引擎）；日志 `MCP 服务 %s 已装载 %d 个工具` / `装载失败` |
| 上下文水位恒 0 | `GET /chat/state` 的 `context_usage.context_window`（模型未填窗口则为 0）与 `usage_tokens`（无 usage 时按字符/4 估） |
| 界面没反应但后端说发了 | `docs/API.md` §4 的「后端不发 / 前端不接」两列；`stores/events.ts` 的 default 分支会 console.warn 一次 |
| i18n 门禁失败 | 该条文案里的裸 `@` `%` `|`；改成 `{'@'}` 这类转义后重跑 `task i18n` |
| 子进程挂起 / 关不掉 | `pkg/proc.go`：`Kill` 等 `Wait` 最多 3s（`killSettleBudget`）就放行；仍不退出说明孙进程脱离树，查 `KillTree` |
| 界面卡在「正在回答」 | 是否收到 `chat:settled`/`chat:done`；`/chat/stop` 是否被拒；`Stream.Finish` 是否被执行 |
| 索引搜不到 | `GET /index/status` 的 `index_built`；日志 `索引完成：工作区=… 文档=…`；重启后首次搜索会惰性重建 |
| git 操作没反应 | 写接口都回最新 `status`，先看 `code`；`1102` 是凭证（走 GCM），`1101` 看 `detail` 的 stderr |
| 窗口不出现 | `--hidden` 与前端 `Ready()` 是否被调用（首屏画完才调） |
| 装配失败 | 日志 `业务装配失败 [码]`；`Endpoint()` 20s 超时也算失败；再查数据目录可写性 |

**诊断工具陈旧**：静态诊断报「某符号未定义」而 `go build` 通过时先信编译器（`go build ./...` → `go vet ./...` → `gopls check <文件>`）。
已知五种陈旧来源（gopls 导出缓存、新文件未收录、gopls 与项目 Go 版本不一致、项目快照过期、上一轮结论文本缓存）及清理办法见 `AGENTS.md` §4.1。

## 6. 开发态与安装态

| | 开发态 | 安装态 |
|---|---|---|
| 判定 | exe 同级没有 `assets\skills`，且往上能找到同时含 `go.mod` 与 `assets` 的目录 | exe 同级有 `assets\skills` |
| 数据目录 | `%LOCALAPPDATA%\WorkBaby-dev` | `%LOCALAPPDATA%\WorkBaby` |
| 资产目录 | 仓库根的 `assets\` | exe 同级的 `assets\` |
| 前端 | Vite 伺服（端口 9245），产物落 `frontend/dist` 供 `go build` 内嵌 | 内嵌进 exe |
| WebView2 数据 | `data\webview` | 同左 |

仓库根也找不到时按安装态处理：资产取 exe 同级的 `assets`，数据目录不带 `-dev` 后缀。

数据目录完全隔离：删掉 `WorkBaby-dev` 等于恢复出厂。安装态的 WebView2 缓存也落在自己的数据目录，
卸载时能一并清掉（默认会落到 `%APPDATA%\<exe 名>`，安装器不认识它）。

## 7. 发布流程

```bash
task build      # git describe 取版本 → go build -tags production -trimpath -ldflags "-w -s -H windowsgui -X main.version=<ver>"
task package    # INSTALL_SCOPE=user（默认，装 %LOCALAPPDATA%，免 UAC）或 machine（Program Files）
```

- 版本号来源：`git describe --tags --always --dirty`（`build/windows/Taskfile.yml`）；NSIS 侧再取 `git describe --tags --abbrev=0`。
- `task build` 会先跑 `task common:go:mod:tidy` 与前端构建，然后生成 `.syso`（图标 + 版本资源）再链接，最后删掉 `.syso`。
- `task package` 依赖 `makensis`：缺了会直接报「缺 makensis」并给出安装命令，而不是 `executable file not found`。
- 安装包用 `wails3 generate webview2bootstrapper` 拉 WebView2 引导程序；`user` 作用域免 UAC。
- 产物：`bin\WorkBaby.exe`（便携）与 `build\windows\nsis` 下的安装器。

## 8. 常见改动落点

| 要改的东西 | 落点 |
|---|---|
| 新增一个 HTTP 接口 | `domain` 加 REQ/RESP → `service` 写业务 → `api/routes_*.go` 加 handler 并在 `server.go mount` 挂上 → `docs/API.md` 与 `docs/MODULES.md` 同步 |
| 新增一个工具 | `agent/tool_*.go` 实现 `Tool` → `NewRegistry` 或 `Add*` 装上 → `agent/policy.go` 登记只读/写分类（不登记按写处理） |
| 新增一个设置键 | `service/settings.go` 加常量（界面类默认值进 `defaultSettings`）→ `docs/DATA-MODEL.md` 设置键表同步 |
| 新增一个 SSE 事件 | 后端 `Hub.Broadcast` → `stores/events.ts routeFrame` 加分支 → `docs/API.md` §4 与 `docs/MODULES.md` §2 同步 |
| 新增一个路由页 | `frontend/src/router/index.ts` + `views/` + `locales/{zh-CN,en}/` 两套文案（键必须成对） |
| 新增一个错误码 | 先写进 `backend/pkg/codes.go` 与 `docs/API.md` §5，再在代码里用 |
