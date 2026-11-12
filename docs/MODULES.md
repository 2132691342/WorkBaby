# 模块职责与设计取舍

## 分层

```text
main.go（装配根）──► shell ──► api ──► service ──► {repo, engine, chat, parser, platform}
                                       │
                          service ──► pkg / domain（只读类型）
```

依赖单向，逆向导入一律拒绝。判定标准：**下层不知道上层存在**。

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| `pkg` | 标准库 / 第三方纯工具 | domain / repo / service / api / shell |
| `domain` | pkg | 任何实现层 |
| `repo` | pkg / domain | service / api / shell |
| `service` | repo / pkg / domain | api 的路由层 |
| `api` | service / domain / pkg | 越过 service 直摸 repo |
| `shell` | wails / service（仅设置与端口回填） | api |

`main.go` 直接开库与起服务是装配本职；`shell/*.go` 读设置服务的关窗偏好同理。

## pkg/ — 叶子工具

| 文件 | 职责 |
|---|---|
| `apperror.go` | `AppError{Code, Message, Detail}`；`Err` / `WrapErr` / `As` |
| `codes.go` | 业务码常量，与 `docs/API.md` 的错误码表一一对应 |
| `resp.go` | HTTP 信封 `{code, message, detail, data}`；`OK` / `Fail` |
| `paths.go` | `Layout` 目录布局（壳的数据目录 + 用户的 pi 家）+ `ResolvePiAgentDir` |
| `env.go` | `ChildEnv`：只覆写 `PI_CODING_AGENT_DIR` / `PI_SKIP_VERSION_CHECK` / `NO_COLOR`，不遮蔽 HOME、不改 PATH |
| `proc.go` | `RunProc`：argv 直传、隐藏窗口、超时杀进程树、按 LF 切帧 |
| `procflags.go` | `CREATE_NO_WINDOW` 常量 |
| `fsutil.go` | 原子写、目录确保、毫秒时间、存在性探测、`LookPath`（自己扫 PATH，不跑 `where`） |
| `log.go` | 按天滚动日志 + `TailLog` |
| `ulid.go` | `{PREFIX}_{26位ULID}`，前缀白名单 |
| `frontmatter.go` | 技能 SKILL.md 的 YAML 头解析 |
| `json.go` | JSON 读写薄封装（原子写） |
| `cmdline.go` | 命令行切分（Windows 引号规则） |

`pkg` 不含业务判断，可独立测。

## domain/ — 纯类型

一个域一个文件，`VO` / `REQ` / `RESP` / `DTO` / `DO` 同居：

| 文件 | 内容 |
|---|---|
| `core.go` | 引导、设置、偏好、运行时状态（含 `pi_needs_node`）、应用（pi 本体）、引擎身份与能力表 |
| `chat.go` | 会话、消息、回合块、工具卡、提问、队列、运行状态、用量、分叉 |
| `model.go` | 服务商与模型：`ProviderVO` / `ModelOptionVO` / `ModelDefaultVO` 与它们的 REQ |
| `data.go` | 技能、插件、MCP、服务商与模型 |
| `workspace.go` | 文件树、文件内容、git 状态 / 差异 / 历史 / 分支 / stash |
| `doc.go` | 包注释 |

字段一律带业务前缀（`session_id` / `plugin_name` / `skill_dir_name`），
与前端 `types/api.ts` 逐字段对齐——**接口契约唯一落点**。

## repo/ — 持久化

只有两段进 JSON 文件（`workbaby.json`），因为其余对象的真相都在磁盘或引擎里：

| 段 | 用途 |
|---|---|
| `settings` | 设置 KV |
| `skill_toggles` | 技能开关台账（目录搬移已表达状态，这里记「用户改过默认值」） |

启动整读进内存，变更即原子整写（临时文件 + 换名），进程内互斥锁。
**全项目不引入 cgo，也不引入数据库。**

## api/ — HTTP 薄壳

`server.go` 装配：`gin.Recovery` → 令牌鉴权 → 分组路由 → `NoRoute` 回 1404 信封。
handler 只做三件事：读参数、调 service、回信封。**不写业务逻辑、不碰 repo**。

| 文件 | 路由组 |
|---|---|
| `routes_core.go` | `/bootstrap` `/settings` `/workspaces` `/prefs` `/logs` |
| `routes_chat.go` | `/chat/*` `/messages/*` |
| `routes_data.go` | `/providers` `/models` `/skills` `/plugins` `/mcp` `/sessions` |
| `routes_files.go` | `/files/*`：列目录、搜索、读、存、建目录、删、改名 |
| `routes_git.go` | `/git/*`：状态、差异、历史、分支、提交、stash、推拉 |
| `routes_events.go` | `/events` SSE 长连接 |
| `exe.go` | 共享小工具 |

