# API 契约

字段名全部取自 `backend/domain/*.go` 的 json tag 原文。行为约束（空串语义、上限、落盘副作用、排障顺序）
只写在 `docs/MODULES.md` 对应章节，本文不复述。

错误码逐条「下一步该做什么」见 `docs/TROUBLESHOOTING.md`（本文 §5 只给触发条件）；
工具侧的入参与硬上限见 `docs/TOOL-REFERENCE.md`；`api_protocol` 的行为差异见 `docs/PROVIDERS.md`。

## 1. 信封与鉴权

这一节讲什么：HTTP 出口的统一形状、鉴权方式与中间件顺序，全部请求都要过这一道。

- baseURL：`http://127.0.0.1:{port}/api/v1`。端口随机（监听 `127.0.0.1:0` 后取实际值），令牌每次启动随机；
  两者由 Wails 绑定 `Endpoint()` 回填，字段为 `wb_port / wb_token / wb_version / wb_contract / wb_base_url`，
  前端直接用 `wb_base_url`（已含 `/api/v1` 前缀）。
- 中间件顺序：`gin.Recovery` → CORS → 鉴权 → 路由 → `NoRoute`。
- CORS 排在鉴权之前：浏览器对带自定义头的跨源请求先发不带令牌的 OPTIONS 预检，
  先鉴权会把预检打成 401，之后所有真请求都被浏览器拦下。预检直接回 204 + 允许头
  （Methods：GET/POST/OPTIONS；Headers：Content-Type, X-Desktop-Token；Max-Age 600）。
- 允许源限 `http(s)://wails.localhost` 与 `localhost` / `127.0.0.1` 任意端口；服务只绑回环地址，
  真正的门禁是令牌，源检查只是让浏览器放行。
- 鉴权取值：先读 `X-Desktop-Token` 请求头；头为空时退回 `?token=` 查询参数
  （SSE 的 EventSource 带不了自定义头，走这条）。不匹配回 **401 且不带信封**——
  那不是业务失败，是根本没进门。
- **恒回 200**，业务失败看信封 `code`；`code` 是数字字符串，`"0"` 即成功，失败时 `data` 为 null。

```json
{ "code": "0", "message": "", "detail": "可选原话", "data": { } }
```

- `message` 直接给用户看，`detail` 只进诊断面板。未知接口由 `NoRoute` 回 `1404`（信封形态），
  前端拿它和业务错误走同一条显示路径。
- **只允许 GET / POST**；动词靠路径段表达（`POST /sessions/delete`）。`GET` 不写盘；
  唯一例外：`/chat/state` 会隐式建会话并写 `prefs.last_session_file`（见 `docs/ARCHITECTURE.md` §3）。
- **有专门接口的键不能走通用保存入口**：`POST /settings/save` 收到 `workspace.current` 或 `workspace.history`
  时直接回 `1000`，`message` 指路「工作区请走 `/settings/workspace`（要校验目录并同步历史）」。
  这样绕不过目录校验，也不会把文件接口的根指到任意位置。这两个键因此各自只有一个写入方
  （`Settings.SetWorkspace` / `ForgetWorkspace`）。

## 2. 参数与出参约定

这一节讲什么：入参怎么传、缺省怎么兜底、出参字段长什么样。

- 没有路径段变量，一律查询参数：`?path=` `?session=` `?workspace=` `?q=` `?query=` `?hash=`
  `?limit=` `?lines=` `?name=` `?since=` `?token=`。
- `limit` 类缺省值由 handler 兜底，**超上限不是截到上限，而是回到缺省值**（三者口径不完全一致，逐个写清）：
  `/git/history` 缺省 50、`> 500` 时**回到 50**（`service/git.go` `History`）；`/git/file-history` 缺省 40、
  `> 200` 时**回到 40**（`service/git_history.go` `FileHistory`）；`/files/search` 缺省 50、`> 200` 时**截到 200**
  （`service/files.go` `Search` 把非法值直接赋成上限）；`/logs` 缺省 500，不设上限。
  要「尽可能多地拿」就传上限本身，传更大的数只会得到缺省那一页。
- 整数查询参数为空或解析失败按缺省值走，不报错（`queryInt`）；`/events` 的 `?since=` 非数字当 0。
- POST body 一律 JSON；解析失败回 `1000`，`detail` 是解析器原话（`bind`）。
  body **上限 16 MB**（`maxRequestBodyBytes`，`backend/api/server.go`）：绑定前先包一层 `MaxBytesReader`，
  不会先把整个 body 读进内存再判；超限按解析失败处理，回 `1000`。这是每个请求单独的上限，
  与文件写入的 5 MB 不是一回事。
- 只收 `GET` / `POST`；`OPTIONS` 预检由 CORS 中间件直接回 204，不进鉴权。
- 出参字段名一律 snake_case + 业务前缀；时间戳毫秒 int64（会话文件内的 ISO 串是落盘格式，不进 DTO）。
- **密钥只在入口收、在出口换掩码**：服务商密钥读出恒为 `api_key_masked`（前端从拿不到原文）；
  设置里的 `web.search_key` 在 `GET /settings` 与 `GET /prefs` 的 `pref_values` 里都被换成 `••••`（`maskSettings`）。
  掩码只用于读回显，写入路径不经过它，所以「空串 = 保留原值」的语义仍然成立。
