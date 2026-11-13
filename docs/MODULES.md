# 模块职责与设计取舍

## 分层

```text
main.go（装配根）──► shell ──► api ──► service ──► {repo, agent, web, index, pkg}
                                       │
                          service ──► pkg / domain（只读类型）
```

依赖单向，逆向导入一律拒绝。判定标准：**下层不知道上层存在**。

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| `pkg` | 标准库 / 第三方纯工具 | domain / repo / service / api / shell |
| `domain` | pkg | 任何实现层 |
| `repo` | pkg / domain | service / api / shell |
| `agent` | pkg / domain | api / service / 界面 |
| `web` / `index` | pkg | api / service / agent |
| `service` | repo / agent / web / index / pkg / domain | api 的路由层 |
| `api` | service / domain / pkg | 越过 service 直摸 repo |
| `shell` | wails / service（仅设置与端口回填） | api |

`main.go` 直接开库与起服务是装配本职；`shell/*.go` 读设置服务的关窗偏好同理。

## pkg/ — 叶子工具

| 文件 | 职责 |
|---|---|
| `apperror.go` | `AppError{Code, Message, Detail}`；`Err` / `WrapErr` / `As` |
| `codes.go` | 业务码常量，与 `docs/API.md` 的错误码表一一对应 |
| `resp.go` | HTTP 信封 `{code, message, detail, data}`；`OK` / `Fail` |
| `paths.go` | `Layout` 目录布局（数据目录、会话目录、技能区、设置文件、数据库） |
| `proc.go` | `RunProc`：argv 直传、隐藏窗口、超时杀进程树 |
| `procflags.go` | `CREATE_NO_WINDOW` 常量 |
| `fsutil.go` | 原子写、目录确保、存在性探测、`LookPath`（自己扫 PATH，不跑 `where`） |
| `sessionfile.go` | 会话文件读取（上限内取尾段）与工作区目录编码 |
| `log.go` | 按天滚动日志 + `TailLog` |
| `ulid.go` | `{PREFIX}_{26位ULID}`，前缀白名单 |
| `frontmatter.go` | 技能 SKILL.md 的 YAML 头解析 |
| `json.go` | JSON 读写薄封装（原子写） |
| `cmdline.go` | 命令行切分（Windows 引号规则） |
| `ansi.go` | ANSI 转义码清洗（状态文字要能直接显示） |
| `ulid.go` / `pkg.NowMs` / `pkg.NowISO` | 时间戳一律毫秒 int64（`pkg.NowMs()`）；禁止 ISO 串进 DTO |

`pkg` 不含业务判断，可独立测。

## domain/ — 纯类型

一个域一个文件，`VO` / `REQ` / `RESP` / `DTO` / `DO` 同居：

| 文件 | 内容 |
|---|---|
| `core.go` | 引导、设置、偏好、运行时状态、应用清单、引擎身份与能力表、配置原文 |
| `chat.go` | 会话、消息、回合块、工具卡、提问、队列、运行状态、用量 |
| `data.go` | 技能、MCP |
| `provider.go` | 服务商、模型清单与探测入参 |
| `store.go` | SQLite 持久层的行映射形状（会话索引、用量、工具调用、服务商、模型、检索文档、计划状态） |
| `workspace.go` | 文件树、文件内容、git 状态 / 差异 / 历史 / 分支 / stash |
| `doc.go` | 包注释 |

字段一律带业务前缀（`session_id` / `skill_dir_name` / `provider_name`），
与前端 `types/api*.ts` 逐字段对齐——**接口契约唯一落点**。

## repo/ — 持久化

| 文件 | 职责 |
|---|---|
| `store.go` | `workbaby.json` 设置台账：`settings` 键值 + `skill_toggles` 技能开关记账 |
| `db.go` | SQLite 连接与迁移（GORM + 纯 Go 驱动，外键与 WAL 都开） |
| `provider_repo.go` | 服务商与模型的增删改查 |
| `session_repo.go` | 会话索引与用量记录的读写 |

