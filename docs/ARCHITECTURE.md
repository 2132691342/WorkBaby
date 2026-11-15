# 架构：为什么这样搭

WorkBaby 是**自带引擎**的 Windows 桌面 AI 工作台：智能体循环、模型协议、工具执行全部在本进程内用 Go 实现。
本文只讲取舍与约束；按功能排障、模块的接口与事件见 `docs/MODULES.md`，持久化细节见 `docs/DATA-MODEL.md`，编解码见 `docs/API.md`。

---

## 1. 进程拓扑：一个进程，没有第二个

```
┌──────────────────────── WorkBaby.exe（单一 Go 进程） ────────────────────────┐
│  shell/   Wails 应用 · 单实例 · 窗口 · 托盘 · 绑定面（Endpoint/选目录/窗控）  │
│  ── 起 ──►  api/   gin @ 127.0.0.1:<随机端口>  /api/v1/*  +  /events (SSE)    │
│             service/  业务编排（chat/model/skill/mcp/file/git/web/…）         │
│             agent/    原生引擎：轮次循环 · 模型适配 · 工具集 · 会话落盘        │
│             web/ index/  联网与预索引：能力包，只依赖 pkg                     │
│             repo/     JSON 设置台账 + SQLite（三张表）                        │
│             domain/ pkg/  纯类型 · 叶子工具                                   │
│                      │ 子进程（一律隐藏窗口、超时杀树）                        │
│                      ▼                                                        │
│              git.exe · powershell · 用户配置的 MCP 服务                       │
│  WebView2 渲染前端（Vue 3 + Element Plus）                                    │
│  └── 只经 /api/v1 与 window.wails 绑定，不碰文件系统、不起子进程              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**为什么引擎不拉子进程**：拉起、握手、崩溃重启、版本探测、状态同步会各多一条失败链路，
而收益只是隔离。进程内实现让会话、工具、设置直接共享内存，代价是引擎崩溃即应用崩溃——
这由「引擎是纯 Go 包、工具调用可取消、子进程全走 `pkg/proc`」来兜。

**为什么业务走回环 HTTP 而不是全 Wails 绑定**：HTTP 可 curl、可脱离壳调试、可用 `httptest` 打路由（`api.Server.Handler`）。
Wails 绑定只暴露系统能力（端点、目录选择、窗控、关闭决策），不承载业务——绑定层一旦膨胀，测试就只能起整个壳。

**端口与令牌每次启动随机**（`shell/app.go newToken`，`WB_DEV_TOKEN` 仅在调试时覆盖）：
服务只绑回环，浏览器侧唯一的门禁就是令牌；`Origin` 白名单只管放行 WebView2 虚拟域与本地开发端口。

---

## 2. 分层与依赖方向

```
main(装配) ──► shell ──► api ──► service ──► {repo, agent, web, index, pkg}
                                  │
                   service ──► pkg / domain（只读类型）