约定：**只允许 GET / POST**，动词靠路径段表达（`POST /sessions/delete`）。

## service/ — 业务编排

| 文件 | 职责 |
|---|---|
| `app.go` / `app_provision.go` / `app_actions.go` / `app_config.go` | 装配根、后台预热（清遗留库 → 撤旧 `npmCommand` → 探测 pi → 拉起引擎）、状态汇总与系统动作 |
| `chat_service.go` / `chat_state.go` / `chat_actions.go` | 引擎绑定与生命周期（含崩溃自动重启、后台预热 Warm）、发送、停止、插话、回答、压缩。状态总装走两条路径：引擎已绑定走实况快路径；未就绪先回放会话文件并触发预热，不等冷启动 |
| `chat/rpc_client.go` | pi 子进程 RPC：启动、发命令、收事件、超时；关停等真退出再返回 |
| `chat/frames.go` | domain 词汇 → SSE 帧 |
| `chat/json_helpers.go` | RPC 记录（`map[string]any`）的类型安全取值小工具 |
| `parser/{entries,history,summary,diff,stat,todos,time}.go` | 会话 JSONL → 条目树 / 活动分支 / 统计 / 待办派生 |
| `plugins.go` / `plugin_install.go` / `plugin_updates.go` / `plugin_catalog.go` / `plugin_settings.go` | 插件装 / 卸 / 开 / 关 / 升 / 重试；进度上报与失败台账；registry 最新版检查；`packages[]` 读写与来源串规范化。内置清单收六条（能力不重叠），下架插件的旧声明按「包名 + 钉住的版本」整串比对后撤回，用户自己升过级的不动；升级走「备份旧目录 → 装新的 → 失败回滚」；`piSettingsMu` 串行化所有 settings.json 的读-改-写（含 `npmCommand`） |
| `skills.go` / `skills_scan.go` / `skills_edit.go` / `skill_catalog.go` / `skill_fs.go` | 技能落位、开关（搬目录）、导入、编辑、新建、删除、重置（`skills.go`）；扫描与来源判定（`skills_scan.go`）；正文读写（`skills_edit.go`）。落盘一律经 `copyTreeAtomic`（临时目录 + 换名）；开关 / 重置 / 删除以传入的 SKILL.md 路径定位技能目录，动作对象是整个目录；`Provision` 自愈「两个区同时有同一个技能」的破损态，并清掉已下架内置技能在沙箱里的旧副本（下架名单在 `removedBuiltinSkills`） |
| `mcp.go` / `mcp_probe.go` | `mcp.json` 增删改查与开关；连接测试（MCP initialize 握手）。删除只动界面显示的那一条（项目级遮蔽用户级时不连带删除）；探测先看 HTTP 状态码再解析，失败文案带上服务端 stderr |
| `models.go` / `model_probe.go` | `models.json` / `auth.json` 读写，上游探测。`/providers` 回服务商视图（密钥掩码），`/models` 回全部模型平铺（对话页下拉，按服务商名排序） |
| `jsonutil.go` | 工作区历史行的 JSON 编解码（坏值当空，不阻断） |
| `settings.go` | 设置、工作区清单（含仓库与会话概况）、偏好 |
| `system.go` / `values.go` / `autostart.go` | 系统动作、常量、开机自启 |
| `files.go` / `sessions.go` / `session_html.go` | 文件浏览、会话列表与详情、改名保护、导出 HTML |
| `git.go` / `git_history.go` / `git_sync.go` / `git_stash.go` / `git_write.go` | git 读与写；stash 四件套；凭证失败归类并给下一步；目录级仓库探测 |
| `engine/runtime.go` / `engine/npmroot.go` / `engine/system.go` | 入口探测（手动路径 → PATH → npm 全局根，全程只查目录与 `pkg.LookPath`，不跑 `where` / `which`）、版本读取、CLI 调用；`npmroot.go` 按环境变量 → node 同目录 → 版本管理器前缀找全局 `node_modules`；`system.go` 探 node / git / python 的版本，并在「pi 在但 node 缺」时给专门的指引 |
| `platform/` | Windows 外壳能力：开机自启（`autostart_windows.go`，落 HKCU 用户级自启项）、资源管理器定位与系统默认程序打开（`api.go` / `shell.go`）。只依赖标准库与 `pkg`，不感知业务 |

## shell/ — Wails 面

| 文件 | 职责 |
|---|---|
| `app.go` | 装配根 `Run`、服务生命周期、绑定面（`Endpoint` / `Ready` / 选目录 / 窗控） |
| `window.go` | 建窗、显形、缩放与关窗决策 |
| `tray.go` | 托盘菜单 |
| `windows.go` | Windows 专属：显示缩放、DWM |
| `endpoint.go` | 端点结构（交给前端的 `{port, token, version}`） |

