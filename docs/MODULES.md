# 模块 · MODULES

每个 crate 一节：职责 / 对外接口 / 边界。依赖方向见 `AGENTS.md` §2.2，此处不重复。
深入设计见各专项文档（[KERNEL](KERNEL.md) / [TOOLS](TOOLS.md) / [PROVIDER](PROVIDER.md) /
[SESSIONS](SESSIONS.md) / [SKILLS](SKILLS.md) / [FEATURES](FEATURES.md) /
[WORKSPACE](WORKSPACE.md) / [MCP](MCP.md) / [FRONTEND](FRONTEND.md) / [SHELL](SHELL.md)）。

## wb-pkg（叶子工具包）

| 模块 | 职责 |
|---|---|
| error | AppError：`code`(String) + message + details；new / wrap / with_details |
| ulid | `{PREFIX}_{26位ULID}` 生成与校验；裸 ULID 供自拼前缀实体 |
| fsutil | 原子写（临时文件 + rename）、ensure_dir、目录列举、mtime |
| realpath | 短名与符号链接归一（`\\?\` 前缀处理），路径比较前置 |
| ansi | 剥 ANSI 转义序列 |
| httprules | GET/POST 白名单与 `/api/v1/{resource}/{action}/:id` 路径风格校验 |
| resp | Resp 信封构造 |

## wb-domain（纯类型）

一个聚合根一个文件，DO / DTO / REQ / RESP / VO 同居；字段一律带业务前缀。

| 文件 | 内容 |
|---|---|
| `session.rs` | 会话与消息类型：列表 VO、详情消息、索引 DO |
| `chat.rs` | 对话入出参 |
| `provider.rs` | 模型配置、凭据、可用模型；`models.json` / `auth.json` 外部契约 |
| `skill.rs` | 技能 frontmatter 与管理 REQ |
| `feature.rs` | 内置功能 VO 与装载预设 |
| `tool.rs` | 工具 VO 与开关 REQ |
| `git.rs` | Git 视图类型与写操作 REQ |
| `mcp.rs` | MCP 服务器 VO、配置 REQ 与 wire 配置 |
| `resource.rs` | 文件树、帮助文档、斜杠命令、背景图、上下文分解、自省快照、文件字节 |
| `settings.rs` | 设置 KV 与启动数据 VO |
| `stats.rs` | 用量聚合 |
| `subagent.rs` | 子代理角色 VO 与运行结果 |
| `error.rs` | 错误码常量（按段位分组） |
| `events.rs` | SSE 事件载荷类型 |
| `runtime.rs` | 内置运行时的路径契约：数据目录名、PortableGit / Python 根、bash/git/python 候选定位、子进程 PATH 构造 |

## wb-repo（SQLite）

| 模块 | 职责 |
|---|---|
| db | 连接打开（WAL + busy_timeout）与 `user_version` 步进迁移 |
| settings | 设置 KV 读写 |
| session_index | 会话列表索引 |
| usage | 用量聚合写入与查询 |
| toggle | 内置功能 / 技能 / 工具三类开关 |

主库表：`settings_kv`、`session_index`、`usage_daily`、`feature_toggle`、`skill_toggle`、
`tool_toggle`。

连接：单写多读（WAL + busy_timeout）；迁移 `PRAGMA user_version` 步进。
`wb-features` 的自有库（记忆 / frecency / 知识库）不在本仓管——各自开自己的 DB 文件。

## wb-provider（模型接入）

| 模块 | 职责 |
|---|---|
| config | `models.json` / `auth.json` 读写与解析后的可请求形态 |
| client | 客户端工厂：按 `api` 字段选 wire 实现 |
| openai | openai-completions wire（覆盖 OpenAI/DeepSeek/Moonshot/OpenRouter 等）；流式累加器 |
| types | `ModelClient` trait 与 `ModelEvent` / `Usage` / `StopReason` 等基础类型 |

## wb-tools（工具体系）

| 模块 | 职责 |
|---|---|
| registry | Tool trait（name/label/description/parameters/annotations/execute）+ ToolRegistry |
| builtin/mod | 内置 8 工具注册：read / write / edit / bash / python / grep / find / ls |
| builtin/* | 各内置工具实现：read / write / edit / bash / python / grep / find / ls |
| exec | 共享命令执行器（bash 等模型工具走这里）：CREATE_NO_WINDOW / 超时 / 取消 / 输出上限 |
| truncate | 输出截断（2000 行 / 50KB / 单行 500 字符），按字符边界切 |

## wb-kernel（Agent 内核）

| 模块 | 职责 |
|---|---|
| session | 会话树：JSONL 追加、条目类型、leaf→root 回放、分支编辑 |
| agent | 主循环：turn 状态机、steering 队列、工具批执行、事件发射、自动重试 |
| blocks | 消息块构造：文本与思考合并、工具调用定稿、工具执行 |
| compaction | 阈值触发（reserve 16384 / keepRecent 20000）、切点算法、清洗与结构化摘要 |
| subagent | 子代理：隔离子会话跑任务，只把结论回灌父上下文；过程经 `observe` 回调发出供观测 |
| events | 内核事件枚举（唯一出参语言） |

对上暴露：运行入口（`run` / `run_subagent` 及各自的配置与结果类型）+
会话模型（`SessionFile` / `Entry` / `Msg` / `Block`）+ 事件（`KernelEvent` / `EmitFn`）+
子代理观测（`SubagentEvent` / `ObserveFn`）。
系统提示词在 service 层组装（`wb-service::prompt`），内核只接收成品字符串。

## wb-skills（技能）

| 模块 | 职责 |
|---|---|
| frontmatter | SKILL.md 解析与校验（名称规则、description 必填且 ≤1024） |
| registry | 三区扫描（内置包 / 用户 / 工作区）、同名先到先得、开关、提示词注入文本 |

内置技能包（22 个）以 `include_dir` 编译进二进制，扫描时按包指纹校验数据目录里的解压结果，不符即清空重解压。

## wb-features（12 项内置功能）

统一形态：`FeatureCtx`（service 注入：数据目录/工作区/SSE 出口）+ 0..n 个 `Tool` 实现 +
可选 UI 桥事件（chat:ask/notify/status/widget）。功能代码不依赖 service。

| 模块 | 内容 |
|---|---|
| todo | 待办清单与依赖图（文件存储） |
| ask_user | 结构化提问 + AnswerBoard（一次性投递） |
| memory | 记忆工具 + `memory_store`（FTS5 + 密钥扫描）+ `memory_consolidate`（去重整合） |
| web | 搜索与抓取工具 + `web_parse`（正文提取）+ `web_providers`（多搜索源）+ `web_files`（PDF / 克隆）+ `web_video`（字幕） |
| fff | 模糊搜索 + `fff/store`（frecency 库） |
| context_mode | 沙箱执行 + 知识库（`sandbox.rs` 运行时表、`store.rs` FTS5 + RRF） |
| lsp | 语言诊断与修复（`adapters.rs` 服务器目录、`client.rs` JSON-RPC 客户端、`edits.rs` 文本编辑、`diagnostics.rs` / `fix.rs` 工具层） |
| plan | 计划评审（`plan.rs` 提交 / 勾选 / 收件箱、`plan_types.rs` 数据形态、`plan_annotate.rs` 标注） |
| goal | 目标追踪（单活跃目标状态机）+ 续跑提示 |
| loadout | 工具与技能开关的命名预设（service 层） |
| inspect | 自省快照数据面（service 层） |
| context_view | 上下文占用分解数据面（service 层） |

开关停用即整体撤出（service 装配时按 `feature_toggle` 表过滤，重建注册表）。

## wb-git（工作区版本视图）

| 模块 | 职责 |
|---|---|
| exec | git 命令执行（argv 数组直传、超时、CREATE_NO_WINDOW、`core.quotepath=false`；内置 PortableGit 优先于系统 PATH） |
| parse | porcelain `-z` 输出解析、分支与领先落后 |
| write | 写白名单动作；危险参数拒绝 |

读 status / diff / 历史 / 分支；写限白名单（stage/unstage/discard/commit/分支/合并/推送）。

## wb-mcp（MCP 客户端）

| 模块 | 职责 |
|---|---|
| config | `mcp.json` 读写（全局 / 工作区两份，后者覆盖同名） |
| pool | 连接池：按需连接（连接超时默认 10 秒）、超时不阻塞会话、不自动重连 |

基于 rmcp 官方 SDK；stdio + streamable HTTP 两传输。
工具并入注册表并命名 `mcp__<server>__<tool>`。

## wb-service（业务编排）

| 模块 | 职责 |
|---|---|
| chat | 对话 run 管理：校验、排队、唤醒内核、目标续跑 |
| chat_events | KernelEvent → SSE 事件翻译（载荷走 `wb_domain::events` 结构体） |
| prompt | 系统提示词组装（工具 / 规则 / 技能 / 项目上下文 / 工作目录） |
| features | 功能装配：构造 FeatureCtx + 按开关并入功能工具 + 功能清单 + 自省快照 |
| task | task 工具：派子代理干活（需内核与运行上下文） |
| agents | 子代理观测：活跃表 + SubagentEvent → `agent:*` SSE 载荷 |
| review | 分支差异审查（PR 的本地等价物） |
| resources | 技能管理（建 / 导 / 删 / 正文）与工具视图 |
| files | 文件树与单文件字节读取（文档预览的数据源） |
| misc | 内置文档、斜杠命令、工作区历史、背景图 |
| providers | Provider 增删改查、连通测试、模型清单拉取 |
| mcp | MCP 工具并入注册表与连接编排 |
| git | Git 读写编排 |
| loadout | 装载预设管理 |
| context | 上下文分解 |
| store | 会话索引与文件定位 |
| runtime | 内置运行时：PortableGit 与 Python 两份 zip 编译期嵌入，启动时按指纹解压到数据目录 |
| usage | 用量统计：run 结束入日表 + 启动时回填历史 |

依赖全部能力域，但不引 axum / tauri。

## wb-api（组合根 + HTTP）

| 模块 | 职责 |
|---|---|
| lib | 组合根 Startup：装配 Services + Hub + axum，返回端口 |
| routes | 主路由表与响应助手（ok_json / err_json） |
| session_routes | 会话路由 |
| chat_routes | 对话路由 |
| system_routes | Provider / 技能 / 文件 / 文档 / 命令 / 设置附属 |
| git_routes | Git 路由 |
| mcp_routes | MCP 路由 |
| hub | SSE Hub：订阅 / seq / last_event_id 重放（心跳由 axum KeepAlive 提供） |
| cors | CORS：仅放行本机来源与 Tauri webview，方法限 GET / POST |
| bin/standalone | 独立调试入口：不起壳直接起 HTTP 服务 |

依赖 wb-service / wb-repo / wb-kernel / wb-domain / wb-pkg 与 axum / tokio；
禁止直摸 provider / tools / skills / features / git / mcp。
组合根例外：开库（`Repo::open`）与会话文件操作（`SessionFile` / `Entry`——
改名 / 切模型 / 切思考档 / 导出）在 api 层直接做，其余能力域一律经 service。

## 第三方依赖

通用第三方版本集中在根 `Cargo.toml` 的 `[workspace.dependencies]`：
serde / serde_json / serde_yaml / tokio / tokio-util / async-trait / thiserror / ulid /
regex / dunce / rusqlite（bundled）/ reqwest（rustls）/ eventsource-stream / futures /
axum / tower-http / tracing 系 / chrono / ignore / walkdir。
各 crate 以 `workspace = true` 引用，crate 内不写版本号，升级只动根表。

两个例外：wb-mcp 专属的 rmcp 与 src-tauri 的 tauri 系依赖，
版本就近写在各自 `Cargo.toml`，升级只动声明处。

## src-tauri（壳）

`lib.rs`（tauri builder + 单实例 + 托盘 + 端口握手事件 + 窗控）、
`main.rs`（入口，只调 `workbaby_lib::run`）、
`cmds.rs`（8 项系统能力命令）。

壳内不写业务。