**为什么只有设置台账用 JSON**：条目量级几十条，零依赖、内容人能直接读。
JSON 存不下的是「服务商 + 模型清单」这种要一起增删改、要按模型维度查统计的关系数据，
摊成文件后每次查询都得整读整解析，放进 SQLite 更省事。

设置台账启动整读进内存，变更即原子整写（临时文件 + 换名），进程内互斥锁。
文件损坏不堵启动：解析失败时挪到旁边留证，从空白重新开始。

**全项目无 cgo**：SQLite 走 `glebarez/sqlite` 纯 Go 驱动，静态构建与交叉编译是这套交付的前提。

## api/ — HTTP 薄壳

`server.go` 装配：`gin.Recovery` → CORS → 令牌鉴权 → 分组路由 → `NoRoute` 回 1404 信封。
handler 只做三件事：读参数、调 service、回信封。**不写业务逻辑、不碰 repo**。

| 文件 | 路由组 |
|---|---|
| `routes_core.go` | `/bootstrap` `/settings` `/workspaces` `/prefs` `/logs` `/engines` `/health` `/apps` `/config` |
| `routes_chat.go` | `/chat/*` `/messages/*` |
| `routes_data.go` | `/providers` `/skills` `/mcp` `/sessions` |
| `routes_files.go` | `/files/*`：工作区文件浏览与编辑 |
| `routes_git.go` | `/git/*`：状态、差异、历史、分支、提交、stash、推拉、冲突解决 |
| `routes_web.go` | `/web/*`：联网状态、设置、搜索、抓取 |
| `routes_index.go` | `/index/*`：索引状态、检索、重建 |
| `routes_events.go` | `/events` SSE 长连接 |
| `exe.go` | 共享小工具 |

约定：**只允许 GET / POST**，动词靠路径段表达（`POST /sessions/delete`）。

## service/ — 业务编排

| 文件 | 职责 |
|---|---|
| `app.go` | 装配根：把各 service 造好并互相接线；工作区切换、关窗行为、引擎快照 |
| `app_actions.go` | 首屏总包、概览健康快照、应用清单（外部工具链检测）、配置原文入口 |
| `app_config.go` | 配置原文的转发 |
| `chat_service.go` | 会话绑定与生命周期、发送 / 停止 / 插话 / 排队、提问通道、模型解析、克隆与分叉与改名 |
| `chat_state.go` | 聊天状态总装：实况 + 会话文件回放 + 待办 + 计划模式 + 斜杠命令 |
| `chat_actions.go` | 附件折进消息、压缩、回答提问、单条队列移除、导出 HTML、自动压缩开关 |
| `chat_stream.go` | 流式翻译器：引擎事件 → 界面词汇（块表、增量帧、用量帧、待办帧） |
| `plan_mode.go` | 计划模式开关：状态落进会话文件，闸门同步到引擎 |
| `searcher.go` | 工作区预索引的建索引、检索与 frecency 记账 |
| `web.go` | 联网门面：配置 → 引擎工具出口 → 界面接口 |
| `models.go` / `model_probe.go` | 服务商与模型的增删改查、连通性探测；密钥恒为掩码；新会话的默认模型 |
| `skills.go` / `skills_scan.go` / `skills_edit.go` / `skill_catalog.go` / `skill_fs.go` | 技能落位、扫描、开关（搬目录）、导入、编辑、新建、删除、重置 |
| `mcp.go` / `mcp_probe.go` | `mcp.json` 增删改查与开关；连接测试（MCP initialize 握手） |
| `files.go` / `sessions.go` / `session_html.go` | 文件浏览、会话列表与详情、改名保护、导出 HTML |
| `git.go` / `git_history.go` / `git_sync.go` / `git_stash.go` / `git_write.go` | git 读与写；stash 四件套；冲突解决；目录级仓库探测 |
| `settings.go` / `values.go` / `system.go` / `autostart.go` | 设置键名、工作区清单、系统动作、开机自启 |
| `parser/` | 会话 JSONL → 条目树 / 活动分支 / 统计 / 待办派生 / 计划模式派生 |
| `platform/` | Windows 外壳能力：开机自启（落 HKCU 用户级自启项）、资源管理器定位、系统默认程序打开 |

