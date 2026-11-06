# 模块职责与依赖取舍

## 分层

```text
main.go（装配根）──► shell ──► api ──► service ──► {repo, engine, chat, parser, platform}
                                       │
                          service ──► pkg / domain（只读类型）
```

依赖单向，逆向导入一律拒绝。判定标准：**下层不知道上层存在**。

## pkg/（叶子工具，无业务语义）

| 文件 | 职责 |
|---|---|
| `apperror.go` | `AppError{Code, Message, Detail}`；`Err` / `WrapErr` / `As` 三个构造与提取 |
| `codes.go` | 业务码常量，与 `docs/ERROR-CODES.md` 一一对应 |
| `resp.go` | HTTP 信封 `{code, message, detail, data}`；`OK` / `Fail` |
| `paths.go` | `Layout` 沙箱目录布局 + `*Ready()` 不抛探测 |
| `env.go` | `SandboxEnv`：PATH 前置内置运行时、HOME/npm 锁进沙箱、`PI_CODING_AGENT_DIR` 重定向 |
| `proc.go` | `RunProc`：argv 直传、隐藏窗口、超时杀、按 LF 切帧 |
| `procflags.go` | `CREATE_NO_WINDOW` 常量：与 `HideWindow` 一起保证子进程不闪黑框 |
| `fsutil.go` | 原子写、目录确保、毫秒时间、存在性探测 |
| `log.go` | 按天滚动日志 + `TailLog` |
| `ulid.go` | `{PREFIX}_{26位ULID}`，前缀白名单 |
| `ziputil.go` | 解压（用于运行时 zip） |
| `frontmatter.go` | 技能 SKILL.md 的 YAML 头解析 |
| `json.go` | 读写 JSON 的薄封装（原子写） |
| `cmdline.go` | 命令行切分（windows 引号规则） |

pkg 可测、可单测，**不含业务判断**。

## domain/（纯类型）

一个域一个文件，`VO` / `REQ` / `RESP` / `DTO` / `DO` 同居：

| 文件 | 内容 |
|---|---|
| `core.go` | 引导、设置、偏好、运行时状态、应用（pi 本体）状态 |
| `chat.go` | 会话、消息、回合块、工具卡、提问、队列、运行状态 |
| `data.go` | 技能、插件、MCP、服务商与模型 |
| `workspace.go` | 文件树、文件内容、git 状态/差异/历史/分支 |

字段全部带业务前缀（`session_id` / `plugin_name` / `skill_dir_name`），
与前端 `types/api.ts` 逐字段对齐——**接口契约唯一落点**。

## repo/（持久化）

只有两件事进 SQLite，因为其余对象的真相都在磁盘或引擎里：

| 表 | 用途 |
|---|---|
| `setting` | 设置 KV（工作区、关窗行为、开机自启记录等） |
| `skill_toggle` | 技能开关台账（目录搬移已表达状态，这里记「用户改过默认值」） |

连接参数：WAL + `busy_timeout=5000` + `foreign_keys=1`。
驱动是 `glebarez/sqlite`（modernc 纯 Go 实现），**全项目不引入 cgo**。

## api/（HTTP 薄壳）

`server.go` 装配：`gin.Recovery` → 令牌鉴权 → 分组路由 → `NoRoute` 回 1404 信封。
handler 只做三件事：读参数、调 service、回信封。**不写业务逻辑、不碰 repo**。

| 文件 | 路由组 |
|---|---|
| `server.go` | `/bootstrap` `/settings` `/prefs` `/engines` `/apps` `/sessions` `/chat` `/messages` `/providers` `/skills` `/plugins` `/mcp` `/files` `/system` `/config` `/events` |
| `routes_git.go` | `/git/*`：状态、差异、历史、分支、提交、推拉 |
| `routes_data.go` | 数据类处理（技能 / 插件 / MCP / 配置） |
| `routes_chat.go` | 对话类处理（发送 / 停止 / 插话 / 回答 / 压缩） |
| `routes_core.go` | 引导、设置、偏好、运行时探测 |
| `routes_events.go` | SSE 长连接 |
| `exe.go` | 服务装配内的共享小工具 |

约定：**只允许 GET / POST**，动词靠路径段表达（`POST /sessions/delete`）。

## service/（业务编排）

| 文件 | 职责 |
|---|---|
| `app.go` / `app_provision.go` / `app_actions.go` / `app_config.go` | 装配根、首启装配、应用（pi 本体）的装/升/卸、配置读写 |
| `chat_service.go` / `chat_state.go` / `chat_actions.go` | 引擎绑定与生命周期（含崩溃自动重启）、发送、停止、插话、回答、压缩；状态总装单拆一文件 |
| `chat/rpc_client.go` | pi 子进程 RPC：启动、发命令、收事件、超时；关停等真退出再返回 |
| `chat/frames.go` | domain 词汇 → SSE 帧（引擎不感知界面） |
| `parser/{entries,history,summary,diff,stat,todos,time}.go` | 会话 JSONL → 条目树 / 活动分支 / 统计 / 待办派生 |
| `plugins.go` / `plugin_install.go` / `plugin_updates.go` / `plugin_catalog.go` / `plugin_settings.go` | 插件装/卸/开/关/升/重试；安装占位、进度上报与失败台账；registry 最新版检查；`settings.json` 的 `packages[]` 读写 |
| `skills.go` / `skill_catalog.go` | 技能落位、开关（搬目录）、导入、编辑、新建、删除、重置 |
| `mcp.go` / `mcp_probe.go` | `mcp.json` 增删改查与开关；连接测试（MCP initialize 握手） |
| `models.go` / `model_probe.go` | `models.json` / `auth.json` 读写，上游探测 |
| `settings.go` / `system.go` / `values.go` / `autostart.go` | 设置、系统动作、常量、开机自启；工作区清单（目录存活 / 仓库分支 / 改动数 / 会话数） |
| `files.go` / `sessions.go` / `session_html.go` | 文件浏览、会话列表与详情、导出 HTML |
| `git.go` / `git_history.go` / `git_sync.go` / `git_stash.go` / `git_write.go` | git 读与写；stash 四件套；凭证失败归 1102 并给下一步指引 |
| `engine/runtime.go` / `engine/provision.go` / `engine/npm.go` | 运行时探测、解压、npm 执行 |
| `platform/` | Windows 专属：开机自启（注册表）、系统打开 |