- 只有成功标志的接口回 `{"ok": true}`（`OkRESP`）。

## 3. 路由表

这一节讲什么：全部端点的逐条对账——入参字段、出参字段、语义与副作用。
字段名是 json tag 原文；`[]` 表示数组，`{}` 表示键值对象，`?` 表示 omitempty 可缺省。

### 3.1 引导 / 设置 / 偏好（MODULES §12）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/bootstrap` | — | `app_version,app_contract,engines[],active_engine,workspace,settings{},layouts[],workspaces[],runtime,prefs` | 首屏总包，`app_contract` 当前为 3；`engines[]` 单行 `EngineIdentity`，`workspaces[]` 行见 3.1 `/workspaces` |
| GET | `/settings` | — | 键值 `{}` | 全部设置键值；`web.search_key` 恒为掩码 |
| POST | `/settings/save` | 键值 `{}`（`SaveSettingsREQ`） | 键值 `{}` | 落盘后回全量；`ui.mention_sessions` 收敛到 1~20；`workspace.current` / `workspace.history` 回 `1000`（只能走 `/settings/workspace`）；`agent.bash_confirm` 控制危险命令确认闸门（`"0"` 关，其余开） |
| POST | `/settings/workspace` | `workspace` | `workspace` | 切工作区；会话绑定随之失效 |
| GET | `/workspaces` | — | `workspace,workspace_rows[],active_engine` | 行：`workspace_path,workspace_exists,workspace_is_repo,workspace_branch?,workspace_dirty,session_count,updated_at` |
| POST | `/settings/workspace/forget` | `workspace_path` | `workspace,workspace_rows[],active_engine` | 只去历史，不删磁盘 |
| GET | `/prefs` | — | `pref_values{},auto_start,restore_session,last_session_file,close_action,data_dir,log_dir` | `auto_start` 以注册表为准；`pref_values` 里的密钥同样掩码 |
| POST | `/prefs/autostart` | `pref_enabled` | `AppPrefsVO` | 写 HKCU Run 项 |
| POST | `/prefs/restore` | `pref_enabled` | `AppPrefsVO` | 只记设置，执行者是前端 |
| POST | `/prefs/close-action` | `close_action`（`tray`\|`quit`） | `AppPrefsVO` | 非 `quit` 一律按 `tray` |
| POST | `/prefs/data-dir` | — | `{ok}` | 资源管理器打开数据目录 |
| GET | `/logs?lines=` | `lines` | `lines[]` | 当日日志尾部 |

### 3.2 引擎 / 应用 / 健康 / 系统（MODULES §14）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/engines` | — | `engines[],layouts[],runtime,instance,detected_at` | `instance` 现造：`engine_kind,running,busy,session_file,work_dir,started_at` |
| POST | `/engines/detect` | — | `runtime,apps[]` | 重新探测工具链；广播 `engine:detected` |
| POST | `/engines/repair` | — | `runtime,apps[]` | 与 detect 完全等价，同一 handler 逻辑 |
| POST | `/engines/restart` | — | `{ok}` | 中止当前运行（`Chat.Stop()`）并广播 `engine:ready` |
| GET | `/health/engines` | — | `health_engines[],active_engine,workspace,runtime,total_sessions,total_skills,total_mcp,live_engines,memory_used_mb,detected_at` | 概览页首屏；`health_engines[]` 行：`engine_kind,engine_label,engine_version,installed,running,busy,session_count,skill_count,mcp_count,config_files[],caps`；`engine_exe/process_id/started_at/call_count/last_error` 与顶层 `live_engines` 恒为零值（进程内没有子进程可报） |
| GET | `/apps` | — | `apps[]` | 行：`app_key,app_name,app_version,app_path,installed,app_source,install_note?`；`required/can_locate/blocker?` 恒为零值（探测只报装没装与装在哪） |
| GET | `/config/files` | — | `entries[]` | 行：`config_name,config_label,file_path,file_exists,size_bytes` |
| GET | `/config/text?name=` | `name`（`settings`\|`mcp`） | `file_path,file_text,truncated,found` | 白名单，不接受外部路径；文件还没生成回 `found:false`，名字不在白名单回 `1000` |
| POST | `/system/reveal` | `target_path` | `{ok}` | 资源管理器定位 |
| POST | `/system/open` | `target_path` | `{ok}` | 系统默认程序打开 |

### 3.3 会话（MODULES §3）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/sessions?workspace=&q=` | `workspace`（空则扫全部工作区）、`q` | `sessions[]` | 行：`session_id,session_path,session_title,session_cwd,session_model,updated_at,size_bytes,message_count,total_tokens,cost_usd`；`message_count` 是消息条数（用户 + 助手 + 工具结果各算一条），不是对话轮数；`q` 命中标题或正文 |
| GET | `/sessions/detail?path=` | `path` | `session,items[]` | 行：`item_kind,item_role,item_text?,item_model?,tool_name?,tool_call_id?,tool_is_error,created_at,item_tokens?,cost_usd?` |
| POST | `/sessions/delete` | `session_path` | `{ok}` | 引擎正打开这份会话时先中止运行并切到新会话再删（`PrepareDelete`）；一并清 `chat.hide:<路径>` 台账键 |
| POST | `/sessions/rename` | `session_path,session_title` | `{ok}` | 追加 `session_info` 条目；被引擎打开时走引擎句柄改名（`RenameActive`，与运行共用写锁）并广播 `session:title` |