## agent/ — 原生引擎

| 文件 | 职责 |
|---|---|
| `provider.go` | 归一后的消息 / 内容块 / 工具声明 / 流式事件；`Streamer` 接口；分片累积器 |
| `provider_openai.go` | `/chat/completions` 的 SSE 适配器 |
| `loop.go` | 轮次循环：调模型 → 跑工具 → 回填 → 再调；中止与插话队列 |
| `session.go` | 会话文件：追加条目、叶子指针、标题派生、越界守卫 |
| `session_format.go` | 引擎消息 ↔ 落盘形态的往返翻译 |
| `tool.go` | 工具注册表、工作区路径守卫、准入闸门调用点 |
| `policy.go` | 工具准入策略：只读白名单 / 写工具名单 / 默认按写处理 |
| `tidy.go` | 工具输出折叠策略 |
| `prompt.go` | 系统提示：身份纪律 + 从注册表现读的工具清单 + 工作区事实 |
| `tool_fs.go` / `tool_search.go` / `tool_shell.go` | 文件、检索、命令三类工具 |
| `tool_todo.go` / `tool_ask.go` / `tool_web.go` / `tool_index.go` | 待办、提问、联网、索引检索工具 |

## web/ — 联网能力

| 文件 | 职责 |
|---|---|
| `search.go` | 搜索适配器：DuckDuckGo / Brave / Exa / Tavily 统一收口成 `SearchResult` |
| `search_parse.go` | DuckDuckGo HTML 结果页解析与跳转链接还原 |
| `client.go` | HTTP 客户端、地址安全校验（`ValidateURL` / `safeDial`）、逐跳重定向校验 |
| `extract.go` | HTML 正文抽取：去脚本 / 样式 / 导航，还原实体 |
| `pdf.go` | PDF 文字抽取 |
| `inflate.go` | 解压辅助 |

## index/ — 工作区预索引

| 文件 | 职责 |
|---|---|
| `index.go` | 遍历建索引：收文件与符号、跳过依赖目录与二进制、限制体积 |
| `search.go` | 模糊检索与相关度分档；frecency 频次账 |

## sse/ — 事件广播

`hub.go` 统一发号、补帧、断开慢客户端。每个订阅者一条带缓冲队列 + 发送泵：
补帧先入队、实时帧后入队，同一队列结构性排除了乱序。队列写满即断开。
`heartbeat.go` 心跳是注释行，不占 seq。

## shell/ — Wails 面

| 文件 | 职责 |
|---|---|
| `app.go` | 装配根 `Run`、服务生命周期、绑定面（`Endpoint` / `Ready` / 选目录 / 窗控） |
| `window.go` | 建窗、显形、缩放与关窗决策 |
| `tray.go` | 托盘菜单 |
| `windows.go` | Windows 专属：显示缩放、DWM |
| `endpoint.go` | 端点结构（交给前端的 `{port, token, version, contract}`） |

本包之外不知道 Wails 存在。

## 前端

