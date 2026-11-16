# 架构：为什么这样搭

WorkBaby 是 Windows 单机 AI 工作台：一个 Go 进程里同时装桌面壳、HTTP 服务、智能体引擎与持久化。
本文回答四件事：怎么搭的、为什么这样搭、换来什么代价、并发与故障下的不变量是什么。
按功能排障看 `docs/MODULES.md`，接口与事件全表看 `docs/API.md`，落盘形状看 `docs/DATA-MODEL.md`，命令与环境看 `docs/DEV.md`。

本文的每个大节按同一骨架写：**一句话定位** → **设计**（怎么搭的、为什么）→ **实现**（具体链路与关键符号）→ **取舍**（优势与代价）→ **坑**（不变量与易错点）。
正文里的 `文件:行` 都相对仓库根；行号只用来定位，语义以符号名为准。

---

## 1. 进程拓扑

**只有一个进程、一个端口、一套令牌。** 桌面壳、HTTP 服务、引擎、持久化全在同一地址空间里；
外部程序只有 `git.exe`、`powershell` 和用户配置的 MCP 服务，它们都是子进程。

### 设计

- **引擎在进程内**（`backend/agent/`）。引擎拆成独立进程要多出拉起、握手、崩溃重启、版本探测、状态同步五条失败链路，
  换来的只有进程隔离。进程内实现让会话、工具注册表、设置直接共享内存，跨模块调用是函数调用而不是 RPC。
- **业务全走回环 HTTP**，Wails 绑定面只承载系统能力（端点回填、目录/文件选择框、窗控、关窗决策、外链打开）。
  HTTP 可以 `curl`、可以脱离壳调试、可以在测试里直接打路由（`api.Server.Handler`，`backend/api/server.go:82`）。
  绑定面一旦承载业务，测试就只能起整个壳。
- **SSE 与业务接口同端口同进程**（`GET /api/v1/events`，`backend/api/server.go:267`）。事件与状态是同一份事实的两个视图，
  拆成两个端口就要多维护一套鉴权、一套生命周期、一条跨端口的顺序问题。
- **无边框窗口**（`Frameless: true`，`backend/shell/app.go:102`）：标题栏由前端自绘，窗控走绑定面
  （`WindowMinimize`/`WindowToggleMaximize`/`WindowClose`，`backend/shell/window.go:118-137`）。
  窗口初始 `Hidden`（`backend/shell/app.go:108`），首屏画完由前端调 `Ready()` 显形，避免一帧白屏。

### 实现

```
┌────────────────────── WorkBaby.exe（单一 Go 进程） ──────────────────────┐
│ main.go            内嵌 frontend/dist + 注入版本号，交给 shell.Run       │
│ shell/             Wails 应用 · 单实例 · 无边框窗口 · 托盘 · 绑定面       │
│   └─ 起─► api/     gin @ 127.0.0.1:<随机端口>  /api/v1/*  与  /events     │
│           service/ 业务编排（chat/model/skill/mcp/file/git/web/search…）  │
│           agent/   引擎：轮次循环 · 模型适配 · 工具注册表 · 会话落盘        │
│           web/ index/ sse/   联网 · 预索引 · 事件广播（能力包）            │
│           repo/    JSON 设置台账 + SQLite（三张表）                        │
│           domain/ pkg/       纯类型 · 叶子工具                             │
│                    │ 子进程：argv 直传、隐藏窗口、超时杀树                 │
│                    ▼                                                       │
│            git.exe · powershell · 用户配置的 MCP 服务                      │
│ WebView2 渲染 Vue 3 前端：只经 /api/v1 与 window.wails 绑定                │
└────────────────────────────────────────────────────────────────────────────┘
```

- **端口怎么挑**：`net.Listen("tcp", "127.0.0.1:0")` 让操作系统分配空闲端口，实际端口从 listener 读回
  （`backend/api/server.go:60-66`）。`Server.Port()` 未启动时为 0。写超时故意不设——SSE 长连接会被写超时掐断（`backend/api/server.go:67-71`）。
- **令牌怎么发**：`newToken()` 先看环境变量 `WB_DEV_TOKEN`，非空就用它（只服务本地调试与 `scripts/backend.ps1`）；
  否则取 24 字节随机数转 hex，随机源失败时退化为 `t-<纳秒时间戳>`（`backend/shell/app.go:313-322`）。
- **令牌怎么到前端**：前端调绑定面 `Endpoint()`，它先等装配完成（上限 20 秒，超时就把失败原话抛给界面），
  返回 `{wb_port, wb_token, wb_version, wb_contract, wb_base_url}`（`backend/shell/app.go:258-275`）。
  axios 实例把令牌写进每个请求的 `X-Desktop-Token` 头（`frontend/src/api/http.ts:73-75`）；
  `EventSource` 带不了自定义头，所以令牌走查询参数（`streamURL()`，`frontend/src/api/http.ts:108-111`）。
  服务端两条都收：先看头，空了再看 `?token=`（`backend/api/server.go:138-151`）。
- **门禁的真实组成**：服务只绑 `127.0.0.1`，令牌每次启动随机；`Origin` 白名单只是让浏览器放行，
  不是门禁（`originAllowed` 放行 `wails.localhost` 与 `localhost`/`127.0.0.1` 任意端口，`backend/api/server.go:123-135`）。
- **子进程出口**：命令工具走 `powershell -NoProfile -NonInteractive -Command <整条语句>`，argv 直传（`backend/agent/tool_shell.go:45-56`）；
  git 走 `pkg.LookPath("git")` + `pkg.RunProc`（`backend/service/git.go:40-57`）；MCP 服务走 `pkg.StartPersistent`（`backend/agent/mcp_client.go:64`）。
  三个出口都带隐藏窗口属性（`HiddenProcAttr`，`backend/pkg/proc.go:289`）。

