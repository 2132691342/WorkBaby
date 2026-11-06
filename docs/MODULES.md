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

规模：`pkg` 14 文件 / 850 行，`domain` 5 / 1350，`repo` 1 / 120，
`api` 7 / 1450，`service` 47 / 7600，`sse` 2 / 160，`shell` 5 / 680。

## pkg/ — 叶子工具

| 文件 | 职责 |
|---|---|
| `apperror.go` | `AppError{Code, Message, Detail}`；`Err` / `WrapErr` / `As` |
| `codes.go` | 业务码常量，与 `docs/ERROR-CODES.md` 一一对应 |
| `resp.go` | HTTP 信封 `{code, message, detail, data}`；`OK` / `Fail` |
| `paths.go` | `Layout` 沙箱目录布局 + `*Ready()` 不抛探测 |
| `env.go` | `SandboxEnv`：PATH 前置内置运行时、HOME/npm 锁进沙箱、重定向 pi 的家 |
| `proc.go` | `RunProc`：argv 直传、隐藏窗口、超时杀进程树、按 LF 切帧 |
| `procflags.go` | `CREATE_NO_WINDOW` 常量 |
| `fsutil.go` | 原子写、目录确保、毫秒时间、存在性探测 |
| `log.go` | 按天滚动日志 + `TailLog` |
| `ulid.go` | `{PREFIX}_{26位ULID}`，前缀白名单 |
| `ziputil.go` | 解压（先落 `.unpack` 再改名，中途失败不留半个运行时） |
| `frontmatter.go` | 技能 SKILL.md 的 YAML 头解析 |
| `json.go` | JSON 读写薄封装（原子写） |
| `cmdline.go` | 命令行切分（Windows 引号规则） |

`pkg` 不含业务判断，可独立测。

## domain/ — 纯类型

一个域一个文件，`VO` / `REQ` / `RESP` / `DTO` / `DO` 同居：

| 文件 | 内容 |
|---|---|
| `core.go` | 引导、设置、偏好、运行时状态、应用（pi 本体）、引擎身份与能力表 |
| `chat.go` | 会话、消息、回合块、工具卡、提问、队列、运行状态、用量 |
| `data.go` | 技能、插件、MCP、服务商与模型 |
| `workspace.go` | 文件树、文件内容、git 状态 / 差异 / 历史 / 分支 / stash |
| `doc.go` | 包注释 |

字段一律带业务前缀（`session_id` / `plugin_name` / `skill_dir_name`），
与前端 `types/api.ts` 逐字段对齐——**接口契约唯一落点**。

## repo/ — 持久化

只有两张表进 SQLite，因为其余对象的真相都在磁盘或引擎里：

| 表 | 用途 |
|---|---|
| `setting` | 设置 KV |
| `skill_toggle` | 技能开关台账（目录搬移已表达状态，这里记「用户改过默认值」） |

连接参数：WAL + `busy_timeout=5000` + `foreign_keys=1`。
驱动是 `glebarez/sqlite`（modernc 纯 Go 实现），**全项目不引入 cgo**。

## api/ — HTTP 薄壳

`server.go` 装配：`gin.Recovery` → 令牌鉴权 → 分组路由 → `NoRoute` 回 1404 信封。
handler 只做三件事：读参数、调 service、回信封。**不写业务逻辑、不碰 repo**。

| 文件 | 路由组 |
|---|---|
| `routes_core.go` | `/bootstrap` `/settings` `/workspaces` `/prefs` `/logs` |
| `routes_chat.go` | `/chat/*` `/messages/*` |
| `routes_data.go` | `/providers` `/skills` `/plugins` `/mcp` `/sessions` |
| `routes_git.go` | `/git/*`：状态、差异、历史、分支、提交、stash、推拉 |
| `routes_events.go` | `/events` SSE 长连接 |
| `exe.go` | 共享小工具 |

约定：**只允许 GET / POST**，动词靠路径段表达（`POST /sessions/delete`）。

## service/ — 业务编排