| 目录 | 职责 |
|---|---|
| `api/` | `http.ts` 封装 `/api/v1`（信封拆包 + 令牌），`wails.ts` 封装绑定 |
| `stores/` | Pinia：`app`（引导/设置/主题/语言）、`chat`（会话与消息）、`events`（SSE 分帧）、`refresh`（后台事件驱动的刷新信号）、`prefs` |
| `composables/` | 输入区全家：附件、斜杠与提及菜单、草稿；git 动作；消息流；运行状态与计时；文件预览抽屉；服务商表单；视口等比缩放；toast |
| `types/` | 后端契约的 TS 镜像：`api.ts` 总出口，按域拆五个文件（chat / core / data / web / workspace），字段名与 Go json tag 一一对应 |
| `locales/` | i18n：`zh-CN/` 与 `en/` 一域一文件 |
| `components/` | 按域分目录：apps / chat / common / layout / mcp / models / overview / sessions / settings / skills / workspace |
| `views/` | 12 个路由页：概览 / 对话 / 工作区 / 会话 / 应用 / 模型 / 技能 / MCP / 联网 / 代码 / 用量 / 设置 |
| `utils/` | `md.ts` 流式 markdown 渲染（块级增量缓存，唯一出口）、`sse.ts` SSE 分帧读取、`format.ts` 读数格式化（token 数与耗时）、`diag.ts` 诊断上报 |

## 关键取舍

| 决定 | 为什么 | 否决的方案与理由 |
|---|---|---|
| Wails3 而非 Electron | 单 exe、Go 直接管子进程、不拖 node 依赖链 | Electron 要自带一个 node 运行时，壳就多了一层要维护的环境 |
| 业务走 HTTP 而非全 Wails 绑定 | 可 curl、可测、前端可脱离壳调试 | 全绑定会把上百个方法糊进 JS，测试只能起整个壳 |
| 引擎写在进程内而非拉起子进程 | 没有拉起、握手、崩溃重启、版本探测；会话与工具直接共享内存 | 子进程要多一套 RPC 协议、多一份状态同步、多一条失败链路 |
| 设置台账用 JSON、派生数据用 SQLite | 台账条目少、人要能直接读；关系数据要按维度查 | 全 JSON：服务商 + 模型要一起增删改，每次查询整读整解析 |
| 纯 Go SQLite 驱动而非 cgo | 静态构建与交叉编译是交付前提 | `mattn/go-sqlite3` 性能更好，但要 C 编译链 |
| 会话用 JSONL 追加而非整份重写 | 断电时最多丢最后一行，不会写坏整份历史 | 整份重写：写到一半断电，唯一真相就没了 |
| 标题由条目派生而非写进会话头 | 会话头只在创建时写一次，之后永不重写 | 重写会话头改标题：同一份风险，且多一次整份重写 |
| 技能开关用搬目录 | 目录位置即状态，零额外状态 | 改写 SKILL.md 头——污染用户文件 |
| 计划模式闸门挂在注册表上 | 一次实现管住全部写工具 | 逐工具判断：新增工具忘了判就等于没拦 |
| 未登记工具默认按「会改」处理 | 漏登记写工具的代价（闸门失效）远大于漏登记读工具（模型多问一句） | 默认放行：新增写工具若忘了登记，计划模式名存实亡 |
| 系统提示从注册表现读 | 提示里写的工具集不会与实际可调用的不一致 | 手写清单：两边必然漂移，模型会调用不存在的工具 |
| 工具输出折叠后仍留全文 | 界面要能翻全文，模型不该为几万字日志付出上下文代价 | 二选一：全给模型会撑爆上下文，全不给界面就没法查 |
| 联网地址校验做两遍 | 字面校验拦不住「域名解析到 127.0.0.1」 | 只看字符串：域名绕过，直接读本机服务 |
| 重定向逐跳校验 | 只查首地址的话，公网地址跳到 127.0.0.1 就绕过去了 | 交给默认实现：默认会跟到内网地址 |
| 免密钥搜索做默认 | 用户配第一个模型就能联网，不必先申请搜索服务账号 | 默认要密钥：多一道与核心价值无关的门槛 |
| 索引是派生数据 | 随时能从工作区重建，写失败只降级检索 | 索引当真相：重建一次要重扫全仓库，且与文件可能不同步 |
| 长文本按块增量渲染 | 全量重解析是 O(字数²) | 每次 tick 整体重渲染，长回答直接卡死界面 |