### 取舍

- 换来：一条链路排到底（一个进程、一个端口、一份日志目录）；测试可以直接打路由，不必起壳；跨模块零序列化成本。
- 付出：引擎的 panic 会终止整个应用。HTTP 请求协程有 `gin.Recovery()` 兜住（`backend/api/server.go:55`），
  runner 协程没有兜底——引擎代码因此必须只在可取消的边界上做重活，外部程序全部经 `pkg/proc` 带超时杀树。
- 付出：令牌与端口每次随机，外部程序没法稳定连上来。要对外提供服务就得另开一层，那是架构变更而不是配置项。

### 坑

- `Endpoint()` 在服务没起来时返回 error，而不是端口 0：前端把 error 原话显示成启动失败原因（`frontend/src/stores/app.ts:76-77,99-101`）。
- WebView2 的页面源是虚拟域，对回环服务而言永远是跨源：所有请求都带 `Origin`，预检不带令牌，所以 CORS 必须排在鉴权之前（§6）。
- 单实例标识 `com.workbaby.desktop`（`backend/shell/app.go:43`）：第二次启动在 `application.New` 内部完成「通知首个实例 → 退出」，
  走不到建窗与装配，因此第二个进程不会重复占端口和数据目录。

---

## 2. 分层与依赖方向

**判定标准只有一条：下层不知道上层存在。** 这条标准是可验证的——依赖边就是下面那张表，多一条都能当场查出来。

### 设计

- 依赖箭头单向向下：`main → shell → api → service → {repo, agent, web, index, sse} → pkg/domain`。
- 每层各管一件事，越往下越没有业务判断：`domain` 只有类型与 JSON tag，`pkg` 只有叶子工具，
  `repo` 只管存取，`agent` 只管轮次循环，`service` 才装业务规则，`api` 只解析与转发，`shell` 只碰窗口与托盘。
- 组合根是一处而不是两处：`main.go` 只 import `pkg` 与 `shell`（`main.go:6-12`），
  开台账、开 SQLite、造 hub、发令牌、起 gin 全在 `shell.assemble` 里（`backend/shell/app.go:227-254`）。
  `shell` 对 `api` 只用四个符号：`api.New`、`Start`、`Port`、`Stop`（`backend/shell/app.go:248-249,269,273`、`backend/shell/window.go:206`），
  不碰任何 handler，因此「shell 不承载业务」是可核对的事实而不是口号。

### 实现

下表是 `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./backend/...` 的实际结果（内部包依赖，已删掉第三方与标准库）：

| 包 | 实际依赖的内部包 | 这一层是什么 |
|---|---|---|
| `pkg` | 无 | 叶子工具：`apperror`/`codes`/`ulid`/`fsutil`/`proc`/`paths`/`log` |
| `domain` | 无 | 纯类型包：VO/REQ/RESP/DTO/DO + 错误码常量，契约唯一落点 |
| `index` | 无 | 预索引与模糊检索算法 |
| `sse` | `domain` | 事件广播：`seq` 发号、断线补帧、心跳（只为 `EngineTag` 引 `domain`，`backend/sse/hub.go:150-156`） |
| `web` | `pkg` | 联网能力包：搜索适配器、正文抽取、地址安全 |
| `repo` | `pkg`、`domain` | JSON 设置台账 + SQLite 三张表 |
| `agent` | `pkg`、`domain`、`web` | 引擎：轮次循环、模型适配、工具注册表、会话落盘 |
| `service/parser` | `pkg`、`domain` | 会话 JSONL → 条目树 / 活动分支 / 待办 / 摘要的只读派生 |
| `service/platform` | `pkg` | Windows 专属动作：注册表自启、资源管理器定位、默认程序打开 |
| `service` | `agent`、`domain`、`index`、`pkg`、`repo`、`sse`、`web`、`service/parser`、`service/platform` | 业务编排与规则 |
| `api` | `domain`、`pkg`、`service`、`sse` | gin 薄壳：解析入参、调 service、回信封 |
| `shell` | `api`、`domain`、`pkg`、`repo`、`service`、`sse` | Wails 外壳与装配 |
| 前端 | 自身 + `/api/v1` + `@wails` 绑定 | 界面与状态，无文件系统与子进程能力 |

可核对的反向缺失（这些边一条都没有）：`agent` 不 import `service`/`api`；`repo` 不 import `service`；`api` 不 import `repo`；
`web`/`index`/`sse` 不 import `agent`/`service`；`pkg`/`domain`/`index` 不 import 任何本项目内部包。
联网工具直接调 `web`，是因为 `agent` 与 `web` 的边只用到 `web.SearchResult` 这一个类型（`backend/agent/tool_web.go:15`）。

### 取舍

- 换来：`agent`、`index`、`service/parser`、`pkg` 能脱离壳单测（`tests/` 里直接构造它们，不引 Wails）；
  换 HTTP 框架、换界面技术只动 `api`/`shell`/`frontend`。
- 代价 1：`pkg` 引了 gin——信封出口 `pkg/resp.go` 的参数是 `*gin.Context`，所以「叶子工具包」不是零框架依赖。
  写 `pkg` 的用例时要带上 gin 上下文，这是它唯一的额外成本。
- 代价 2：`service` 是扇入最大的包，装配顺序与接线全在 `service.New` 一处（§3）。好处是接线一眼看完，
  坏处是这个文件是全局改动的高频冲突点，因此它被按职责拆成多个 service 文件，装配点只留一个。