### 3.4 对话（MODULES §1）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/chat/state?session=` | `session` | `ChatStateRESP`（见下） | 无会话时隐式建会话并写 `prefs.last_session_file` |
| POST | `/chat/send` | `session_file,message,attachments[]?` | `run_id,disposition` | `disposition`：`started`（起跑）/`queued`（Busy 转插话队列）/`handled`（斜杠命令已消费，不登记运行） |
| POST | `/chat/stop` | — | `steering[],follow_up[]` | cancel 当前运行**并作废两条排队**：先把队里的文本取出来、`ClearQueued` 清引擎侧，再 `cancel()`；被作废的话术原样回给界面，前端据此放回输入框。没有运行中时不报错（`cancel == nil` 直接返回空队列）。作废了非空队列还会补一帧空 `chat:queue` |
| POST | `/chat/steer` | `message,attachments[]?`（复用 `ChatSendREQ`，`session_file` 不读） | `{ok}` | 进 steering 队，本轮收尾后注入；无运行中回 `5003` |
| POST | `/chat/followup` | `message,attachments[]?`（同上） | `{ok}` | 进 follow_up 队，本轮结束后另起一轮；无运行中回 `5003` |
| POST | `/chat/clear-queue` | — | `steering[],follow_up[]` | 返回被清掉的文本；引擎侧队列一并清 |
| POST | `/chat/remove-queued` | `queue_kind`（`steering`\|`followup`）、`queue_index` | `steering[],follow_up[]` | 移除后回剩余两队列；无运行中回 `5003` |
| POST | `/chat/compact` | `session_file,instructions` | `{ok}` | 同步总结上游（120s）；Busy 时 `5002` |
| POST | `/chat/answer` | `session_file,ask_id,value?,confirmed?,cancelled?,answers[]?` | `{ok}` | `answers[]` 项：`question_id,selected_options[],custom_input?` |
| POST | `/chat/new` | — | `session_file` | Busy 时 `5002` |
| POST | `/chat/clone` | — | `session_file` | 只复制消息条目；Busy 时 `5002` |
| POST | `/chat/fork` | `session_file,turn_id` | `session_file,fork_text?` | 复制到分叉点（含该条用户消息）；Busy 时 `5002`，分叉点不在会话里回 `5003` |
| POST | `/chat/export-html` | `session_file`（空则当前会话） | `file_name,file_content` | 自包含 HTML；无会话可导出回 `5003` |
| POST | `/chat/model` | `session_file,provider_name,model_id` | `{ok}` | 写 `model.active_provider` / `model.active_model`，广播 `chat:model` |
| POST | `/chat/thinking` | `session_file,thinking_level`（`off/minimal/low/medium/high`） | `{ok}` | 写 `model.thinking_level` |
| POST | `/chat/autocompact` | `session_file,auto_compaction` | `{ok}` | 写 `model.auto_compaction` |
| POST | `/chat/plan` | `plan_enabled`（复用 `ChatPlanREQ`，`session_file` 不读） | `plan_enabled,blocked_tools[],plan_message` | 状态落会话文件 `custom` 条目 |
| POST | `/messages/hide` | `session_file,turn_id` | `{ok}` | 台账在设置文件（`chat.hide:<路径>`），会话文件不动 |
| POST | `/messages/unhide` | `session_file,turn_id` | `{ok}` | 同上 |

`ChatStateRESP`：`engine_kind,session_file,work_dir,engine_running,streaming,model?,thinking_level,thinking_levels[],context_usage?,auto_compaction,queued,stats?,history[],todos[],plan_mode,plan_state?,commands[],caps,bind_err?,context_warn?,ask_pending?`。
- `thinking_levels[]` 是 `off/low/medium/high` 四档（`/chat/thinking` 接受的档位多一个 `minimal`）。
- `context_usage`：`usage_tokens,context_window,usage_percent`；`stats`：`session_id,user_messages,assistant_messages,tool_calls,total_messages,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,total_tokens,context_usage`。
  token 四个分项是真聚合：扫会话条目里每条消息的 `usage`，把 `input`/`output`/`cacheRead`/`cacheWrite` 各自累加（`Chat.usageSnapshot`）。
  `cost_usd` 仍恒为零值——聚合路径不累加花费，要算钱得逐条读会话里的 `usage.cost.total`。
- `context_warn`：自动压缩开着且占用过八成时给一句话提示，否则为空。
- `ask_pending`：引擎在等的提问清单。提问是阻塞式工具调用（`chat_service.ask` 挂进 waiters 等 `/chat/answer`），刷新界面后 `AskCard` 据此补回提问卡，模型不用被弃置在等待里。
- `queued`：`steering[],follow_up[]`；`plan_state`：`plan_enabled,blocked_tools[]`；`model?`：`model_id,model_name,provider_name,api_protocol,supports_reasoning,context_window,max_tokens,input_kinds?`。
- `commands[]`：`command_name,command_source,command_desc?,command_body?`；`history[]`：`turn_role,turn_blocks[],turn_model?,turn_usage?,turn_id?`。
- `ChatAttachmentVO`：`attachment_kind`（`file`\|`image`\|`session`）、`file_path?,file_name?,image_base64?,image_mime?,attachment_body?`；文件附件以路径引用折进消息，图片走内容块，会话引用整段正文折进消息。