本包之外不知道 Wails 存在。

## sse/ — 事件广播

`hub.go` 统一发号、补帧、断开慢客户端。每个订阅者一条带缓冲队列 + 发送泵：
补帧先入队、实时帧后入队，同一队列结构性排除了乱序。队列写满即断开。
`heartbeat.go` 心跳是注释行，不占 seq。

## 前端

| 目录 | 职责 |
|---|---|
| `api/` | `http.ts` 封装 `/api/v1`（信封拆包 + 令牌），`wails.ts` 封装绑定 |
| `stores/` | Pinia：`app`（引导/设置/主题/语言）、`chat`（会话与消息）、`events`（SSE 分帧）、`refresh`（后台事件驱动的刷新信号）、`prefs` |
| `composables/` | 输入区全家：附件（粘贴图 / 回形针 / 工作区引用）、斜杠与提及菜单、草稿；git 动作（提交 / 推拉 / stash 的确认流）；消息流（发送 · 插话 · 排队 · 重投）；运行状态与计时；文件预览抽屉；服务商表单；视口等比缩放；toast |
| `types/` | 后端契约的 TS 镜像：`api.ts` 总出口，按域拆四个文件，字段名与 Go json tag 一一对应 |
| `locales/` | i18n：`zh-CN/` 与 `en/` 一域一文件 |
| `components/` | 按域分目录：apps / chat / common / layout / mcp / models / overview / plugins / sessions / settings / skills / workspace |
| `views/` | 11 个路由页：概览 / 对话 / 工作区 / 会话 / 模型 / 技能 / 插件 / MCP / 应用 / 用量 / 设置 |
| `utils/` | `md.ts` 流式 markdown 渲染（块级增量缓存，唯一出口）、`sse.ts` SSE 分帧读取、`diag.ts` 诊断上报 |

## 关键取舍

| 决定 | 为什么 | 否决的方案与理由 |
|---|---|---|
| Wails3 而非 Electron | 单 exe、Go 直接管子进程、不拖 node 依赖链 | Electron 要自带一个 node 运行时，壳就多了一层要维护的环境 |
| 业务走 HTTP 而非全 Wails 绑定 | 可 curl、可测、前端可脱离壳调试 | 全绑定会把上百个方法糊进 JS，测试只能起整个壳 |
| 标准库 JSON 文件做持久化 | 条目量级几十条，零依赖、零编译链、内容人能直接读 | SQLite 要么 cgo 要么拖一个纯 Go 驱动；事务与 WAL 对几十条 KV 是负资产 |
| 只把偏好与台账进存储 | 其余对象的真相在磁盘或引擎里 | 全量入库＝两份真相，必然不同步 |
| 插件只经 `settings.json` | pi 原生机制，升级引擎不失效 | 自建注册表＝第二份真相 |
| 技能开关用搬目录 | pi 只扫 `skills/`，移走即失联，零额外状态 | 改写 SKILL.md 头——污染用户文件 |
| ULID 而非自增 | 时间有序、可离线生成、无中心分配 | 自增会把「会话文件是真相」变成「库是真相」 |
| 用用户已装的 pi | 配置与会话只有一份真相；命令行与 GUI 互不打架 | 复制一份隔离目录——两边长期漂移，用户不知道哪个才生效 |
| 不代装 pi / 运行时 | 机器是用户的，壳只负责用好已有的东西 | 随包分发 + 自动下载：多 100MB+、多一条会失败的安装链，且跟用户自己装的版本冲突 |
| 推荐清单只展示不预置 | 用户的 `settings.json` 与 `skills/` 是他自己的 | 首启静默写入——用户下次在终端里看到一堆没装过的东西 |
| 一个工作区一个引擎 | pi 的会话目录与工具相对路径都以工作区为根 | 全局单引擎在切目录时路径必然错乱 |
| 长文本按块增量渲染 | 全量重解析是 O(字数²) | 每次 tick 整体重渲染，长回答直接卡死界面 |

## 单文件行数纪律

每个文件 ≤ 500 行是硬约束。超了按「概念」拆，不按行数随机切：

| 拆分 | 为什么这样切 |
|---|---|
| `domain/chat.go` + `domain/model.go` | 前者是「引擎怎么跑一轮」，后者是「引擎能选哪些模型」，改动原因不同 |
| `api/routes_data.go` + `api/routes_files.go` | 前者管管理域对象，后者管工作区里的手工文件操作 |
| `views/WorkspaceView.vue` + `components/workspace/WorkspaceHead.vue` | 页头只读外部传进来的状态，自己不发请求，搬出去不会多出一份数据来源 |
| `utils/format.ts` | 读数格式两处共用（轮尾读数 / 活动条），各自实现一遍必然出现同一轮两个数字 |
