# 架构

WorkBaby 是一个 **自带引擎** 的 Windows 桌面 AI 工作台：智能体循环、模型协议、
工具执行全部在本进程内用 Go 实现。整体结构都围绕这条事实。

## 进程拓扑

```
┌─────────────────────────── WorkBaby.exe ───────────────────────────┐
│  Wails 应用（单一 Go 进程）                                         │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ shell/  窗口 · 托盘 · 单实例 · 绑定面（Endpoint / 选目录 / 窗控）│  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ api/    gin 路由（薄壳：解析入参 → 调 service → 回信封）       │  │
│  │         127.0.0.1:<随机端口>  /api/v1/*                       │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ service/  业务编排：chat session model skill mcp file git     │  │
│  │            web searcher plan_mode                            │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ agent/    原生引擎：轮次循环 · 模型适配 · 工具集 · 会话落盘    │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ web/      联网能力：搜索适配器 · 网页正文抽取 · 地址安全      │  │
│  │ index/    工作区预索引：文件与符号抽取 · 模糊检索 · frecency  │  │
│  ├──────────────────────────────────────────────────────────────┤  │
│  │ repo/        持久化：JSON 设置台账 + SQLite 派生数据          │  │
│  │ domain/      纯类型（VO / REQ / RESP / DTO / DO）             │  │
│  │ pkg/         叶子工具（错误、路径、进程、日志、ULID）          │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                    │ 子进程（一律隐藏窗口）                          │
│                    ▼                                                │
│            git.exe / powershell（工具与版本控制）                    │
│                                                                     │
│  WebView2 渲染前端（Vue 3 + Element Plus）                          │
│  └── 只经 /api/v1 + window.wails 绑定，不碰文件系统与子进程         │
└─────────────────────────────────────────────────────────────────────┘
```

**没有外部引擎依赖**：不需要装 node、不需要装引擎 CLI、不需要 npm 插件生态。
用户配好一个服务商的 API Key 就能开始干活。

**两条通道分工**：业务全部走回环 HTTP（可 curl、可测、前端可脱离壳调试）；
Wails 绑定只暴露系统能力——拿端点、选目录、窗控、托盘提示，不承载业务。

## 分层与依赖方向