导出有两种，只有一种走后端：

| 格式 | 谁生成 | 触发方式 | 内容口径 |
|---|---|---|---|
| HTML | 后端 `Sessions.Export`（`service/session_html.go`） | `POST /chat/export-html`，或斜杠命令 `exporthtml` | 自包含单文件，样式内联，文件名由后端按会话名给 |
| Markdown | **纯前端** `useBuiltinCommands.ts` 的 `exportMarkdown`，没有对应 HTTP 接口 | 只有斜杠命令 `export` | 遍历 `/chat/state` 给出的 `chat.history`，每轮打 `## 我` / `## 助手` 后贴 `block_kind="text"` 的正文，Blob 下载 `session-<时间戳>.md` |

Markdown 覆盖 `/chat/state` 返回的全部历史轮次，但只取 `block_kind="text"` 的正文：
思考块、工具卡片、差异与用量都不导出。要完整存档（含样式与工具块）用 HTML。两者都不改会话文件。

### 3.5 模型服务商（MODULES §4）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/models` | — | `models[]` | 行：`provider_name,model_id,model_name?,api_protocol?,context_window,max_tokens,supports_reasoning,input_kinds?` |
| GET | `/providers` | — | `providers[],config_path,read_error?` | 行：`provider_name,base_url,api_protocol,auth_kind,api_key_masked,has_key,models[],model_count`；密钥恒为掩码 |
| POST | `/providers/upsert` | `provider_name,base_url,api_protocol,auth_kind,api_key,models[]?` | `ProvidersRESP` | `models` 非 null 即整表替换对账 |
| POST | `/providers/delete` | `provider_name` | `ProvidersRESP` | 连带模型并清活跃/默认指针 |
| POST | `/providers/model/save` | `provider_name,model`（条目形状同 `ProviderModelEntryVO`） | `ProvidersRESP` | 按 `model_id` 定位，存在即改 |
| POST | `/providers/model/delete` | `provider_name,model_id` | `ProvidersRESP` | 指针被指向时一并清 |
| POST | `/providers/models` | `provider_name,base_url,api_protocol,api_key` | `probe_ok,model_count,models[]?,probe_message` | 列模型即连通性测试；失败不报错，用 `probe_ok:false` + `probe_message` 表达（错误原话里的密钥已掩码） |
| POST | `/providers/test` | 同上 | 同上 | 同一 handler |
| GET | `/models/defaults` | — | `default_provider,default_model,default_thinking_level,settings_file` | 新会话的起步设置 |
| POST | `/models/defaults` | `default_provider,default_model,default_thinking_level` | 同上 | 服务商与模型必须成对填写或成对留空，否则 `3000` |

模型条目 `ProviderModelEntryVO`：`model_id,model_name?,api_protocol?,supports_reasoning?,context_window,max_tokens,input_kinds?`。

`config_path` 是 `data\workbaby.db` 的绝对路径，**不是**某个 `models.json` 文件：服务商与模型存在 SQLite 里
（`service/models.go` 的 `Providers()` 回填 `Layout.DatabasePath()`），磁盘上没有同名 JSON 可看可改。
`domain/provider.go` 里该字段的注释写的是 `models.json 路径`，与本字段的实际取值不符，以本节为准。

### 3.6 技能（MODULES §5）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/skills` | — | `skills[],recommended_total,recommended_installed,available[]` | 三区磁盘扫描 + 未装推荐 |
| POST | `/skills/install` | `skill_dir_name` | `{ok}` | 只认推荐清单里的目录名 |
| GET | `/skills/content?path=` | `path` | `file_text` | 必须是 SKILL.md |
| POST | `/skills/save` | `skill_path,file_text` | `file_text` | 保存后回正文（`SkillContentRESP`） |
| POST | `/skills/toggle` | `skill_path,skill_enabled` | `{ok}` | 搬目录即开关 |
| POST | `/skills/create` | `skill_name,skill_desc,skill_body` | `skill_path` | 建 `data\skills\<清洗后名字>` |
| POST | `/skills/import` | `skill_from_path` | `skill_dir_name,skill_path` | 整目录复制，源目录之后删改不影响副本 |
| POST | `/skills/delete` | `skill_path` | `{ok}` | 传目录路径一律拒，只能删 SKILL.md |
| POST | `/skills/reset` | `skill_path` | `{ok}` | 从随包原件覆盖 |

`skills[]` 行：`skill_name,skill_dir_name,skill_desc,skill_path,skill_dir,skill_source,skill_group,skill_requires?,skill_enabled,skill_editable,skill_modified`；
`available[]` 行：`skill_dir_name,skill_group,skill_desc,skill_requires?`。