| 文件 | 职责 |
|---|---|
| `app.go` / `app_provision.go` / `app_actions.go` / `app_config.go` | 装配根、首启装配、pi 本体的装 / 升 / 卸 |
| `chat_service.go` / `chat_state.go` / `chat_actions.go` | 引擎绑定与生命周期（含崩溃自动重启）、发送、停止、插话、回答、压缩 |
| `chat/rpc_client.go` | pi 子进程 RPC：启动、发命令、收事件、超时；关停等真退出再返回 |
| `chat/frames.go` | domain 词汇 → SSE 帧 |
| `parser/{entries,history,summary,diff,stat,todos,time}.go` | 会话 JSONL → 条目树 / 活动分支 / 统计 / 待办派生 |
| `plugins.go` / `plugin_install.go` / `plugin_updates.go` / `plugin_catalog.go` / `plugin_settings.go` | 插件装 / 卸 / 开 / 关 / 升 / 重试；进度上报与失败台账；registry 最新版检查；`packages[]` 读写与来源串规范化 |
| `skills.go` / `skill_catalog.go` | 技能落位、开关（搬目录）、导入、编辑、新建、删除、重置 |
| `mcp.go` / `mcp_probe.go` | `mcp.json` 增删改查与开关；连接测试（MCP initialize 握手） |
| `models.go` / `model_probe.go` | `models.json` / `auth.json` 读写，上游探测 |
| `settings.go` | 设置、工作区清单（含仓库与会话概况）、偏好 |
| `system.go` / `values.go` / `autostart.go` | 系统动作、常量、开机自启 |
| `files.go` / `sessions.go` / `session_html.go` | 文件浏览、会话列表与详情、改名保护、导出 HTML |
| `git.go` / `git_history.go` / `git_sync.go` / `git_stash.go` / `git_write.go` | git 读与写；stash 四件套；凭证失败归类并给下一步；目录级仓库探测 |
| `engine/runtime.go` / `engine/provision.go` / `engine/npm.go` | 运行时探测、解压、npm 执行 |
| `platform/` | Windows 专属：开机自启（注册表）、系统打开 |

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
| `composables/` | 附件、草稿、菜单、git 动作、消息流、运行状态、toast |
| `types/` | 后端契约的 TS 镜像：`api.ts` 总出口，按域拆四个文件，字段名与 Go json tag 一一对应 |
| `locales/` | i18n：`zh-CN/` 与 `en/` 一域一文件 |
| `components/` | 按域分目录：chat / workspace / mcp / models / overview / sessions / settings / skills / apps / common / layout |
| `views/` | 11 个路由页：概览 / 对话 / 工作区 / 会话 / 模型 / 技能 / 插件 / MCP / 应用 / 用量 / 设置 |
| `utils/md.ts` | 流式 markdown 渲染（块级增量缓存，唯一出口） |

## 关键取舍

| 决定 | 为什么 | 否决的方案与理由 |
|---|---|---|
| Wails3 而非 Electron | 单 exe、无 node 依赖链、Go 直接管子进程 | Electron 要再带一个 node 运行时，与「内置 node」自相矛盾 |
| 业务走 HTTP 而非全 Wails 绑定 | 可 curl、可测、前端可脱离壳调试 | 全绑定会把上百个方法糊进 JS，测试只能起整个壳 |
| gorm + `glebarez/sqlite` | 纯 Go，无 cgo，免编译链 | `mattn/go-sqlite3` 要 cgo；纯 JSON 文件无事务与并发语义 |
| 只把偏好与台账进库 | 其余对象的真相在磁盘或引擎里 | 全量入库＝两份真相，必然不同步 |
| 插件只经 `settings.json` | pi 原生机制，升级引擎不失效 | 自建注册表＝第二份真相 |
| 技能开关用搬目录 | pi 只扫 `skills/`，移走即失联，零额外状态 | 改写 SKILL.md 头——污染用户文件 |
| ULID 而非自增 | 时间有序、可离线生成、无中心分配 | 自增会把「会话文件是真相」变成「库是真相」 |
| 内置运行时缺失即报错 | 回退系统命令等于破坏沙箱承诺 | 静默回退——用户以为在用沙箱，实际在跑自己的环境 |
| 一个工作区一个引擎 | pi 的会话目录与工具相对路径都以工作区为根 | 全局单引擎在切目录时路径必然错乱 |
| 长文本按块增量渲染 | 全量重解析是 O(字数²) | 每次 tick 整体重渲染，长回答直接卡死界面 |