- 代价 3：`domain` 是全局共享的单一契约文件集，动它等于同时动所有层的编译。字段前缀与错误码段位因此必须先在文档里定（`AGENTS.md` §2.3/§2.4）。

### 坑

- 依赖方向没有构建期强制手段（没有 import 守卫测试），它靠两条纪律：新增内部 import 必须过评审，以及上表是实测的——
  改错了 `go list` 立刻能看出来。
- `service/platform` 与 `service/parser` 是子包不是独立层：它们的依赖集合比 `service` 更小，因此可以单独测。

---

## 3. 装配顺序

**启动只做「界面能说话」必需的事，其余全在后台。** 顺序不是历史包袱，是每一步的失败面决定的。

### 设计

- **目录布局先于一切**：数据目录不可写必须弹框告诉用户，而 WebView2 的缓存路径要在 `application.New` 时就交进去
  （`backend/shell/app.go:50-61`）。先解析布局、再设日志目录，之后每一步失败才有地方说话。
- **窗口先建后显**：建窗时 `Hidden`，前端首屏画完调 `Ready()` 才显形（`backend/shell/app.go:108`、`backend/shell/window.go:13-33`）。
  装配期显形会让用户看到一帧白屏。
- **托盘在装配之前**：用户可能一启动就把窗口关进托盘，此时装配还没完成，`closeAction()` 要能回答（默认收托盘，`backend/shell/app.go:294-300`）。
- **HTTP 先于重活**：端口在 `assemble` 末尾就绪；会话初始化、MCP 子进程握手、工作区索引全部推到后台（见「实现」末条）。
- **装配失败不重试也不吞**：`startup` 把失败原因记进 `startErr`，`Endpoint()` 把原话抛给前端，界面显示一句能读的话
  （`backend/shell/app.go:212-224,278-292`）。

### 实现

外壳段（`shell.Run`，`backend/shell/app.go:50-141`）：

1. `resolveLayout`：exe 同级有 `assets\skills` 判为安装态；否则从当前目录往上找带 `go.mod` 与 `assets` 的仓库根，判为开发态，
   数据目录名变成 `WorkBaby-dev`，两套数据互不污染（`backend/shell/app.go:149-177`、`backend/pkg/paths.go:36-58`、`backend/shell/endpoint.go:35-50`）。
2. `pkg.SetLogDir(layout.LogDir)`。
3. `application.New`：注册服务、内嵌前端资源、单实例 `com.workbaby.desktop`、`WebviewUserDataPath` 指到 `data\webview`
   （不指过去的话，那几十 MB 缓存会落在 `%APPDATA%` 下，卸载器不认识它）。
4. 建窗：尺寸按主屏缩放比放大后声明（v3 把 `Width/Height` 当物理像素，`backend/shell/app.go:143-147`、`backend/shell/windows.go:17`）、
   `Frameless`、`Hidden`、背景色 `248,250,252`。
5. 关窗钩子：`CloseActionQuit` 走退出，否则收托盘；首次收托盘先问一次「收进托盘还是退出」（`backend/shell/app.go:94-112`）。
6. 建托盘与托盘菜单（「最近工作目录」子菜单随设置重建，`backend/shell/tray.go:22-51,83-88`）。
7. Wails 生命周期钩子 `ServiceStartup` → `startup` → `assemble`。

业务段（`assemble`，`backend/shell/app.go:227-254`），顺序是硬约束：

1. `repo.Open(layout.StorePath)` 开 JSON 台账 → 2. `repo.OpenDB(layout.DatabasePath())` 开 SQLite 并迁移三张表
→ 3. `sse.New(1024)` 建广播中心 → 4. `newToken()` 发令牌 → 5. `service.New(...)` 装配业务
→ 6. `api.New(svc, hub, token)` + `srv.Start()` 起监听。

`service.New` 的接线（`backend/service/app.go:71-120`）：
`Settings.EnsureDefaults` → `NewGit` 并回填 `settings.Git`（工作区行要带分支与改动数）→ `NewSessions` → `NewModels`
→ `NewWeb` → `NewSearcher` → `agent.NewStore(data\sessions, settings.Workspace)` → `NewTemplates`
→ `NewChat`（造工具注册表 + 准入策略 + 待答提问表）→ `chat.Templates` → `chat.Tools.AddWebTools` / `AddIndexTool`
→ `newMcpRuntime` + `NewMcp` + `go mcpRuntime.Rebuild()` → `sessions.Active = chat.CurrentFile` → 组装 `App`（含 `NewFiles`、`NewSkills`）。

- 联网与检索工具**无条件装上**：装配时不看「搜网配好了没」，只看运行时调用能否成功（默认搜索适配器 `duckduckgo`，`backend/service/web.go:43-53`）。
  工具清单从注册表现读，因此系统提示与实际可调用工具永远一致（`backend/agent/prompt.go:10-21`）。
- `sessions.Active` 必须排在 `NewChat` 之后：改名要避让引擎正在写的那份会话文件（`backend/service/sessions.go:132-136`）。
- 两处重物在后台：`go a.svc.Chat.Warm()`（`backend/shell/app.go:223`，绑定或新建会话，`backend/service/chat_session.go:147-154`）
  与 `go mcpRuntime.Rebuild()`（`backend/service/app.go:98`，MCP 子进程握手以秒计）；
  工作区索引更晚——第一次搜索时才建（`EnsureIndexed`，`backend/service/searcher.go:125-135`）。