### 3.7 模板（MODULES §13）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/templates` | — | `rows[]` | 行：`template_name,template_desc,template_body,template_args` |
| POST | `/templates/save` | `template_name,template_desc,template_body,template_args` | `{ok,template_name}`（`PromptTemplateSaveRESP`） | 先 `sanitizeTemplateName`（小写、只留 `a-z0-9-`、去首尾 `-`）；清洗结果为空回 `1000`「模板名不能为空（只允许小写字母、数字与连字符）」。`template_name` 回的是**实际落盘的**名字，删除要用它——拿原文去删会找不到刚存下的那一份 |
| POST | `/templates/delete` | `template_name`（复用 `PromptTemplateREQ`，其余字段不读） | `{ok}` | 同名清洗后删除；幂等（不存在不算错），清洗后为空回 `1000` |

### 3.8 MCP（MODULES §6）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/mcp` | — | `servers[],user_path,project_path?,read_errors?` | 用户级 + 项目级合并，同名项目级覆盖 |
| POST | `/mcp/save` | `server_name,command_text,server_url,transport_kind,mcp_desc,work_dir,env_list[],mcp_timeout,mcp_scope`（`user`\|`project`） | `McpListRESP` | 后台重建引擎工具段 |
| POST | `/mcp/toggle` | `server_name,mcp_enabled` | `McpListRESP` | 同上触发重建 |
| POST | `/mcp/probe` | `server_name` | `probe_ok,probe_message,probe_tools` | 真握手（stdio 拉进程 / http 发请求） |
| POST | `/mcp/remove` | `server_name` | `McpListRESP` | 只删界面显示的那条 |

`servers[]` 行：`server_name,transport_kind,command_line?,server_url?,mcp_desc?,mcp_work_dir?,mcp_env_text?,mcp_timeout?,mcp_enabled,scope_kind,config_path`。

### 3.9 工作区文件（MODULES §7）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/files?path=` | `path` | `dir_path,entries[],truncated` | 行：`entry_name,entry_is_dir,size_bytes,modified_at` |
| GET | `/files/search?query=&limit=` | `query,limit` | `search_query,hits[],truncated` | 行：`file_path,entry_name`；只匹配文件名 |
| GET | `/files/read?path=` | `path` | `file_path,file_text,size_bytes,is_binary,file_kind,file_base64?,truncated` | 二进制可预览回 base64 |
| POST | `/files/save` | `file_path,file_text` | `{ok}` | 父目录一并建，5MB 上限 |
| POST | `/files/mkdir` | `file_path` | `{ok}` | 已存在拒 |
| POST | `/files/delete` | `file_path` | `{ok}` | 递归；工作区根拒删 |
| POST | `/files/rename` | `file_path,new_file_path` | `{ok}` | 源必须存在、目标必须不存在 |

### 3.10 Git（MODULES §8）

写接口一律回最新 `GitStatusRESP`：`git_available,unavailable_reason?,is_repo,workspace,repo_root,branch_name,upstream_ref,ahead,behind,repo_state,repo_state_detail?,remotes[],changed_files[],has_conflict,git_version`；
`remotes[]`：`remote_name,remote_url`；`changed_files[]`：`file_path,change_code,is_staged,has_conflict,insertions,deletions`。

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/git/status` | — | `GitStatusRESP` | `porcelain -z` 解析 |
| GET | `/git/diff?path=` | `path` | `file_path,truncated,dropped,diff_lines[]` | 行：`line_kind,old_no,new_no,line_text`；上限 4000 行 |
| GET | `/git/history?limit=` | `limit` | `commits[]` | 行：`commit_hash,short_hash,commit_subject,author_name,commit_time,file_count,insertions,deletions` |
| GET | `/git/commit?hash=` | `hash` | `commit,commit_body,parent_hashes[],changed_files[],diff` | 单提交详情 |
| GET | `/git/file-history?path=&limit=` | `path,limit` | `commits[]` | 同 history 形状 |
| GET | `/git/file-diff?hash=&path=` | `hash,path` | `GitDiffRESP` | 单提交里某文件的差异 |
| GET | `/git/branches` | — | 裸数组 `[]GitBranchVO`（不套对象） | 行：`branch_name,is_current,remote_name?,ahead,behind`；本地与远端分支同列 |
| GET | `/git/stash/list` | — | `stashes[]` | 行：`stash_ref,stash_subject`，最新在前 |
| POST | `/git/stage` | `file_paths[]`（`GitPathsREQ`） | `GitStatusRESP` | 加入暂存 |
| POST | `/git/unstage` | `file_paths[]`（`GitPathsREQ`） | `GitStatusRESP` | 移出暂存 |
| POST | `/git/discard` | `file_paths[]`（`GitPathsREQ`） | `GitStatusRESP` | 丢弃工作区改动（未跟踪文件直接删） |
| POST | `/git/resolve` | `file_paths[]`（`GitPathsREQ`） | `GitStatusRESP` | 把冲突文件标记为已解决；与下一条不是同一个接口 |
| POST | `/git/conflict/resolve` | `file_path,resolve_side`（`GitConflictREQ`；`ours`\|`theirs`\|`""` = 我改完了） | `GitStatusRESP` | 按口径解决**一个**冲突文件；入参是单文件，不是 `file_paths[]` |
| POST | `/git/commit` | `commit_message,is_amend` | `status,commit_hash` | — |
| POST | `/git/branch` | `branch_name`（复用 `GitBranchREQ`，`is_rebase` 不读） | `GitStatusRESP` | 新建分支 |
| POST | `/git/branch/delete` | `branch_name`（同上） | `GitStatusRESP` | — |
| POST | `/git/switch` | `branch_name`（同上） | `GitStatusRESP` | — |
| POST | `/git/merge` | `branch_name,is_rebase` | `GitStatusRESP` | 冲突落 `repo_state` |
| POST | `/git/state` | `state_action`（`continue`\|`abort`） | `GitStatusRESP` | 冲突收尾 |
| POST | `/git/init` | `branch_name` | `GitStatusRESP` | — |
| POST | `/git/remote` | `remote_name,remote_url` | `GitStatusRESP` | — |
| POST | `/git/push` | `remote_name,branch_name,force_mode,set_upstream,confirm`（`GitSyncREQ`，`is_rebase` 不读） | `GitStatusRESP` | `remote_name` 空按 `origin`；`force_mode` 取 `""`/`none`/`with-lease`，`with-lease` 需 `confirm` 等于分支名，裸 `--force` 不提供 |
| POST | `/git/pull` | `remote_name,branch_name,is_rebase`（同 `GitSyncREQ`，推送字段不读） | `GitStatusRESP` | — |
| POST | `/git/fetch` | `remote_name`（同 `GitSyncREQ`，只读这一个） | `GitStatusRESP` | 只更新远端引用 |
| POST | `/git/stash/push` | `stash_message`（`GitStashREQ`） | `GitStatusRESP` | — |
| POST | `/git/stash/pop` | — | `GitStatusRESP` | 恢复最新一条 stash，不带入参 |
| POST | `/git/stash/drop` | `stash_ref,confirm`（`GitStashREQ`） | `GitStatusRESP` | `confirm` 必须等于 `stash_ref` |

### 3.11 预索引（MODULES §9）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/index/status` | — | `index_workspace,index_doc_count,index_built,index_building,index_built_at` | 状态卡数据源 |
| POST | `/index/search` | `search_query,search_limit` | `search_hits[],index_status` | 命中行：`doc_path,doc_kind,doc_symbol?,doc_symbol_kind?,doc_line?,hit_score,hit_reason?` |
| POST | `/index/rebuild` | — | `IndexStatusVO` | 立即受理（`index_building` 置位）即返回；重建在后台协程跑，`GET /index/status` 轮询到 `index_built` 为止；`building` 期间幂等受理 |