`service` 可以与 `repo` / `pkg` / `domain` / 子进程对话；**不能碰 `api`**。

## shell/（Wails 面）

| 文件 | 职责 |
|---|---|
| `app.go` | 装配根 `Run`、服务生命周期、绑定面（`Endpoint` / `Ready` / 选目录 / 窗控） |
| `window.go` | 建窗、显形、缩放与关窗决策 |
| `tray.go` | 托盘菜单 |
| `windows.go` | Windows 专属：显示缩放、DWM 等 |
| `endpoint.go` | 端点结构（交给前端的 `{port, token, version}`） |

本包之外（`backend/` 其余部分）**不知道 Wails 存在**。

## sse/

`hub.go` 统一发号（seq 单调）、补帧（按 `since`）、慢客户端断开。每个订阅者一条
带缓冲队列 + 发送泵：补帧先入队、实时帧后入队，同一队列保证乱序不可能发生；
队列写满即按断开处理，一个卡住的窗口拖不住广播。`heartbeat.go` 心跳是注释行，不占 seq。

## 稳定性约束（改代码前必读）

这些地方踩过坑，改动时别把保护去掉：

| 约束 | 为什么 |
|---|---|
| `PiRpc.writeLine` 整行写入在 `writeMu` 内 | 并发 `prompt` / `steer` 各写一次 `io.WriteString` 会交错成半行，引擎按行解析就永久错位 |
| 会话改名要避让引擎正在写的文件 | 两路同时追加 JSONL 会写出交错行，会话文件是唯一真相，写坏就整份回放不出来 |
| 动插件目录前停引擎、动完重启 | Windows 上运行中的 node 锁包文件；且包只在引擎启动时加载，不重启就是「界面写着新版本、跑的是旧行为」 |
| 引擎 stdout 按 LF 切帧 | `bufio.Scanner` 会把 U+2028 / U+2029 也当分隔符，而它们在 JSON 字符串里合法 |
| 工作区清单的 git 探测 8 秒超时 | 历史里可能挂着网络盘或拔掉的移动盘，124 秒默认超时会把整张清单一起卡死 |
| 退出前等引擎真退出再动磁盘 | 进程还活着就删不了包目录，睡若干毫秒赌运气不够 |

## 前端

| 目录 | 职责 |
|---|---|
| `api/` | `http.ts` 封装 `/api/v1`（信封拆包 + 令牌）、`wails.ts` 封装绑定 |
| `stores/` | Pinia：`app`（引导/设置/主题/语言）、`chat`（会话与消息）、`events`（SSE 分帧）、`refresh`（后台事件驱动的清单刷新信号）、`prefs` |
| `composables/` | 组合式逻辑：附件、草稿、菜单、git 动作、消息流、运行状态、toast |
| `types/` | 后端契约的 TS 镜像：`api.ts` 总出口，按域拆 `api-core` / `api-chat` / `api-data` / `api-workspace`，字段名与 Go json tag 一一对应 |
| `locales/` | i18n 文案：`zh-CN/` 与 `en/` 一域一文件（common / overview / chat / workspace / manage / settings），key 路径与界面域对应 |
| `components/` | 按域分目录：chat / workspace / mcp / models / overview / sessions / settings / skills / apps / common / layout |
| `views/` | 路由页：概览 / 对话 / 工作区 / 会话 / 模型 / 技能 / 插件 / MCP / 应用 / 用量 / 设置 |
| `utils/md.ts` | 流式 markdown 渲染（块级缓存，唯一出口） |

## 关键取舍

| 决定 | 为什么 | 替代方案与否决理由 |
|---|---|---|
| Wails3 而非 Electron | 单 exe、无 node 依赖链、Go 直接管子进程 | Electron 要再带一个 node 运行时，与「内置 node」的方案自相矛盾 |
| gin | 成熟、路由与中间件够用，无多余抽象 | 裸 `net/http` 手写路由表——省一个依赖但失去中间件生态 |
| gorm + glebarez/sqlite | 纯 Go，**无 cgo**，交叉编译与免编译链 | `mattn/go-sqlite3` 要 cgo（否决）；`sql.js` 无文件锁语义 |
| 业务走 HTTP 而非全 Wails 绑定 | 可 curl、可测、前端可脱离壳调试 | 全绑定会把 100+ 方法糊进 JS，测试只能起整个壳 |
| 插件只经 `settings.json` | pi 原生机制，升级引擎不失效 | 自建注册表索引——第二份真相必然与引擎不一致 |
| 技能开关用搬目录 | pi 只扫 `skills/`，移走即失联，零额外状态 | 改写 SKILL.md 头——污染用户文件 |
| ULID 而非自增 | 时间有序、可离线生成、无中心分配 | 自增 ID 会把「会话文件是真相」变成「库是真相」 |