前端段（`frontend/src/stores/app.ts:76-98`）：`getEndpoint()` → `GET /bootstrap` → 应用主题/动效/字号/语言 →
`ready=true` → `connect()` 建 SSE → 拉会话与引擎读数 → 恢复上次会话。首屏渲染完才 `notifyReady()`（`frontend/src/App.vue:81`）；
带 `--hidden` 启动时 `Ready()` 只刷新托盘菜单、不显窗。

**`GET /chat/state` 的写副作用**：`State` 调 `BindSession(session)`，无会话时 `Store.Create` 并 `Settings.RememberSession`
（`backend/service/chat_state.go:20`、`backend/service/chat_service.go:83-124`）。
它是「拉状态」，但会话初始化必须发生一次，否则界面无处可画。`docs/API.md` §1 对这一条例外做了引用。

### 取舍

- 换来：端口与首屏不等 MCP 握手、不等索引扫盘、不等会话目录建完；启动失败的表达是一句话而不是白屏或卡死的进度条。
- 代价 1：界面先就绪意味着有一小段时间能力不全——索引页会显示「没建」，MCP 页会在后台装载完成后靠 `mcp:changed` 帧刷新。
- 代价 2：装配顺序不可调换的两条（`db` 先于业务、`sessions.Active` 后于 `NewChat`）没有编译期保证，只有本节这一段说明和 §5 的事实约束。

### 坑

- `newToken` 的随机源失败退化为时间戳串：调试时能看出来（`t-` 前缀），但不要拿它当密钥强度。
- `assemble` 里任何一步 `return err` 之后端口就不会监听，此时 `Stop()`/`db.Close()` 由 `shutdown` 空指针判断兜住。
- 台账文件损坏不堵启动：`repo.Open` 把它挪成 `workbaby.json.bad-<毫秒>` 再从空白开始（`backend/repo/store.go:54-61`）；
  这是刻意的——半截 JSON 多半来自断电，留着会让每次启动都失败。

---

## 4. 并发与锁模型

**锁的边界按「一份事实的写入顺序」划，不按调用点划。** 每把锁都要能回答：保护什么、持有多久、为什么不能更短。

### 设计

- 一份事实只有一个写入者，读的人拿副本：会话条目、块表、排队镜像、索引 docs 都是这样。
- 持锁区间 = 该事实从内存推进到磁盘（或推进到可观察状态）的那一次动作；出锁之后才做可能慢的事（写 socket、拉起子进程）。
- 高频路径上的读用原子量免锁（`Chat.running`、`revealed`、`quitting`），锁只保护需要整体一致的字段组。

### 实现

| 锁 | 位置 | 保护什么 | 持有时长 | 为什么不能更短 |
|---|---|---|---|---|
| `Hub.mu` | `backend/sse/hub.go:42-49` | `seq`、环形缓冲 `buffer`、订户表 `sinks`/`nextID` | 发一帧：`seq++` + 追加缓冲 + 快照订户指针 | `seq` 必须单调，且缓冲里的 `seq` 与发号同源，否则补帧会错位；入队放在出锁之后，卡住的窗口才不会拖住广播（`backend/sse/hub.go:114-140`） |
| `subscriber.frames`（容量 64） | `backend/sse/hub.go:62` | 单个连接的帧序 | 非阻塞：满则 `sub.close()` 断连 | 补帧先入队、实时帧后入队，走同一条队列，结构性排除乱序；不背压是因为一个慢客户端会拖死所有客户端 |
| `writeMu` | `backend/api/routes_events.go:26` | 一次 SSE 连接上的 `ResponseWriter` | 写一帧并 `Flush` | 帧来自 hub 发送泵、心跳来自 ticker，两者不同协程；交错会把一帧劈成两半，前端静默丢帧 |
| `Session.mu` | `backend/agent/session.go:35` | 条目镜像、`leafId`、`title`，以及整段 `append`（分配 ID、串父指针、`O_APPEND` 落盘） | 一次条目追加 | 两路并发追加会写出交错半行，整份会话回放不出来；内存镜像与文件必须同一次推进，不能拆开 |
| `Chat.mu` | `backend/service/chat_service.go:37-52` | `session`、`runner`、`cancel`、`queued` 镜像、`level`、`waiters` | 单次读或单次赋值（几纳秒级） | 这几个字段必须一起变才自洽：绑定会话与登记运行不能交叉，取 runner 插话不能拿到半新半旧的队 |
| `Runner.mu` | `backend/agent/loop.go:58` | `queue`、`followUp`、`steps` | 一次入队 / 一次取走 | 写来自 HTTP 协程（`Steer`/`FollowUp`），读来自 runner 协程；`FollowUpQueued` 取走即清，派发与清空必须原子 |
| `Git.writeMu` | `backend/service/git.go:24` | git **写**命令通道 | 一条命令（超时 120 秒） | git 用 `index.lock` 保护索引，并发写会撞锁失败且报错看不出是并发造成；`fetch` 也走这把锁，因为它改远端引用（`backend/service/git_sync.go:63-68`） |
| `Mcp.mu` | `backend/service/mcp.go:28` | `mcp.json` 与项目级 `.pi\mcp.json` 的读-改-写 | 一次读-改-写 | 两份文件共用一把锁：并发时后写会拿回改前旧表，前一次改动凭空消失 |
| `rebuildMu`（容量 1 的 channel） | `backend/service/mcp_runtime.go:29,41,47-48` | `Rebuild` 全程 | 一轮装载（含子进程握手） | 并发重建会留下半新半旧：旧 clients 已 `Close`、新工具还没装上；后到者等前者完成即可 |
| `Registry.mu` | `backend/agent/tool.go:49` | `tools`、`order`、`mcpTools` | 一次注册 / 一次整段替换 / 一次查名 | 内置段装配期一次装完，MCP 段运行中整段换（`ReplaceMcpTools`，`backend/agent/tool.go:152-180`），而轮次循环随时在按名查 |
| `Store.mu` | `backend/repo/store.go:19` | 台账内存态 + 落盘 | 一次 `Save`/`Delete`/`SetSkillEnabled` | 整写走临时文件换名（`pkg.WriteFileAtomic`，`backend/pkg/fsutil.go:58-80`），持锁到换名完成才能保证读到的永远是完整文件 |
| `ToolPolicy.mu` | `backend/agent/policy.go:13` | 只读态与它绑定的会话 ID | 一次读 / 一次切会话 | 闸门与界面横幅必须读同一处，分两份就会有一份先过期 |
| `Searcher.mu`（RWMutex）+ `building` | `backend/service/searcher.go:24-36,85-96` | 内存 `docs`、`frecency`、`workspace`、`indexedAt` | 读：拷指针；写：换一次索引 | HTTP 协程与工具协程同时读；重建期间并发请求直接返回旧状态，不重复扫盘 |
| `Stream.mu` | `backend/service/chat_stream.go:20-36` | 块表 `Blocks`、`toolIndex`、`usage`、`Streaming` | 一次块推进 / 一次取 `run_id` | 块表是「界面此刻该显示什么」的唯一答案，引擎回调与 HTTP 读同时碰它 |
| `PersistentProc` 的三个 once/锁 | `backend/pkg/proc.go:105-114,117-119,226-245` | `killOnce`、`closeOnce`、`exited` 广播、`stderrMu` | `Kill`：杀树后最多等 `killSettleBudget`（3 秒） | 重复 `Kill` 必须幂等且不 panic；等 `Wait` 收尾只给 3 秒，超时就放行——把「杀不干净」变成可观察的失败而不是死锁；stderr 由单协程收、多处读，必须加锁 |