### 3.12 联网（MODULES §10）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/web/status` | — | `web_kind,web_has_key,web_message,web_kinds[]` | 未配密钥时 `web_message` 给指引 |
| POST | `/web/settings` | `web_kind,web_api_key` | 同上 | `web_api_key` 空串 = 保留原值 |
| POST | `/web/search` | `search_query,search_limit` | `search_results[],search_provider` | 行：`result_title,result_url,result_snippet,result_source?` |
| POST | `/web/fetch` | `fetch_url` | `fetch_url,fetch_text,fetch_char_count` | 按 Content-Type 分流（PDF 无文字回 `5003`） |

### 3.13 事件（MODULES §2）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/events?since=&token=` | `since`（上次 seq）、`token` | `text/event-stream` | 首帧 `stream:open` 带当前 seq（不占新号）；心跳是 `: ping` 注释行，不占 seq |

## 4. SSE 事件全表

这一节讲什么：服务端会把哪些事件广播出去、每帧带什么字段、什么时候发。

- 连接与帧：`event:` 行恒为 `message`，`data:` 是整帧 JSON `{"seq":n,"event":"chat:start","data":{...},"sent_at":ms}`；
  真正的语义事件名在帧体内 `event` 字段。响应头：`text/event-stream` + `no-cache` + `Connection: keep-alive`。
- seq 语义：hub 单调发号，每广播一次 +1；`stream:open` 报当前 seq 但不占新号。
- since 补帧：订阅时回放缓冲区里 `seq > since` 的帧，缓冲区保留最近 1024 帧；
  缺口超出缓冲区由前端拿 `/chat/state` 全量对账，服务端不假装补齐。
- 心跳：每 25s 写一行 `: ping\n\n` 注释行，不占 seq；连接断开即停。
- 多数事件经 `sse.EngineTag` 补上 `engine_kind:"native"`（含 chat 帧与 `session:info/session:title/chat:model/chat:queue/chat:ask/workspace:changed/mcp:changed/models:changed/skill:status/engine:detected`）；
  `chat:compaction` 在载荷里手工放 `engine_kind`；`chat:plan/engine:ready/app:close_requested` 与 `stream:open` 不带。
  经 `Stream.emit` 出去的 `chat:start/live/patch/turn/turn_end/todos/error/settled/done` 额外带 `run_id`，其余事件不带。