```

| 层 | 允许依赖 | 禁止依赖 | 判定标准 |
|---|---|---|---|
| `pkg` | 标准库 / 第三方纯工具 | domain/repo/service/api/shell | 不含业务判断，可独立测 |
| `domain` | 无（纯类型包） | 任何实现层 | 只有类型与 JSON tag，契约唯一落点 |
| `repo` | pkg、domain | service/api/shell | 只管存取，不知道谁在问 |
| `agent` | pkg、domain、web | api/service/界面 | 不知道 HTTP 与界面的存在，可脱离壳单测 |
| `web` | pkg | api/service/agent | 联网能力包，被 agent 的联网工具直接调用 |
| `index` | 无（纯算法包） | api/service/agent | 索引与检索算法，被 agent 的索引工具直接调用 |
| `service` | repo/agent/web/index/pkg/domain | api 的路由层 | 业务规则只在这里 |
| `api` | service/domain/pkg | 直摸 repo | 只解析入参、调 service、回信封 |
| `shell` | wails、service（仅设置与端口回填） | api 的业务实现 | 本包之外不知道 Wails 存在 |
| 前端 | 自身 + `/api/v1` | 文件系统、子进程 | 全部能力经 HTTP |

**组合根例外**：`main.go` 与 `shell.Run`/`assemble` 直接开 JSON 台账与 SQLite、起 gin——那是装配本职；
`shell/window.go` 读设置服务的关窗偏好同理。

逆向导入一律拒绝。工程规范（命名、错误码、行数、注释）以 `AGENTS.md` 为唯一权威。

---

## 3. 装配顺序

`main.go` → `shell.Run(opts)`（版本号与内嵌前端由 main 注入，shell 不读构建信息）：

1. `resolveLayout`：exe 同级有 `assets\skills` → 安装态；否则往上找仓库根 → 开发态（数据目录加 `-dev` 后缀）。
2. `pkg.SetLogDir`：日志目录先就绪，之后每一步失败才有地方说话。
3. `application.New`：Wails 应用 + 单实例 `com.workbaby.desktop` + WebView2 数据目录锁进 `data\webview`。
4. 建窗（`Hidden`）：装配期显形会让用户看到一帧白屏。物理尺寸按屏幕缩放放大后再声明。
5. 建托盘：用户可能一启动就把窗口关进托盘；托盘菜单里的「最近工作目录」随设置重建。
6. `ServiceStartup` → `assemble`：开 `workbaby.json` → 开 SQLite（迁移 `provider`/`provider_model`/`search_doc`）→ `sse.New(1024)` →
   生成令牌 → `service.New`（见下）→ `api.New(...).Start()` 监听随机端口。
7. `go Chat.Warm()`：会话初始化后台跑，不阻塞端口与首屏。
8. 前端 `getEndpoint()`：拿到 `{port, token, version, contract}` 后才建 SSE、拉 `/bootstrap`。
9. 前端 `Ready()`：首屏画完才显窗（`--hidden` 时只上托盘）。

`service.New` 的接线顺序（`service/app.go`）：`Settings.EnsureDefaults` → `NewGit`（回填 `settings.Git` 让工作区行带分支与改动数）→
`NewSessions` → `NewModels` → `NewWeb` → `NewSearcher` → `agent.NewStore` → `NewTemplates` → `NewChat`（造注册表 + 策略 + 待答表）→
`chat.Templates` → `AddWebTools`/`AddIndexTool`（**无条件装**，可用性取决于上游配置而非装配条件）→
`newMcpRuntime` + `NewMcp` + `go mcpRuntime.Rebuild()`（子进程握手以秒计，不能卡就绪）→
`sessions.Active = chat.CurrentFile`（改名要避让引擎正在写的那份文件）。

**`GET /chat/state` 的写副作用**：`State` 调 `BindSession(session)`，无会话时 `Store.Create` 并 `Settings.RememberSession`。
它是「拉状态」，但会话初始化必须发生一次，否则界面无处画。知道这一点，就不必为「打开聊天页就多一个会话文件」吃惊。

---

## 4. 并发与锁模型

进程内多协程共享状态，锁的边界比锁本身重要：**谁持锁、保护什么、失败时怎么降级**。

| 位置 | 机制 | 保护什么 / 为什么 |
|---|---|---|
| `sse/hub.go` | `Hub.mu` 保护 `seq/buffer/sinks`；广播时先快照订户再出锁入队 | `seq` 必须单调、缓冲必须与发号同源；入队不阻塞，卡住的窗口不能拖住广播与其他客户端 |
| `sse/hub.go` | 每订阅者 64 缓冲 + 单发送泵 | 补帧先入队、实时帧后入队，**同一条队列**结构性排除乱序；写满即断开该连接 |
| `api/routes_events.go` | `writeMu` 串住帧与心跳 | 两者来自不同 goroutine，`http.ResponseWriter` 不支持并发写；交错会把一帧劈成两半，前端静默丢帧 |
| `agent/session.go` | `Session.mu` 串行化 `append` 与 `Entries`/`ContextUsage` 等读 | 并发追加会写成交错半行，整份会话回放不出来；条目只在内存尾部追加，不重写历史 |
| `service/chat_service.go` | `Chat.mu` 保护 `session/runner/cancel/queued/waiters` | 绑定、运行登记、排队镜像、提问通道要一致；`running` 用 `atomic.Bool` 免锁读 |
| `agent/loop.go` | `Runner.mu` 保护 `queue/followUp/steps` | 插话与追加来自 HTTP 协程，读取在 runner 协程；`FollowUpQueued` 取走即清 |
| `service/git.go` | `Git.writeMu` 串行化**写**命令 | 写命令抢 `index.lock`，抢不到只报一句看不出原因的失败；读命令不进锁。`fetch` 也走这把锁——它要改远端引用，与本地写命令并发会打乱索引状态 |
| `service/mcp.go` | `Mcp.mu` 串行化 `mcp.json` 的读-改-写 | 两份文件共一把锁：「保存 A」与「删除 B」并发时后写会拿回改前旧表，前一次改动凭空消失 |
| `service/mcp_runtime.go` | `rebuildMu`（容量 1 channel）串行化 `Rebuild` | 并发重建会半新半旧：旧 clients 已 Close、新工具还没装上 |
| `service/skills.go` | 无锁，靠目录搬移的原子性 | 状态就是目录位置；台账 `skill_toggles` 只是记账，写失败只 Warn 不回滚 |
| `repo/store.go` | 进程内互斥 + 原子整写（临时文件换名） | 台账可能被多协程同时改；整写让断电只丢最后一次更新而不是写坏文件 |
| `pkg/proc.go` | `PersistentProc`：`killOnce`/`closeOnce` 幂等、`exited` 广播、`stderrMu` | `Kill` 等 `Wait` 收尾**最多 3 秒**（`killSettleBudget`）后放行——把「杀不干净」变成可观察的失败，绝不死锁；stderr 由单协程收、读者并发读，必须加锁 |
| `agent/tool.go` | `Registry.mu` 保护 `tools/order/mcpTools` | 内置工具装配期一次装完，MCP 段可运行中整段替换（`ReplaceMcpTools`） |
| `agent/policy.go` | `ToolPolicy.mu`（RWMutex） | 只读态按会话记；`SetReadOnly` 在切会话时同步，闸门与界面读同一处 |
| `service/searcher.go` | `searcher.mu`（RWMutex）+ `building` 标志 | 内存索引被 HTTP 与工具协程同时读；重建期间并发请求直接返回旧状态而不是重复扫盘 |
| `service/chat_stream.go` | `Stream.mu` 保护块表 | 块表是界面「此刻该显示什么」的唯一答案；引擎回调与 HTTP 读同时碰它 |

**中止与收尾**：`Chat.Stop` 只 `cancel()`；runner 在 `ctx.Err()` 处落盘半截回复（`stop_reason=aborted`），
`Stream.Finish` 连发 `chat:settled` + `chat:done` 两帧——界面据此无论走哪条分支都能解掉流式态。

**启动不等重物**：端口与界面先就绪，会话初始化（`Warm`）与 MCP 首轮装载都在后台。

---

## 5. 五类真相来源

「这份数据从哪来」决定排障方向。改错来源就会写出两份互相漂移的事实。

| # | 来源 | 载体 | 唯一负责写 | 谁在读 |
|---|---|---|---|---|
| 1 | 会话 | `data\sessions\<工作区编码>\SES_<ULID>.jsonl`，逐行追加，`parentId` 串分支 | `agent/session.go`（引擎） | `agent`（上下文）、`service/parser`（历史/待办/计划模式/摘要）、`service/sessions.go`、`session_html.go` |
| 2 | 设置台账 | `data\workbaby.json`：`settings` 键值 + `skill_toggles` 记账 | `repo/store.go` | `service/settings.go`、`shell`（关窗偏好）、`repo` 自身 |
| 3 | 服务商目录 | `data\workbaby.db` 的 `provider`/`provider_model` | `repo/provider_repo.go`（经 `service/models.go`） | 模型页、对话页下拉、`resolveModel` |
| 4 | 能力配置 | `data\mcp.json`（用户级）与 `<工作区>\.pi\mcp.json`（项目级）；技能目录 `data\skills`、`data\skills_disabled`、`<工作区>\.pi\skills`；模板 `data\templates\*.md` | `service/mcp.go`、`service/skills.go`、`service/templates.go` | `mcp_runtime`（装载工具）、技能页/斜杠命令、`chat_state.commands()` |
| 5 | 派生数据 | `data\workbaby.db` 的 `search_doc`；进程内索引与 frecency | `service/searcher.go` | 索引页、`index_search` 工具 |

`<工作区>\.pi\` 是本项目约定的**项目级配置目录**：跟着仓库走的那部分配置放这里，
与用户级的 `data\` 分开。项目级同名条目覆盖用户级——仓库里写的配置优先于本机全局配置。
`.pi` 只是目录名，不指向任何外部工具。

**为什么会话用 JSONL 追加**：断电最多丢最后一行，不会写坏整份历史；会话头只在创建时写一次，
所以标题由条目派生（改名条目优先、否则首条用户消息）——重写会话头改标题是同一份风险再加一次整份重写。

**为什么设置用 JSON、服务商与索引用 SQLite**：台账条目几十条，零依赖、人可直接读；
服务商+模型要一起增删改、要按模型维度查，摊成文件每次查询都得整读整解析。纯 Go 驱动（`glebarez/sqlite`）不引 cgo，
静态构建与交叉编译是交付前提；外键与 WAL 都开，让「读的同时后台写索引」不互阻。

**派生数据的边界**：索引可在重启后由首次搜索惰性重建（`EnsureIndexed`）；
`search_doc` 目前**只写不读**（`ReplaceWorkspace`/`BumpHit`/`Count` 有调用方，`Search` 读内存），
所以 frecency 不跨重启生效——这不是 bug 的遮羞布，是现状。

**为什么用量没有独立账本**：用量本来就在每条消息的 `usage` 上，会话是真相；另建账本等于维护第二份可漂移的真相，
而扫描聚合的成本（打开用量页时才做）远低于维护成本。

---

## 6. 安全边界

| 边界 | 做法 | 为什么这样做 |
|---|---|---|
| 路径（工具） | `agent.Registry.resolve`：相对路径按工作区根拼、`Inside` 校验 | 工具入参来自对话内容，不能成为逃出工作区的通道 |
| 路径（文件接口） | `service/files.go guard`：先归一再判越界，`..` 折叠后仍越界即 1002 | 一个 `../../` 就能摸到任意文件 |
| 路径（技能/会话） | `Skills.guardPath`（三个托管根）；`parser.GuardSession`（必须在 `data\sessions` 下） | 删除与改名是不可逆动作，边界必须比读更紧 |
| 路径（git） | 命令在**工作区目录**里跑；`GIT_TERMINAL_PROMPT=0` | 不给一个能跑在任意目录的 git 入口 |
| 网络 | `web.ValidateURL` 字面校验 + `safeDial` 连接前复核解析 IP + 重定向逐跳校验；环回/私网/链路本地/CGNAT 全拒 | 只查首地址的话，公网地址跳到 `127.0.0.1` 就绕过去了 |
| 密钥 | 读出来恒为掩码（`maskKey`：≤8 全掩，否则前 4+后 4）；`api_key` 空串 = 保留原值；配置原文白名单只有 `workbaby.json` 与 `mcp.json` | 界面永远拿不到原文，回传的必然是掩码，把掩码当新值存会把密钥写坏 |
| 只读计划模式 | 闸门挂在注册表（`Registry.Run` 在解析入参**之前**判）；未登记工具按「会改」处理；状态落会话文件 `custom` 条目 | 逐工具判断时，新增写工具忘了判就等于没拦；漏拦写工具的代价远大于漏拦读工具 |
| 子进程 | argv 直传不进 shell、`SysProcAttr.HideWindow`、超时必杀树、`ChildEnv` 只追加 `NO_COLOR=1` 与 `GIT_TERMINAL_PROMPT=0` | 不遮蔽 HOME、不改 PATH 顺序：工具跑的是用户自己的环境 |
| CWD | 工具与 git 都在**当前工作区**内解析相对路径；`Files.guard` 绝不拿进程 CWD 解析 | 进程 CWD 在安装态是 exe 目录，偏出去就是写错地方 |
| HTTP | 只 GET/POST、CORS 排在鉴权之前、信封恒 200、鉴权失败回裸 401 | 预检不带令牌，先鉴权会把预检打成 401，之后所有真请求都被浏览器拦下 |
| MCP | 服务定义由 service 解好喂给引擎（`McpServerDef`）；引擎不认 `mcp.json` 的 map 形状 | 协议解析与策略留在上层，引擎只管「起进程、按行收发、按名调用」 |

**工具输出折叠不是安全边界而是上下文预算**：单条超 4000 字符折成头 3000 + 尾 1000，折叠事实随 `details.tidied` 落盘——
界面仍能翻全文，模型不为几万字日志付上下文代价。同理 `trimToWindow` 在超窗时丢最早消息但始终留着最后一条：
那是用户刚说的那句话，丢了模型就没法回答。

---

## 7. 交叉引用

- 模块级链路、接口、事件、错误码与排障入口：`docs/MODULES.md`
- 全路由契约、信封、SSE 事件词汇全表、错误码双索引：`docs/API.md`
- 持久化载体、设置键全表、会话条目类型全表：`docs/DATA-MODEL.md`
- 环境、命令、测试策略、打包发布：`docs/DEV.md`
- 编码规范、依赖方向强制条款、门禁命令：`AGENTS.md`
