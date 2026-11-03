# 模块 · MODULES

每个 crate 一节：职责 / 内部模块 / 对外接口 / 边界。依赖方向见 `AGENTS.md` §2.2，
深入设计见各专项文档。

## wb-pkg（叶子工具包）

不依赖任何 wb-* crate，纯工具与错误类型。

| 模块 | 职责 |
|---|---|
| `error` | AppError：`code`(String) + `message` + `details`；`new` / `wrap` / `with_details` |
| `ulid` | `{PREFIX}_{26位ULID}` 生成与校验；裸 ULID 供自拼前缀实体；`Prefix` 枚举是前缀白名单 |
| `fsutil` | 原子写（临时文件 + rename）、`ensure_dir`、目录列举、mtime |
| `realpath` | 短名与符号链接归一（`\\?\` 长路径前缀处理）、`lexical_clean`、`relative_to`（越界报 1003） |
| `ansi` | 剥 ANSI 转义序列 |
| `httprules` | GET/POST 白名单与 `/api/v1/{resource}/action/:id` 路径风格校验 |
| `proc` | 子进程纪律：`CREATE_NO_WINDOW` 常量、`hide_window`、`resolve_command`（纯路径查找，不起子进程，自展开 `.exe` / `.cmd`） |
| `resp` | `Resp` 信封构造 |

## wb-domain（纯类型）

一个聚合根一个文件，DO / DTO / REQ / RESP / VO 同居；字段一律带业务前缀；
只依赖 `wb-pkg`。

| 文件 | 内容 |
|---|---|
| `session.rs` | 会话与消息类型：列表 VO、详情消息、索引 DO |
| `chat.rs` | 对话入出参 |
| `provider.rs` | 模型配置、凭据、可用模型；`models.json` / `auth.json` 外部契约 |
| `skill.rs` | 技能 frontmatter 与管理 REQ |
| `feature.rs` | 内置功能 VO（含 `feature_configurable`）、装载预设、功能配置字段表 VO |
| `feature_state.rs` | 功能活状态：待办 / 目标 / 计划 / 记忆的 VO 与写入 REQ |
| `tool.rs` | 工具 VO 与开关 REQ |
| `git.rs` | Git 视图类型与写操作 REQ |
| `mcp.rs` | MCP 服务器 VO、配置 REQ 与 wire 配置 |
| `resource.rs` | 文件树、帮助文档、斜杠命令、背景图、上下文分解、自省快照、文件字节 |
| `settings.rs` | 设置 KV 与启动数据 VO |
| `stats.rs` | 用量聚合 |
| `subagent.rs` | 子代理角色 VO 与运行结果 |
| `events.rs` | SSE 事件载荷类型（前后端契约的唯一出处） |
| `error.rs` | 错误码常量（按段位分模块） |
| `messages.rs` | 错误码 → 中/英默认文案（不经过界面也要能读懂：日志、调试、兜底） |
| `runtime.rs` | 内置运行时的路径契约：目录名、PortableGit / Python 根、bash/git/python 候选定位、子进程 PATH 构造 |
| `runtime_status.rs` | 运行时状态 VO（两件是否就位、命中路径、解压进度与失败原因） |

## wb-repo（SQLite）

主库 `settings.db`，单写多读（WAL + `busy_timeout` 5 秒），迁移用 `PRAGMA user_version` 步进。

| 模块 | 职责 |
|---|---|
| `db` | 连接打开与迁移 |
| `settings` | 设置 KV 读写 |
| `session_index` | 会话列表索引 |
| `usage` | 用量聚合写入与查询 |
| `toggle` | 内置功能 / 技能 / 工具三类开关 |

表：`settings_kv`、`session_index`、`usage_daily`、`feature_toggle`、`skill_toggle`、`tool_toggle`。
`wb-features` 的自有库（记忆 / frecency / 知识库）不在本仓管，各自开自己的 DB 文件。

## wb-provider（模型接入）

| 模块 | 职责 |
|---|---|
| `types` | `ModelClient` trait、`ModelEvent`、`StreamRequest` / `ReqMessage` / `ContentPart`、`Usage` / `StopReason` |
| `config` | `models.json` / `auth.json` 读写与解析后的可请求形态；密钥级联 |
| `client` | 客户端工厂：按 `api` 字段选 wire 实现（openai / anthropic / gemini），不支持的 api 报 3005 |
| `anthropic` · `anthropic_stream` | Anthropic Messages wire：system 独立传、tool_result 装在 user 消息里、思考与工具调用独立块 |
| `gemini` | Gemini wire：systemInstruction 独立传、functionCall / functionResponse、thought part |
| `presets` | 内置七家服务商预设（地址 + 模型 + 能力），首启一键添加；密钥永远由用户给 |
| `openai` | openai-completions wire 实现与流式累加器 `Acc` |

## wb-tools（工具体系）

| 模块 | 职责 |
|---|---|
| `registry` | `Tool` trait（name / label / description / parameters / annotations / execute）+ `ToolRegistry` + `ToolOutput` |
| `ctx` | `ToolCtx { session_id, run_id, progress }`：工具回话的会话归属 + 执行过程增量出口 |
| `builtin/` | 内置 9 工具：`read` / `write` / `edit` / `bash` / `powershell` / `python` / `grep` / `find` / `list`(ls) |
| `exec` | 共享命令执行器：CREATE_NO_WINDOW / 超时 / 取消 / 输出上限 / 增量回调 / 注入内置运行时 PATH |
| `truncate` | 输出截断（2000 行 / 50KB / 单行 500 字符），按字符边界切 |
| `archive` | 超长输出先落盘再截断，回一个可再读的句柄（判据与 truncate 一致） |
| `difftext` | 行级差异（add / del / ctx），只给界面看，不进模型上下文 |
| `job` | 后台任务登记（长命令与沙箱执行） |

## wb-kernel（Agent 内核）

| 模块 | 职责 |
|---|---|
| `session` | 会话树：JSONL 追加、条目类型、leaf→root 回放、分支编辑 |
| `session_msg` | 消息类型：`Msg` / `Block` / `Entry` / `UserInput` / `ImagePart` |
| `session_repair` | 回放补齐：为没有结果的工具调用补 `is_error` 结果，保证消息序合法 |
| `agent` | 主循环：turn 状态机、steering 队列、工具批执行、事件发射、自动重试、空转防护 |
| `blocks` | 消息块构造、工具调用定稿、并发分组 `group_calls_for_concurrency`、`Gate` 契约（审批闸门） |
| `toolbatch` | 一批工具按分组执行并把结果按调用顺序回灌 |
| `reqbuild` | 把会话消息展开成请求消息（技能块、图片段、工具结果） |
| `compaction` | 阈值判断、切点算法、摘要生成、活状态回注（`ReinforceFn`）、固定开销 `ContextOverhead` |
| `tokenest` | token 估算：按字符类别加权（西文 4:1 / 中日韩 1:1）；上下文占用优先用上游真实用量 |
| `branch_summary` | 分支摘要：回溯时把被放弃的分支压成一条可读 brief 插进新分支 |
| `throttle` | 过程回显节流器：高频输出块压到 120ms 一帧（丢帧不丢内容） |
| `session_tree` | 会话树：从父子链还原分叉结构（纯派生视图，不碰存储） |
| `subagent` | 子代理：独立会话 + 收窄白名单 + 观测回调；`subagent_prompt` 组装子提示词 |
| `events` | `KernelEvent` 枚举（唯一出参语言） |

对上暴露：`run` / `run_subagent` 与各自配置和结果类型、会话模型
（`SessionFile` / `Entry` / `Msg` / `Block`）、`KernelEvent` / `EmitFn`、
`SubagentEvent` / `ObserveFn` / `SkillLoader`。
系统提示词在 service 层组装（`wb-service::prompt`），内核只接收成品字符串。

## wb-skills（技能）

| 模块 | 职责 |
|---|---|
| `frontmatter` | SKILL.md 解析与校验（名称规则、description 必填且 ≤1024） |
| `registry` | 三区扫描（内置包 / 用户 / 工作区）、同名覆盖、开关、提示词注入文本 |
| `context_files` | 项目指令文件发现：`AGENTS.md` / `CLAUDE.md` 沿祖先目录向上，最近的排最后 |

内置技能包以 `include_dir` 编译进二进制，扫描时按包指纹校验数据目录里的解压结果，
不符即清空重解压。

## wb-features（12 项内置功能）

统一形态：`FeatureCtx`（service 注入：数据目录 / 工作区 / SSE 出口 / 配置表）+
0..n 个 `Tool` 实现 + 可选 UI 桥事件（`chat:ask` / `chat:notify` / `chat:status` / `chat:widget`）。
本 crate 不依赖 service，也不依赖 kernel。

| 模块 | 内容 |
|---|---|
| `todo` | 待办清单与依赖图（文件存储） |
| `ask_user` / `answer_board` | 结构化提问工具与回答板（待答落盘、一次性投递） |
| `memory` / `memory_store` / `memory_consolidate` / `fts` | 记忆工具、FTS5 存储与密钥扫描、自动去重整合、全文检索公共层 |
| `web` / `web_parse` / `web_providers` / `web_files` / `web_video` | 搜索与抓取、正文提取、多搜索源适配、PDF 与仓库克隆、视频字幕 |
| `fff` | 模糊文件名与内容搜索 + frecency 存储 |
| `context_mode`（`sandbox` / `store`） | 沙箱执行与本地知识库（FTS5 + RRF 双路召回） |
| `lsp`（`adapters` / `client` / `diagnostics` / `fix` / `edits`） | 语言服务器目录、JSON-RPC 客户端、诊断与修复工具、文本编辑 |
| `plan` / `plan_types` / `plan_annotate` | 计划提交与勾选、数据形态、标注 |
| `goal` | 目标状态机与续跑提示 |
| `tool_search` | 工具检索（BM25）与按需加载工具 |
| `config` | 功能级配置的读取与全局注入 |

`loadout` / `inspect` / `context_view` 三项是纯数据面，实现在 `wb-service`（不需要模型工具）。

## wb-git（工作区版本视图）

| 模块 | 职责 |
|---|---|
| `exec` | git 命令执行（argv 数组直传、超时、CREATE_NO_WINDOW、`core.quotepath=false`；内置 PortableGit 优先） |
| `parse` | porcelain `-z` 输出解析、分支与领先落后、条目归类 |
| `write` | 写白名单动作；危险参数拒绝 |

读 status / diff / 历史 / 分支；写限白名单（stage / unstage / discard / resolve / commit /
分支 / 合并 / 拉取 / 推送 / init / remote）。

## wb-mcp（MCP 客户端）

| 模块 | 职责 |
|---|---|
| `config` | `mcp.json` 读写（全局 + 工作区两份，后者覆盖同名）；增删请求头（OAuth token 落这里） |
| `pool` | 连接池：按需连接、超时不阻塞会话、不自动重连 |
| `oauth` · `oauth_crypto` | OAuth 登录：授权码 + PKCE + 本地回调端口；PKCE / base64url / URL 编解码 |

基于 rmcp 官方 SDK；stdio + streamable HTTP 两传输。
工具并入注册表并命名 `mcp__<server>__<tool>`。

## wb-service（业务编排）

依赖全部能力域，但不引 axum / tauri。

| 模块 | 职责 |
|---|---|
| `lib` | `Services` 聚合体（数据目录 / 工作区 / repo / 注册表 / 内核配置 / MCP 池 / 运行时进度） |
| `chat` | 对话 run 管理：校验、排队、唤醒内核、目标续跑 |
| `chat_model` | 模型解析：思考档位归一化 + 默认 provider/model 选取 |
| `approval` | 安全闸门：破坏性操作识别 + 审批卡 + 会话级信任名单 |
| `chat_events` | `KernelEvent` → SSE 事件翻译（载荷走 `wb_domain::events` 结构体） |
| `prompt` | 系统提示词组装（工具 / 规则 / 技能 / 项目上下文 / 工作目录） |
| `features` | 功能装配：构造 `FeatureCtx` + 按开关并入功能工具 + 功能清单 + 自省快照 |
| `feature_state` | 功能活状态读取：把落盘的 todo / goal / plan / memory 读成 VO |
| `feature_write` | 功能活状态写入：界面勾选 / 批准 / 标注 / 删改走与模型工具同一份落盘 |
| `feature_config` | 功能级配置：字段表构建（类型 / 候选 / 不可用原因）、保存、键名白名单 |
| `task` | `task` 工具：派子代理干活（按 `ToolCtx.session_id` 查父会话参数，不读全局快照） |
| `agents` | 子代理观测：活跃表 + `SubagentEvent` → `agent:*` SSE 载荷 |
| `review` | `review_diff` 工具：分支差异审查 |
| `resources` / `skills` | 技能管理（建 / 导 / 删 / 正文）与工具视图 |
| `files` / `misc` | 文件树与单文件字节；内置文档、斜杠命令、背景图 |
| `providers` | Provider 增删改查、连通测试、模型清单拉取、内置预设一键添加 |
| `mcp` | MCP 工具并入注册表与连接编排 |
| `git` | Git 读写编排；`store` 会话索引与文件定位；`export` 会话导出 HTML |
| `workspace` | 工作区热切换：校验目录 → 重扫技能 / 重建待办存储 → 重建注册表 → 重连 MCP → 写回 KV → 广播 `app:workspace` |
| `runtime` | 内置运行时：两份 zip 编译期嵌入，后台线程按指纹解压；`status()` 以磁盘可执行文件为准报就位情况 |
| `usage` | 用量统计：run 结束入日表 + 启动时回填历史 |
| `loadout` / `context` | 装载预设管理；上下文分解 |

## wb-api（组合根 + HTTP）

| 模块 | 职责 |
|---|---|
| `lib` | 组合根 `Startup`：装配 Services + Hub + axum，返回端口；工作区解析与默认值兜底 |
| `routes` | 主路由表与响应助手（`ok_json` / `err_json`） |
| `session_routes` / `chat_routes` | 会话与对话路由 |
| `system_routes` | Provider / 技能 / 文件 / 文档 / 命令 / 设置 / 统计 / 运行时 / 工作区 |
| `feature_routes` / `git_routes` / `mcp_routes` | 功能活状态、Git、MCP 路由 |
| `hub` | SSE Hub：订阅 / seq / `last_event_id` 重放（心跳由 axum KeepAlive 提供） |
| `cors` | CORS：仅放行本机来源与 Tauri webview，方法限 GET / POST |
| `workspace_default` | 默认工作区建议与合规判定（系统目录 / 盘根一律不当工作区） |
| `standalone` | 独立调试入口：不起壳直接起 HTTP 服务 |

依赖 wb-service / wb-repo / wb-kernel / wb-domain / wb-pkg 与 axum / tokio；
禁止直摸 provider / tools / skills / features / git / mcp。

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
`cmds.rs`（系统能力命令）。壳内不写业务。