| 事件 | 载荷字段 | 触发时机 | 备注 |
|---|---|---|---|
| `stream:open` | （空载荷） | 连接建立后首帧 | 报当前 seq，供下次 `since` 用 |
| `chat:start` | `started_at,run_id` | 一轮起跑（绑定会话与模型成功后） | — |
| `chat:live` | `phase`（`text`\|`thinking`\|`tool`）、`output_tokens`；`phase:tool` 时另有 `tool_name,tool_call_id` | 每次正文/思考增量、工具开始执行 | `output_tokens` 是当轮累计 |
| `chat:patch` | `mode`（`delta`\|`block`）、`index,kind`（块类型：`text`\|`thinking`\|`tool`）；`delta` 或 `block` 二选一 | 流式块增量或整块替换；工具结束时整块重发 | `block` 是 `ChatBlockVO`；前端 `readPatch` 读 `kind` |
| `chat:turn` | `user_message,turn_blocks[]` | 一条助手消息落盘，整表覆盖块表 | — |
| `chat:turn_end` | `turn_usage` | 紧随 `chat:turn` | 本轮 token 读数 |
| `chat:todos` | `todos[]`（`todo_id,todo_subject,todo_status,todo_active_form?`） | todo 工具执行结束且结果带任务清单 | — |
| `chat:ask` | `ask_id,ask_method,ask_title?,ask_message?,ask_placeholder?,ask_prefill?,ask_options?` | 引擎发起提问等待回答 | — |
| `chat:error` | `message` | 本轮运行失败 | — |
| `chat:settled` | `aborted` | 运行收尾（含中止） | 与 `chat:done` 背靠背各发一帧 |
| `chat:done` | `aborted` | 同上 | 界面据此收尾并重拉 state 与列表 |
| `chat:compaction` | `state`（`start`\|`done`）、`engine_kind` | 手动/自动压缩开始与结束 | 不带 `run_id` |
| `chat:queue` | `steering[],follow_up[]` | 插话/追加入队、单条移除、清空之后；`/chat/stop` 作废了非空排队时也补一帧空队列 | 界面整体替换队列镜像 |
| `chat:model` | `provider_name,model_id` | 切模型成功后 | 界面就地更新模型身份并重拉清单（stores/events.ts） |
| `chat:plan` | `plan_enabled,session_id` | 计划模式开关落盘后 | 界面更新计划态镜像 |
| `chat:retry` | `attempt,max,delay_ms` | 上游调用失败且可重试，进入退避等待 | — |
| `chat:retry_end` | （空载荷） | 重试结束后（成功或放弃） | — |
| `session:info` | `engine_kind`（斜杠命令路径的一处广播为空载荷） | 新建/克隆/分叉会话成功后 | 界面重拉会话列表 |
| `session:title` | `title` | 会话改名成功后（`Chat.RenameAny` 广播，`/sessions/rename` 经引擎写锁追加条目后发出） | 界面刷新会话列表 |
| `workspace:changed` | `workspace` | 切工作区成功后 | 界面重载工作区态与文件面 |
| `mcp:changed` | `engine_kind` | MCP 保存/开关/删除后；随后后台重建引擎工具段 | 界面重拉 MCP 清单 |
| `models:changed` | `engine_kind` | 服务商/模型任何写操作成功后 | 界面重拉模型下拉 |
| `skill:status` | `skill_dir_name`（必有）+ 动作标志之一：`skill_enabled`/`skill_reset`/`skill_imported`/`skill_deleted` | 六个技能写动作（安装/开关/重置/导入/新建/删除）成功后各发一帧 | — |
| `engine:detected` | `runtime` | `/engines/detect`、`/engines/repair` 重新探测后 | — |
| `engine:ready` | （空载荷） | `/engines/restart` 中止当前运行后 | 不带 `engine_kind` |
| `app:close_requested` | `close_action` | 用户关窗且未记住关窗行为时 | 不带 `engine_kind` |

后端从不广播、前端也已无分支的事件：`engine:status` `engine:exit` `engine:commands`
`app:progress` `chat:blocks` `chat:notify` `chat:status` `chat:widget` `chat:editor_text`。
排障时不要据此判断后端故障。

## 5. 错误码

这一节讲什么：全部业务码、各自的触发条件与按模块的分布。码值与 `backend/pkg/codes.go` 一一对应，新增码先改文档再写代码。

### 5.1 按段位（`backend/pkg/codes.go`）

段位含义：1000–1999 通用/文件/git；2000–2999 持久化；3000–3999 服务商/模型；
5000–5999 会话运行；7000–7999 外部进程；8000–8999 技能；9100–9199 MCP。