技能开关不进任何锁：状态就是目录位置（`data\skills` 还是 `data\skills_disabled`），搬移靠 `os.Rename`；
台账里的 `skill_toggles` 只是记账，写失败只 `Warn` 不回滚（`backend/service/skills.go:93-131`）。

**中止与收尾链路**：`Chat.Stop` 只做一件事——`cancel()`（`backend/service/chat_service.go:291-301`）。
runner 在 `ctx.Err()` 处把已经吐出的半截回复落盘并标 `stop_reason=aborted`（`backend/agent/loop.go:324-330`），
`Stream.Finish` 连发 `chat:settled` 与 `chat:done` 两帧（`backend/service/chat_stream.go:278-284`）：
无论界面走哪条分支，都能解掉流式态并拿到「是否被中止」。

### 取舍

- 换来：没有「顺序 bug 类」故障要排查——每份事实的推进都串行，帧序、条目序、台账内容不会交错。
- 代价 1：锁偏粗。`Session.mu` 覆盖一次文件 append，长工具输出落盘时同会话的其它读会等；条目量级（几十到几千行、单次写几 KB）下这个等待是微秒到毫秒。
- 代价 2：SSE 用「踢掉慢客户端」换「广播永不被拖住」。被踢的连接靠前端 `EventSource` 退避重连 + `since` 补帧自愈。
- 代价 3：`persist` 是整写 JSON 而不是增量写。几十条设置的量级下换来的是「断电只丢最后一次更新，不会写坏文件」。

### 坑

- 「一次只跑一轮」由 `Chat.running` 表达，但判定与置位在 `Send` 里是两步（`backend/service/chat_service.go:181-185,212`），
  中间不持锁。它依赖的现实前提是同一份会话只有一个前端在发话；并发 `POST /chat/send` 是两个都起一轮的唯一入口。
- `Subscribe` 只补 `buffer` 里保留的 1024 帧，更早的缺口不假装补齐：连接一开先收 `stream:open` 拿当前 `seq`，
  之后任何跳号都触发前端 `chat/state` 全量对账（`frontend/src/stores/app.ts:205-225`）。
- `refreshQueue` 先出锁取 runner、再进锁刷镜像，两次锁之间 runner 可能已归零——镜像最坏情况是显示空一次，不会显示旧内容之外的东西。

---

## 5. 五类真相来源

**排障先看数据从哪来。** 同一份事实写进两个地方，迟早会漂；下面这张表的「唯一写入者」列就是防漂的界线。

| 类别 | 载体 | 唯一写入者 | 谁在读 | 丢了怎么办 |
|---|---|---|---|---|
| 会话 | `data\sessions\<工作区编码>\SES_<ULID>.jsonl`，逐行追加，`parentId` 串分支 | `backend/agent/session.go` 的 `append`（`AppendMessage`/`Rename`/`AppendUsage`/`AppendCompaction`/`AppendCustom` 全走它） | `agent`（组上下文 `history`）、`service/parser`（历史/待办/计划模式/摘要/统计）、`service/sessions.go`、`service/session_html.go` | 会话就是历史本身，不可重建 |
| 设置台账 | `data\workbaby.json`：`settings` 键值 + `skill_toggles` 记账 + `chat.hide:<会话文件>` 隐藏轮台账 | `backend/repo/store.go`（`Save`/`Delete`/`SetSkillEnabled` → `persist` → `WriteFileAtomic`） | `service/settings.go`、`service/sessions.go`（隐藏轮）、`shell`（关窗偏好） | 文件损坏时挪成 `workbaby.json.bad-<毫秒>` 从空白开始；除技能开关外都可重设 |
| 服务商目录 | `data\workbaby.db` 的 `provider` / `provider_model` | `backend/repo/provider_repo.go`，只由 `service/models.go` 调 | 模型页、对话页下拉、`Chat.resolveModel` | 密钥丢了要用户重填，其余可重新录入 |
| 能力配置 | `data\mcp.json`（用户级）与 `<工作区>\.pi\mcp.json`（项目级）；技能目录 `data\skills`、`data\skills_disabled`、`<工作区>\.pi\skills`，随包原件 `assets\skills`；模板 `data\templates\*.md` | `service/mcp.go`（两份 mcp 文件）、`service/skills.go`（目录搬移/新建/导入/删除）、`service/templates.go` | `service/mcp_runtime.go`（按 `enabled` 装工具）、`service/skills_scan.go`（三区扫描出清单）、`service/chat_state.go commands()`（模板命令） | `assets\skills` 里的原件可 `Reset` 恢复内置技能；其余随仓库或用户数据走 |
| 派生数据 | `data\workbaby.db` 的 `search_doc`；进程内索引 `docs` 与 `frecency` | `service/searcher.go`（`ReplaceWorkspace`、`BumpHit`） | 索引页状态读 `Count`，检索读内存 `docs` | 由扫盘重建；重启后 `frecency` 从空开始 |