```
main(装配) ──► shell ──► api ──► service ──► {repo, agent, web, index, pkg}
                                  │
                   service ──► pkg / domain（只读类型）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| pkg | 标准库 / 第三方纯工具 | domain / repo / service / api / shell |
| domain | pkg | 任何实现层 |
| repo | pkg / domain | service / api / shell |
| agent | pkg / domain | api / service / 界面 |
| web / index | pkg | api / service / agent |
| service | repo / agent / web / index / pkg / domain | api 的路由层 |
| api | service / domain / pkg | 越过 service 直摸 repo |
| shell | wails / service（仅设置与端口回填） | api |
| 前端 | 自身 + `/api/v1` | 直摸文件系统 / 子进程 |

**组合根例外**：`main.go` 直接开库与起服务是装配本职；`shell/*.go` 读
设置服务的关窗偏好同理。

判定标准是**下层不知道上层存在**。`agent` 包不知道 HTTP 与界面的存在，
所以它可以脱离壳单测；`api` 只解析与转发，所以业务规则全在 service 里。

## agent/ — 原生引擎

引擎是一个纯 Go 包，只依赖标准库与 `pkg`，不知道 HTTP、界面与上层编排存在。

| 文件 | 职责 |
|---|---|
| `provider.go` | 归一后的消息 / 内容块 / 工具声明 / 流式事件；`Streamer` 接口 |
| `provider_openai.go` | `/chat/completions` 的 SSE 适配器：正文、思考、工具调用分片、用量 |
| `loop.go` | 轮次循环：调模型 → 跑工具 → 回填结果 → 再调；含中止与插话队列 |
| `session.go` | 会话 JSONL 真相：追加条目、叶子指针、标题派生 |
| `session_format.go` | 引擎消息 ↔ 落盘形态的往返翻译 |
| `tool.go` | 工具注册表与工作区路径守卫 |
| `policy.go` | 工具准入策略：计划模式的只读闸门 |
| `tidy.go` | 工具输出折叠策略：头尾保留 + 中间折叠 |
| `prompt.go` | 系统提示：身份纪律 + 从注册表现读的工具清单 + 工作区事实 |
| `tool_fs.go` | `read` / `write` / `edit` / `ls` |
| `tool_search.go` | `grep` / `find`（进程内实现，不起子进程） |
| `tool_shell.go` | `bash`（Windows 走 PowerShell，超时杀进程树） |
| `tool_todo.go` | `todo`：实时待办清单，全量表随结果落盘 |
| `tool_ask.go` | `ask`：结构化提问，阻塞式等回答 |
| `tool_web.go` | `web_search` / `web_fetch`：联网工具 |
| `tool_index.go` | `index_search`：工作区预索引检索 |

### 轮次循环

```
Run(userMessage)
  ├─ 落盘用户消息
  └─ 循环最多 maxSteps(24) 次
       ├─ streamOnce：系统提示 + 历史 + 工具声明 → 上游流式
       │    └─ 正文 / 思考增量立刻回调给界面（早看到字）
       ├─ 超窗时 trimToWindow 丢最早的消息（始终留最后一条）
       ├─ 落盘助手消息（含工具调用块）
       ├─ 有工具调用 → 顺序执行（并发写同一文件的结果不可预测）
       │    └─ 每个结果作为 tool 消息落盘
       └─ 注入排队插话；没有新输入就收尾
```

**中止要保住已经吐出来的字**：用户看到过这些内容，会话里没有它们就会
「重开应用少了一段」。所以中止时半截回复照样落盘，`stop_reason=aborted`。

### 模型协议

只实现 OpenAI 兼容（`/chat/completions` + `stream=true`），覆盖
DeepSeek / Moonshot / Ollama / vLLM 与各类网关。归一后的事件只有六种：
`text` / `thinking` / `tool_start` / `tool_delta` / `usage` / `stop`。

流式解析的两个坑都在代码里注释着：

| 坑 | 后果 | 落法 |
|---|---|---|
| 工具调用分片交错到达 | 按到达顺序拼入参会把两个函数串成非法 JSON | 按上游 `index` 归并 |
| 正文与思考共用一个槽位 | 两者交替时互相整块重置，吞掉文字 | builder 按「上一个块种类」开新块 |

协议差异只在字段上，不在语义上：思考增量收 `reasoning_content` 与 `reasoning`
两种拼法；有网关用 `200 + error 字段` 报错，适配器一并收口。

### 工具集是能力的唯一边界

模型能做什么，取决于工具注册表里注册了什么。系统提示里的工具清单**从注册表现读**，
所以提示里写的工具集绝不会与实际可调用的工具不一致。

工具按需装上：联网工具要配了搜索服务才装，预索引工具要建了索引才装——
没配时装上它们，模型一调用就报错，不如一开始就不给。

### 计划模式闸门

只读态挂在注册表上，一次实现管住全部写工具；挂成逐工具判断的话，
新增工具时忘了判断就等于没拦。

判定规则在 `agent.IsMutatingTool`：先查只读白名单，再查写工具名单，
都没登记就**按「会改」处理**。默认拦是故意的：漏登记一个写工具，
计划模式就名存实亡；而漏登记一个读工具只会让模型多问一句——两者不对等。

闸门在解析入参**之前**执行：计划模式下的写工具连参数都不该被看一眼。

### 工具输出折叠

大工具输出（几百行文件内容、几万字日志）是上下文的主要消耗。
折叠策略：单条超 4000 字符时保留头 3000 + 尾 1000，中间折叠并标注字数。

完整内容仍留在会话文件里——折叠只是不给模型看，不是丢掉。
折叠这件事要跟着落盘（`details.tidied`），回放时才知道这段被折过。

### 会话真相

会话落在**自己的数据目录** `data\sessions\<工作区编码>\SES_<ULID>.jsonl`，
条目以 `parent_id` 串起分支，所以回放、详情、导出、隐藏台账共用同一套解析。

```
{"type":"session","version":1,"id":"SES_...","timestamp":...,"cwd":"D:\\proj"}
{"type":"message","id":"SES_...","parent_id":"","timestamp":...,"message":{...}}
{"type":"message",...}          ← 工具结果同样是一条 message（role=tool）
{"type":"session_info","title":"我自己起的名字"}
```

标题是**派生**出来的，不是文件的属性：改名条目优先，其次首条用户消息。
所以重开应用、切回旧会话都能还原出同一个标题。

会话头只在创建时写一次，之后永不重写——整份重写在断电时会把唯一真相写坏。

## web/ — 联网能力

| 文件 | 职责 |
|---|---|
| `search.go` | 搜索适配器：DuckDuckGo / Brave / Exa / Tavily 统一收口成 `SearchResult` |
| `search_parse.go` | DuckDuckGo HTML 结果页解析与跳转链接还原 |
| `client.go` | HTTP 客户端：超时、重定向逐跳校验、连接前复核解析出的 IP |
| `extract.go` | HTML 正文抽取：去脚本 / 样式 / 导航，还原实体 |
| `pdf.go` | PDF 文字抽取 |

**地址安全是这条能力的核心边界**：模型给的地址来自对话内容。
校验做两遍——`ValidateURL` 看字面（环回、私网、链路本地、CGNAT 一律拒），
`safeDial` 在真正连接时复核解析出的 IP。重定向逐跳校验：
只查首地址的话，公网地址跳到 `127.0.0.1` 就绕过去了。

免密钥的 DuckDuckGo 是默认搜索服务：用户配第一个模型就能联网搜索，
不必先去申请一个搜索服务的账号。

## index/ — 工作区预索引

一次几千行的 grep 在大仓库里要几秒，而模型每轮都要搜好几次——
把「找文件」这件事预先算好，模型就只等结果。

一次遍历同时收文件与符号（分开遍历等于把目录读两遍）：
文件按路径与正文入库，符号靠正则从正文里抽（Go / TS / JS / Python / Rust 常见声明式）。
二进制、依赖目录（`node_modules` / `.git` / `dist` …）、超大文件一律跳过。

索引上限（不是用户可调的旋钮，是「什么值得进索引」的判断）：
单文件 ≤ 512KB、每文件留正文 ≤ 8000 字符、单次索引 ≤ 20000 条文档。

排序分 = 文本相关度 + frecency（频次 × 时近）。相关度分档：
符号名完全匹配 100 > 符号名包含 60 > 文件名完全匹配 50 > 文件名前缀 40 > 路径包含 30 > 正文包含 10。

frecency 的频次取对数（搜过 100 次的文件不该是搜过 2 次的 50 倍好），
时近用 7 天半衰期的指数衰减。同分时按路径排——结果顺序要稳定，
否则每次搜都跳来跳去。

## repo/ — 持久化

| 段 | 载体 | 内容 |
|---|---|---|
| 设置台账 | `workbaby.json` | `settings`（键值）+ `skill_toggles`（技能开关记账） |
| 派生数据 | `workbaby.db` | `provider` / `provider_model` / `session_index` / `usage_record` / `tool_call_record` / `search_doc` / `plan_state` |

**为什么只有设置台账用 JSON**：条目量级几十条，零依赖、内容人能直接读。
JSON 存不下的是「服务商 + 模型清单」这种要一起增删改、要按模型维度查统计的关系数据，
摊成文件后每次查询都得整读整解析，放进 SQLite 更省事。

SQLite 用纯 Go 驱动（`glebarez/sqlite`），不引 cgo——
静态构建与交叉编译是这套交付的前提。外键与 WAL 都开：
前者保证行不会指向不存在的东西，后者让「读的同时后台在写索引」不互相阻塞。

派生数据（索引、统计）随时可以从真相重建，所以写入失败只降级检索能力，
不影响对话本身。

## api/ — HTTP 薄壳

`server.go` 装配：`gin.Recovery` → CORS → 令牌鉴权 → 分组路由 → `NoRoute` 回 1404 信封。
handler 只做三件事：读参数、调 service、回信封。**不写业务逻辑、不碰 repo**。

**CORS 必须排在鉴权前面**：浏览器对跨源请求先发 OPTIONS 预检，
预检不携带令牌，先鉴权会把预检打成 401，之后所有真请求都被浏览器拦下。

**只允许 GET / POST**，动词靠路径段表达（`POST /sessions/delete`）。
写操作不出现在 GET 上——`GET` 路由不改磁盘状态。

## service/ — 业务编排

| 文件 | 职责 |
|---|---|
| `app.go` / `app_actions.go` / `app_config.go` | 装配根、首屏总包、概览健康快照、应用清单、配置原文 |
| `chat_service.go` / `chat_state.go` / `chat_actions.go` / `chat_stream.go` | 会话绑定与生命周期、发送 / 停止 / 插话 / 回答、状态总装、流式翻译 |
| `plan_mode.go` | 计划模式开关：状态落进会话文件，闸门同步到引擎 |
| `searcher.go` | 工作区预索引的建索引、检索与 frecency 记账 |
| `web.go` | 联网门面：配置 → 引擎工具出口 → 界面接口 |
| `models.go` / `model_probe.go` | 服务商与模型的增删改查、连通性探测 |
| `skills.go` / `skills_scan.go` / `skills_edit.go` / `skill_catalog.go` / `skill_fs.go` | 技能落位、扫描、开关（搬目录）、导入、编辑、新建、删除、重置 |
| `mcp.go` / `mcp_probe.go` | `mcp.json` 增删改查与开关；连接测试（MCP initialize 握手） |
| `files.go` / `sessions.go` / `session_html.go` | 文件浏览、会话列表与详情、改名保护、导出 HTML |
| `git.go` / `git_history.go` / `git_sync.go` / `git_stash.go` / `git_write.go` | git 读与写；stash 四件套；冲突解决；目录级仓库探测 |
| `settings.go` / `values.go` / `system.go` / `autostart.go` | 设置键名、工作区清单、系统动作、开机自启 |
| `parser/` | 会话 JSONL → 条目树 / 活动分支 / 统计 / 待办派生 |

## shell/ — Wails 面

| 文件 | 职责 |
|---|---|
| `app.go` | 装配根 `Run`、服务生命周期、绑定面（`Endpoint` / `Ready` / 选目录 / 窗控） |
| `window.go` | 建窗、显形、缩放与关窗决策 |
| `tray.go` | 托盘菜单 |
| `windows.go` | Windows 专属：显示缩放、DWM |
| `endpoint.go` | 端点结构（交给前端的 `{port, token, version, contract}`） |

本包之外不知道 Wails 存在。

## 装配顺序

`shell.Run` 是装配根：

```
1. resolveLayout    exe 同级有 assets\ → 安装态；否则往上找仓库根 → 开发态
2. pkg.SetLogDir    日志目录先就绪，后续每一步才好排错
3. application.New  Wails 应用 + 单实例 + WebView2 数据目录锁进数据目录
4. 建窗（Hidden）   先不显示，避免装配阶段闪白屏
5. 建托盘           用户可能一启动就关窗，托盘必须先存在
6. ServiceStartup   开设置存储 → 开数据库 → 装配 service → 起 gin（拿到端口）
7. 后台 Warm        会话初始化；不阻塞首屏
8. 前端调 Endpoint  拿到 {port, token} 后建 SSE、拉首屏
9. 前端调 Ready()   由前端决定何时显形
```

## 聊天状态

引擎在进程内，所以没有「等冷启动」这件事：有会话就能画。

| 事实 | 来源 |
|---|---|
| 历史轮 | 会话 JSONL 回放（活动分支） |
| 当前模型 | 设置里的 `model.active_provider` / `model.active_model` |
| 流式态 | `Chat.Busy()` |
| 待办 | 活动分支上最近一次 todo 工具调用派生 |
| 计划模式 | 会话文件里最后一条 `plan-mode` 自定义条目 |

## 前端运行时

```
Wails 绑定（拿 {port, token}）
        │
        ▼
api/http.ts ── axios 实例（统一拆信封，code !== '0' 即抛 ApiError）
        │                          │
        ▼                          ▼
   stores/app（引导·设置·清单）   stores/events（SSE 分帧分发）
        │                          │
        ▼                          ▼
   视图（懒加载路由页） ◄── refresh store（事件驱动的刷新信号）
```

- **启动链路**：`bootstrap` 一次拿全首屏；之后一切增量都从 SSE 来，前端不轮询
- **流式渲染**：正文按顶层块缓存增量渲染，思考块流式期只渲染尾部——长会话不撑爆 DOM
- **断线对账**：前端逐帧跟踪 `seq`，发现跳号就对 `chat/state` 拉一次快照

## 事件流

每个订阅者一条带缓冲队列 + 发送泵：补帧先入队、实时帧后入队，
同一队列保证乱序不可能发生。队列写满即按断开处理——一个卡住的窗口不能拖住广播。

`seq` 由 hub 统一发号且单调递增，是断线补帧的对齐依据。

## 真相来源

| 对象 | 唯一真相 | 写入口 |
|---|---|---|
| 会话 | `data\sessions\**\*.jsonl` | `agent.Session`（追加，持锁） |
| 服务商与模型 | `data\workbaby.db` 的 `provider` / `provider_model` | `service.Models` |
| 技能 | `data\skills\` 与 `data\skills_disabled\` 的目录位置 | 搬目录 |
| MCP | `data\mcp.json` + 工作区 `.pi\mcp.json` | 读写 JSON |
| 壳设置 | `data\workbaby.json` | `repo.Store` |
| 检索索引 | `data\workbaby.db` 的 `search_doc`（派生，可重建） | `service.Searcher` |

**没有第二份注册表**：每类对象只有一个真相，壳只在其上操作。

## 稳定性约束

| 约束 | 失效模式 |
|---|---|
| 会话追加持锁 | 两路并发追加写出交错行，整份会话回放不出来 |
| 工具调用按 `index` 归并分片 | 并行调用串成非法 JSON，入参解析失败 |
| 工具按顺序执行 | 并发写同一文件的结果不可预测 |
| 中止时半截回复照样落盘 | 用户看到过的内容从会话里消失 |
| 引擎回调只在「仍是当前运行」时收尾 | 上一轮的迟到事件把新一轮界面状态抹掉 |
| 标题由条目派生而非写进会话头 | 整份重写时断电把唯一真相写坏 |
| 工具路径经 `pkg.Inside` 守卫 | 对话内容成为逃出工作区的通道 |
| 计划模式「未登记按写处理」 | 漏登记一个写工具，闸门形同虚设 |

## 安全边界

- 令牌每次启动随机生成，只经 Wails 绑定交给自己的 WebView；HTTP 走
  `X-Desktop-Token`，SSE 走 `?token=`（EventSource 带不了自定义头）
- 服务只监听 `127.0.0.1`
- 工具与文件接口一律校验路径在允许的根目录内
- 联网地址一律校验：环回、私网、链路本地、CGNAT 全拒，且连接时复核解析出的 IP
- 密钥读出来恒为掩码；`api_key` 传空串的语义是「保留原值」