| 码 | 常量 | 含义 | 什么情况下会回 |
|---|---|---|---|
| `0` | CodeOK | 成功 | 唯一不视为异常的值 |
| `1000` | CodeBadRequest | 入参不合法 | body 解析失败（`bind`）；空值与未知枚举：空路径/空标题/空消息、`queue_kind`/`thinking_level`/`force_mode`/`resolve_side`/配置名/搜索服务名不认识 |
| `1001` | CodeFileNotFound | 文件或目录不存在 | files、system 的目标不存在；`/settings/workspace` 指向的目录不存在 |
| `1002` | CodePathOutside | 路径越界 | files 的 `guard`（不在工作区内）；web 抓取的内网地址拒（同码不同义，看 message） |
| `1003` | CodeFileIO | 文件读写失败 | files、templates、skills 落盘、system 打开/定位、prefs 自启的平台动作 |
| `1100` | CodeGitMissing | git 不可用 | git 可执行文件探测不到 |
| `1101` | CodeGitFailed | git 命令失败 | git 命令非零退出，`detail` 是 stderr |
| `1102` | CodeGitAuth | 凭证被拒 / 无法输入 | git 需要交互式登录（`GIT_TERMINAL_PROMPT=0` 下没有终端可弹） |
| `2002` | CodeStorage | 本地存储失败 | JSON 台账与 SQLite 的读写（repo 层） |
| `3000` | CodeProviderBad | 服务商配置不合法 | 名称/地址为空、服务商不存在、缺密钥、默认服务商与模型不成对 |
| `3001` | CodeNoProvider | 没有可用服务商 / 模型 | chat 的 `resolveModel`：没选模型，或活跃指针指向已删对象（指针会被顺手清掉） |
| `5000` | CodeSessionBind | 未绑定会话或工作区 | chat、files、git、searcher 的「请先选择工作区」 |
| `5001` | CodeSessionIO | 会话文件读写失败 | agent 的 `Session.append` 与 parser 两处都用；会话删除/改名时的路径越界 |
| `5002` | CodeEngineBusy | 有任务占用引擎 / 构建中 | chat 的切会话/新建/克隆/分叉/压缩；运行未收尾时删会话（`PrepareDelete` 先停轮再删）；索引重建期间的搜索（`searcher`） |
| `5003` | CodeNotFound | 目标不存在 | 分叉点不在会话里、无运行中的插话/追加/移除、无会话可导出、模型不存在、PDF 无文字、会话文件不存在 |
| `7000` | CodeProvision | 外部进程 / 上游调用失败 | web 抓取、压缩总结调上游、子进程启动失败 |
| `8000` | CodeSkillBad | 技能路径或内容不合法 | 目录不存在、无 SKILL.md、重名、名字清洗后为空 |
| `8001` | CodeSkillSave | 技能保存失败 | 安装/保存/开关/重置/导入的文件操作失败 |
| `8002` | CodeSkillDelete | 技能删除失败 | 删目录被拒、项目技能不许在界面删 |
| `9100` | CodeMcpRead | `mcp.json` 读取失败 | mcp |
| `9101` | CodeMcpWrite | `mcp.json` 写入失败 | mcp；也用于服务名不合法、条目不存在等写路径校验 |
| `9102` | CodeMcpCall | MCP 工具调用失败 | 引擎侧 `tools/call`，进工具结果正文，不进 HTTP 信封 |
| `9103` | CodeMcpServer | MCP 服务进程不可达 | 引擎侧；`StartPersistent` 写不进 stdin 时也回它；启动失败归 `7000` |
| `9104` | CodeMcpTimeout | MCP 请求超预算 | 引擎侧 mcp_client |
| `1404` | CodeUnknownRoute | 未知路由 | `NoRoute`（信封形态） |
| `9999` | CodeInternal | 未归类内部错误 | 兜底 wrap：读 exe 路径失败、端口监听失败、注册表写自启失败 |

### 5.2 按模块

各 POST 路由 body 解析失败一律 `1000`，下表不重复列；表里是模块自身会回的码。

| 模块 | 可能出现的码 |
|---|---|
| 引导 / 设置 | 1000, 1001, 1003, 2002, 9999 |
| 引擎 / 系统 | 1000, 1001, 1003, 1404, 9999 |
| 会话 | 1000, 5001, 5002, 5003 |
| 对话 | 1000, 3000, 3001, 5000, 5001, 5002, 5003, 7000 |
| 模型 | 2002, 3000, 5003 |
| 技能 | 1000, 8000, 8001, 8002 |
| MCP | 9100, 9101 |
| 工作区文件 | 1001, 1002, 1003, 5000 |
| Git | 1000, 1100, 1101, 1102, 5000 |
| 预索引 | 1003, 2002, 5000 |
| 联网 | 1000, 1002, 5003, 7000 |
| 模板 | 1000, 1003 |
| 事件流 | 无（不走信封；鉴权失败回裸 401） |

`9102`/`9103`/`9104` 只出现在引擎侧的 MCP 工具结果正文里，不进任何 HTTP 信封——
MCP 的 HTTP 接口只回 `9100`/`9101`。

## 6. 怎么验

这一节讲什么：两条现成的验证通道，各自覆盖什么、不覆盖什么。

- `go test ./tests -run TestHTTPContract`：五个子测试各自钉一段链路。
  - 鉴权门：无令牌与错令牌都回 401，且 body 不含 `"code"`（不带信封）。
  - CORS 预检：OPTIONS 回 204 且带允许源与 `X-Desktop-Token` 允许头；带源的正常请求响应也带允许头。
  - 信封同构：未知路由回 HTTP 200 + `1404`；坏 body 也回信封（`1000`）。
  - 写入落盘：`/settings/save` 真写台账并读回，`/bootstrap` 必须带 `app_contract`。
  - SSE 通道：`?token=` 连上后 `stream:open` 必达，广播一帧测试事件必达，seq 单调。
- `pwsh scripts/smoke.ps1`：先 `task build` 产出 `bin\WorkBaby.exe`；脚本起无窗后端，
  把 18 个免参数只读接口全打一遍（HTTP 200 且 `code=0`），另验无令牌回 401。
  它只验接线不验业务——路由名拼错、service 没接上会在这里露出来，业务正确性归 `tests/`。