- **会话目录的分片**：`<工作区编码>` 由 `pkg.EncodedWorkspace` 把绝对路径里的 `:`/`\`/` `/`/` 折成 `-` 并前后加 `--`
  （`backend/pkg/sessionfile.go:58-67`），一个工作区一个目录，切工作区不会看到别处的会话。
- **项目级配置的目录名**：`.pi` 是本项目约定的项目级配置目录，跟着仓库走，与用户级的 `data\` 分开；
  同名条目项目级优先（`backend/service/mcp_runtime.go:116`、`backend/service/skills.go:38-41`）。
- **标题派生而非重写**：会话头只在 `Store.Create` 时写一次，之后永不整写；标题取活动分支上最后一条改名条目，没有就用首条用户消息
  （`backend/agent/session.go:94-118`、`titleFromEntries`）。
- **用量没有独立账本**：用量本来就在每条消息的 `usage` 上。当前会话的读数由 `usageSnapshot` 扫条目得出
  （`backend/service/chat_state.go:63-103`），用量页则把全部会话文件各读一遍在前端聚合（`frontend/src/views/UsageView.vue:187`）。

### 设计

- **会话用 JSONL 追加**：断电最多丢最后一行，不会写坏整份历史；也保住了「翻回去还能找到自己说过的话」——压缩只追加摘要条目，
  压缩点之前的原文一个字节不动（`backend/agent/loop.go:339-370` 的 `history` 用压缩条目做分界）。
- **设置用 JSON、服务商与索引用 SQLite**：台账条目几十条，零依赖、人可直接读；服务商与模型要一起增删改并按模型维度查，
  摊成文件每次查询都得整读整解析。SQLite 用纯 Go 驱动（`github.com/glebarez/sqlite`）不引 cgo，静态构建与交叉编译是交付前提；
  `foreign_keys(1)`、`journal_mode(WAL)`、`busy_timeout(5000)` 三个 pragma 在连接串上打开（`backend/repo/db.go:30`），
  WAL 让「读的同时后台写索引」不互阻。
- **派生数据的边界说死**：`search_doc` 由 `ReplaceWorkspace`/`BumpHit` 写入，`Count` 只用于索引页的文档数；
  检索命中的排序读的是进程内 `docs` 与 `frecency`（`backend/service/searcher.go:71,113,138-170`），
  所以 `frecency` 不跨重启生效——这是现状的边界，不是待办。

### 取舍

- 换来：每类故障的修复动作明确——会话坏了回放一次就能重建视图，台账坏了从空白重设，索引坏了重扫，配置坏了直接改文件。
- 代价：真相分散在五种载体上，跨类的原子性没有保证。技能开关是典型：目录搬移成功、台账记账失败时，磁盘位置是对的、台账旧了，
  而界面只读磁盘位置，所以不影响行为（§5 表里的「唯一写入者」正是靠这个不对称成立）。

### 坑

- 只有台账与 SQLite 有整写事务；会话 JSONL 的追加不经 `WriteFileAtomic`，靠 `O_APPEND` 一次写一行。
- 隐藏轮清单不在会话文件里，而在台账的 `chat.hide:` 键下——删掉 workbaby.json 会让所有隐藏轮重新显形。
- 随包 `assets\skills` 只被 `Install` 与 `Reset` 读（`backend/service/skills.go:33-34,59,148`），运行时没有任何读取方；
  它缺失只影响内置技能能否装/重置，不影响启动（`backend/shell/app.go:150`）。
- 进程内引擎不扫技能目录：`commands()` 只给内置命令与模板命令，`data\skills` 又在工作区之外，
  工具的路径解析把它挡在 `resolve` 之外（`backend/agent/tool.go:218-233`）。技能对模型生效的通路是工作区内的 `<工作区>\.pi\skills`，
  技能页负责的是装、开关、改、恢复。

---

## 6. 安全边界

**工具入参与网络地址都来自对话内容，不能成为逃逸通道。** 每条边界都必须落在一个具体函数上，否则等于没有。

| 边界 | 落点 | 判定 | 理由 |
|---|---|---|---|
| 工具路径 | `agent.Registry.resolve`（`backend/agent/tool.go:218-233`） | 相对路径按当前工作区拼，`filepath.Clean` 后 `pkg.Inside(workspace, ...)`，不过就 `1002` | 模型给的路径是对话内容；漏一个入口就漏一个任意文件读写 |
| 文件接口路径 | `Files.guard`（`backend/service/files.go:45-61`） | 先归一再判越界；空路径按工作区根；`filepath.Join` 自带 `Clean`，`src/../../etc` 折成可被拒的形状 | 相对路径绝不拿进程 CWD 解析——安装态 CWD 是 exe 目录 |
| 技能路径 | `Skills.guardPath`（`backend/service/skills.go:66-77`） | 必须在 `data\skills`、`data\skills_disabled`、`<工作区>\.pi\skills` 三个根之内，且必须指向 `SKILL.md`；项目技能拒开关与删除 | 删除与搬移不可逆，边界要比读更紧 |
| 会话路径 | `parser.GuardSession`（`backend/service/parser/history.go:271-280`）与 `agent.Guard`（`backend/agent/session.go:393-402`） | 必须在 `data\sessions` 下 | 会话名可以被传进来，不能让它指到别处 |
| git | `Git.runIn` 的 `dir`（`backend/service/git.go:40-52`） | 命令的 cwd 是当前工作区，未选工作区直接拒；环境追加 `GIT_TERMINAL_PROMPT=0` | 不给一个能在任意目录跑的 git 入口；凭证问询立刻失败而不是卡满 120 秒 |
| 出网地址 | `web.ValidateURL` + `safeDial`（`backend/web/client.go:223-247,281-320`） | 只允许 http/https；`localhost` 与 `*.localhost` 直接判私；环回、私有段、链路本地、未指定地址、CGNAT `100.64.0.0/10` 全拒 | 只看字面地址会被「域名解析到 `127.0.0.1`」绕过，所以最终闸门在拨号时复核解析结果 |
| 重定向 | `Transport.CheckRedirect` + 逐跳 `ValidateURL`（`backend/web/client.go:54-58,84-86`） | 跳数上限 5；每一跳都过 `safeDial` | 公网地址跳内网是最常见的绕法 |
| 密钥 | `maskKey`（`backend/service/models.go:297-306`） | 读出来恒为掩码（≤8 全掩，否则前 4 + `••••` + 后 4）；入参 `api_key` 空串语义是「保留原值」（`backend/service/models.go:143-147`） | 界面拿不到原文，回传的必然是掩码；把掩码当新值存会把密钥写坏 |
| 配置原文 | `configFiles` / `ConfigText`（`backend/service/system.go:43-87`） | 白名单只有 `workbaby.json` 与 `mcp.json` 两个名字，只按名字取路径，不接受外部路径；超 512 KB 截断 | 查看配置是排障入口，不能变成任意文件读 |
| 只读闸门 | `Registry.Run` → `GuardPolicy`（`backend/agent/tool.go:189-193`、`backend/agent/policy.go:122-132`） | 在解析入参**之前**判；未登记的工具有名按「会改」处理（`IsMutatingTool` 默认返回 `true`，`backend/agent/policy.go:77`） | 逐工具自判时，新增写工具忘了判就等于没拦；漏拦写工具的代价远大于漏拦读工具 |
| 子进程 | `pkg.RunProc` / `pkg.StartPersistent`（`backend/pkg/proc.go:41-80,123-130`） | argv 直传、永不拼命令行字符串；`HideWindow` + `CREATE_NO_WINDOW`；超时 `KillTree`（`taskkill /T /F`）；环境只追加 `NO_COLOR=1` 与 `GIT_TERMINAL_PROMPT=0` | 命令文本来自模型；不遮蔽 `HOME`、不改 `PATH` 顺序，工具跑的是用户自己的环境 |
| 程序查找 | `pkg.LookPath`（`backend/pkg/fsutil.go:113`） | 自己扫 `PATHEXT`/`PATH`，不跑 `where`/`which` | 跑一次就闪一个控制台窗口 |
| HTTP | `api.Server` 中间件（`backend/api/server.go:53-58,102-151`） | 只挂 GET/POST；CORS 排在校验令牌之前；信封恒 200；鉴权失败回裸 401 | 预检不带令牌，先鉴权会把预检打成 401，之后所有真请求都被浏览器拦下 |
| MCP 定义 | `mcpServerDefOf` → `agent.McpServerDef`（`backend/service/mcp_runtime.go:123-140`、`backend/agent/mcp_client.go:12-20`） | 配置文件形状（`mcpServers` 的 map）由 service 解，引擎只认 `command`/`args`/`cwd`/`env` | 协议解析与准入策略留在上层，引擎只管起进程、按行收发、按名调用 |

**工具输出折叠不是安全边界，是上下文预算**：单条超过 4000 字符折成头 3000 + 尾 1000，折叠事实随 `details.tidied` 落盘
（`backend/agent/tidy.go:21-26`、`backend/agent/loop.go:276-281`），界面仍能翻全文，模型不为几万字日志付上下文代价。
同样，超窗时 `trimToWindow` 丢最早的消息但始终留着最后一条——那是用户刚说的那句话，丢了模型就没法回答
（`backend/agent/loop.go:382-405`）。

### 取舍

- 换来：每个能力入口都能被一句话回答「它够得着什么」；新增工具默认落在更严的一侧。
- 代价：闸门集中在注册表意味着工具自己不需要同族检查，但也意味着**新工具必须登记进 `mutatingTools` 或 `readOnlyTools`**，
  没登记就被当写工具拦下——计划模式里读工具消失，比报错更难查。
- 代价：`safeDial` 在拨号时做一次 DNS 解析复核，多一次往返；换来的是「解析结果骗不过去」。

### 坑

- `Inside` 用 `filepath.Rel` 判包含，两边都先取绝对路径（`backend/pkg/fsutil.go:85-96`）：符号链接不在它的判定范围内，
  所以路径边界要保证的是「写错地方」而不是「读到越权文件」，跨目录软链指向的位置需要另做判断。
- 令牌查询参数会进访问日志与前端诊断面板；`WB_DEV_TOKEN` 只用于本机调试，别把它写进任何持久配置。
- 掩码是不可逆的：任何「把界面回传的 `api_key` 原文再存一次」的代码都会覆盖真密钥，所以空串语义必须先判再做。

---

## 7. 交叉引用

- `docs/MODULES.md`：模块级链路、接口、事件、错误码与排障入口（§14 覆盖壳与引导流程，对应本文 §3）。
- `docs/API.md`：全路由契约、信封、SSE 事件词汇全表、错误码双索引（§1 记录了 `/chat/state` 的写副作用例外，对应本文 §3）。
- `docs/DATA-MODEL.md`：持久化载体、设置键全表、会话条目类型全表（对应本文 §5）。
- `docs/DEV.md`：环境、命令、测试策略、打包发布。
- `docs/USER-GUIDE.md`：面向使用者的任务向导（配服务商 → 第一句话 → 各类功能 → 数据位置 → 卸载），与本文的事实口径一致。
- `docs/TROUBLESHOOTING.md`：业务码到动作的映射与按症状的排查路径（本文 §4 的锁与 §5 的真相分类是它的依据）。
- `docs/TOOL-REFERENCE.md`：工具清单、硬上限与只读判定，对应本文 §2 的 `agent` 层与 §5 的能力边界。
- `docs/PROVIDERS.md`：`api_protocol` 在探测与聊天两条路上的差异，是「模型适配只有一个实现」这一取舍的用户侧说明。
- `AGENTS.md`：工程规范唯一权威——命名、错误码段位（§2.4）、依赖方向强制条款（§2.2）、前端令牌（§2.9）、测试规范（§3）、架构门禁（§4）。
- `README.md` / `DESIGN.md`：面向使用者的产品说明与设计意图，不描述实现细节。

---

## 8. 优势与代价总表

一张表汇总整轮取舍：这么设计换来什么、付出什么、什么时候会后悔。

| 设计 | 换来 | 付出 | 什么时候会后悔 |
|---|---|---|---|
| 引擎在进程内，纯 Go 包 | 一条链路排到底；会话/工具/设置共享内存零序列化；测试能脱壳跑 | 引擎 panic 终止应用（runner 协程无 `Recovery` 兜底） | 需要长任务与界面抢同一份 CPU/内存、或要求引擎崩溃后自动恢复时 |
| 业务走回环 HTTP + 随机端口 | 可 curl、可 `httptest` 直接打路由、可脱离壳调试 | 令牌与端口每次随机，外部程序无法稳定接入；多一套令牌门禁 | 要对外提供 API、或要被别的本地进程驱动时 |
| SSE 与业务同端口 | 一份鉴权、一份生命周期、帧序与状态同源 | 慢客户端与业务请求共用 `http.Server`；无写超时（长连接需要） | 连接数远超本机界面规模时 |
| 台账用 JSON 整写 + 互斥锁 | 几十条设置的量级零依赖、人可直接读、断电不写坏文件 | 并发写全部串行；整写不是增量 | 设置条目上千、或出现多进程共写一份台账的需求时（单实例已经排除了后者） |
| 服务商与索引用 SQLite（纯 Go 驱动 + WAL） | 按维度查询、外键约束、读写不互阻；无 cgo，交叉编译可用 | 多一个文件、一套迁移；驱动性能低于 cgo 版 | 需要全文检索/复杂排序交给数据库时 |
| 会话用 JSONL 只追加 | 断电最多丢一行；分支与压缩只靠父指针表达；可回放重建全部界面状态 | 读要全量扫；改名/隐藏等派生状态分散在两处 | 单会话条目达到数十万行、或需要按条目检索历史时 |
| 每份事实一把粗锁 | 无交错、无顺序 bug，读到的永远是完整一致的一刻 | 持锁区间包含文件写与网络写；跨锁的组合操作要靠调用顺序保证 | 出现长耗时操作在锁内（如大批量落盘）拖慢同会话读者时 |
| 补帧缓冲 1024 + 跳号即全量对账 | 断线重连对用户近乎无感；不假装补齐 | 每连接一份队列与协程；超出保留量就要走 `chat/state` 全量 | 事件量级远超对话（例如高频逐帧渲染）时 |
| 只读闸门挂在注册表、未登记按写处理 | 新增写工具默认被拦；系统提示与准入读同一处 | 新增读工具不登记就在计划模式里不可用 | 工具数量多到登记表成为负担时 |
| 目录位置即技能状态（开关靠 `Rename`） | 无台账与磁盘不一致的持久故障；搬移天然原子 | 台账 `skill_toggles` 可能滞后（写失败只 `Warn`）；跨卷搬移不可用 | 技能要按细粒度开关或跨盘集中存放时 |
| 密钥恒掩码 + 空串保留原值 | 界面与日志里永不出现原文 | 前端不能复制原文；语义要靠约定维持 | 需要「导出配置含密钥」这类需求时 |
| 无边框窗口 + 装配期隐藏 + `Ready()` 显形 | 无白屏一闪；标题栏与主题一致 | 窗控、拖动、最大化都走绑定面，代码比系统标题栏多；DPI 需自己换算 | 需要多窗口或系统级窗口动画时 |
| 单一真相分类（会话 / 台账 / 目录 / SQLite / 派生） | 每个故障有唯一归属，修数据不会牵动别处 | 跨类没有事务；`search_doc` 写而不读（排序读内存） | 需要跨类原子（例如改模型同时重算所有会话计价）时 |
