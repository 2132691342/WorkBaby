# 模块 · MODULES

每个 crate 一节：职责 / 对外接口 / 边界。依赖方向见 `AGENTS.md` §2.2，此处不重复。
深入设计见各专项文档（[KERNEL](KERNEL.md) / [TOOLS](TOOLS.md) / [PROVIDER](PROVIDER.md) /
[SESSIONS](SESSIONS.md) / [SKILLS](SKILLS.md) / [FEATURES](FEATURES.md) /
[WORKSPACE](WORKSPACE.md) / [MCP](MCP.md) / [FRONTEND](FRONTEND.md) / [SHELL](SHELL.md)）。

## wb-pkg（叶子工具包）

| 模块 | 职责 |
|---|---|
| error | AppError：`code`(String) + message + details；new / wrap 两个构造 |
| ulid | `{PREFIX}_{26位ULID}` 生成与校验；裸 ULID 供自拼前缀实体 |
| fsutil | 原子写（临时文件 + rename）、ensure_dir、目录体积、深浅拷贝 |
| realpath | 短名与符号链接归一（`\\?\` 前缀处理），路径比较前置 |
| ansi | 剥 ANSI 转义序列（模型与工具输出进界面必经） |
| httprules | GET/POST 白名单与路径风格校验（api 注册路由时调用） |
| json | Resp 信封构造、nil 数组兜底 |

## wb-domain（纯类型）

一个聚合根一个文件，DO / DTO / REQ / RESP / VO 同居；字段一律带业务前缀。

| 文件 | 内容 |
|---|---|
| `session.rs` | 会话与消息类型：列表 VO、详情消息、索引 DO |
| `chat.rs` | 对话入出参与事件 VO |
| `provider.rs` | 模型配置、凭据、可选模型 |
| `skill.rs` | 技能 frontmatter 与管理 REQ |
| `feature.rs` | 内置功能 VO 与装载预设 |
| `tool.rs` | 工具 VO 与开关 REQ |
| `git.rs` | Git 视图类型与写操作 REQ |
| `mcp.rs` | MCP 服务器 VO 与配置 REQ |
| `resource.rs` | 文件树、帮助文档、斜杠命令、工作区历史、背景图、上下文分解、自省快照、文件字节 |
| `settings.rs` | 设置 KV |
| `stats.rs` | 用量聚合 |
| `error.rs` | 错误码常量（按段位分组） |
| `events.rs` | SSE 事件载荷类型 |

## wb-repo（SQLite）

| 表 | 用途 |
|---|---|
| settings_kv | 界面偏好（key TEXT PRIMARY KEY, value TEXT） |
| session_index | 会话列表索引（id/名称/工作目录/模型/最后活动/消息数/摘要） |
| usage_daily | 统计聚合（日期/模型/会话/input/output/cache/成本） |
| feature_toggle | 内置功能开关（feature_code, enabled, config） |
| skill_toggle | 技能开关（skill_name, enabled） |
| tool_toggle | 工具开关（tool_name, enabled） |

连接：单写多读（WAL + busy_timeout）；迁移 `PRAGMA user_version` 步进。
`wb-features` 的自有库（记忆 / frecency / 知识库）不在本仓管——各自开自己的 DB 文件。

## wb-provider（模型接入）

| 模块 | 职责 |
|---|---|
| config | `models.json` 读写（provider 键 → baseUrl/api/apiKey/models[]），凭据解析（`$ENV`） |
| auth | `auth.json` 凭据存取（api_key / oauth refresh），文件锁 |
| client | ModelClient trait：`stream(req) -> Stream<ModelEvent>` |
| openai | openai-completions wire（覆盖 OpenAI/DeepSeek/Moonshot/OpenRouter 等）；流式累加器 |
| anthropic | anthropic-messages wire（thinking 签名、cache tokens） |
| google | google-generative-ai wire |
| events | ModelEvent：delta（text/thinking/toolcall）/ done（usage/stop_reason）/ error |

## wb-tools（工具体系）

| 模块 | 职责 |
|---|---|
| registry | Tool trait（name/label/description/parameters/annotations/execute）+ ToolRegistry |
| builtin | 内置 7 工具：read / bash / edit / write / grep / find / ls |
| exec | 唯一的子进程出口：CREATE_NO_WINDOW / 超时 / 取消 / 输出上限 |
| truncate | 输出截断（2000 行 / 50KB / 单行 500 字符），按字符边界切 |

## wb-kernel（Agent 内核）

| 模块 | 职责 |
|---|---|
| session | 会话树：JSONL 追加、条目类型、leaf→root 回放、分支编辑 |
| agent | 主循环：turn 状态机、steering/followUp 队列、工具批执行、事件发射 |
| blocks | 消息块构造：文本与思考合并、工具调用定稿、工具执行 |
| system_prompt | 有序命名段组装（tools/rules/skills/project_context/cwd/addendum） |
| compaction | 阈值触发（reserve 16384 / keepRecent 20000）、切点算法、清洗与结构化摘要 |
| retry | agent_end 后错误恢复：剔除失败批 + 退避重试 |

对上只暴露：`run()`、`AgentConfig`、`RunOutcome`。
内核事件枚举是唯一出参语言，不含任何界面词汇。

## wb-skills（技能）

| 模块 | 职责 |
|---|---|
| frontmatter | SKILL.md 解析（name/description/description_zh\|en/version/triggers/metadata 自由嵌套）与校验 |
| registry | 三区扫描（内置包 / 用户 / 工作区）、同名先到先得、开关、提示词注入文本 |

内置技能包（21 个）以 `include_dir` 编译进二进制，首次运行时解压到数据目录。

## wb-features（12 项内置功能）

统一形态：`FeatureCtx`（service 注入：数据目录/工作区/SSE 出口）+ 0..n 个 `Tool` 实现 +
可选 UI 桥事件（chat:ask/notify/status/widget）。功能代码不依赖 service。

| 模块 | 内容 |
|---|---|
| todo | 待办清单与依赖图（文件存储） |
| ask_user | 结构化提问 + AnswerBoard（一次性投递） |
| memory | 记忆工具 + `memory_store`（FTS5 + 密钥扫描） |
| web | 搜索与抓取工具 + `web_parse`（正文提取、结果解析） |
| fff | 模糊搜索 + `fff/store`（frecency 库） |
| context_mode | 沙箱执行 + 知识库（`sandbox.rs` 运行时表、`store.rs` FTS5 + RRF） |
| lsp | 语言诊断与修复（`adapters.rs` 服务器目录、`client.rs` JSON-RPC 客户端、`edits.rs` 文本编辑、`diagnostics.rs` / `fix.rs` 工具层） |
| plan | 计划评审（提交 / 勾选 / 收件箱） |
| goal | 目标追踪（单活跃目标状态机） |
| loadout | 工具与技能开关的命名预设 |
| inspect | 自省快照数据面 |
| context_view | 上下文占用分解数据面 |

开关停用即整体撤出（service 装配时按 `feature_toggle` 表过滤，重建注册表）。

## wb-git（工作区版本视图）

| 模块 | 职责 |
|---|---|
| exec | git 命令执行（argv 数组直传、超时、CREATE_NO_WINDOW） |
| parse | porcelain `-z` 输出解析、分支与领先落后 |
| write | 写白名单动作；危险参数拒绝 |

读 status / diff / 历史 / 分支；写限白名单（stage/unstage/discard/commit/分支/合并/推送）。

## wb-mcp（MCP 客户端）

| 模块 | 职责 |
|---|---|
| config | `mcp.json` 读写（全局 / 工作区两份，后者覆盖同名） |
| pool | 连接池：按需连接、超时不阻塞会话、不自动重连 |

基于 rmcp 官方 SDK；stdio + streamable HTTP 两传输。
工具并入注册表并命名 `mcp__<server>__<tool>`。

## wb-service（业务编排）

| 模块 | 职责 |
|---|---|
| chat | 对话 run 管理：校验、排队、唤醒内核、事件翻译 |
| prompt | 系统提示词组装（工具 / 规则 / 技能 / 项目上下文 / 工作目录） |
| features | 功能装配：构造 FeatureCtx + 按开关并入功能工具 |
| resources | 技能管理（建 / 导 / 删 / 正文）与工具视图 |
| files | 文件树与单文件字节读取（文档预览的数据源） |
| misc | 内置文档、斜杠命令、工作区历史、背景图 |
| providers | Provider 增删改查、连通测试、模型清单拉取 |
| mcp | MCP 工具并入注册表 |
| git | Git 读写编排 |
| loadout | 装载预设管理 |
| context | 上下文分解 |
| store | 会话索引与文件定位 |

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
| hub | SSE Hub：订阅 / 心跳 / seq / last_event_id 重放 / 慢客户端策略 |

## src-tauri（壳）

main.rs（tauri builder + 单实例 + 托盘 + 端口握手事件）、app.rs（窗口与菜单）、
shell_cmd.rs（系统能力命令）、tray.rs（托盘菜单）。
壳内不写业务。
