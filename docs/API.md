# API 契约

字段名全部取自 `backend/domain/*.go` 的 json tag。行为约束（空串语义、上限、落盘副作用、排障顺序）
只写在 `docs/MODULES.md` 对应章节，本文不复述。

## 1. 信封与鉴权

- baseURL：`http://127.0.0.1:{port}/api/v1`。端口与令牌由 Wails 绑定 `Endpoint()` 回填（`wb_base_url` 已含前缀）。
- 鉴权：HTTP 走 `X-Desktop-Token` 头；SSE 走 `?token=`（EventSource 带不了自定义头）。令牌每次启动随机。
- CORS：`gin.Recovery` → CORS → 鉴权 → 路由 → `NoRoute`。预检 OPTIONS 不带令牌，故 CORS 必须排在鉴权之前，
  直接回 204 + 允许头。允许源限 `http(s)://wails.localhost` 与 `localhost`/`127.0.0.1`（真正的门禁是令牌）。
- **只允许 GET / POST**；动词靠路径段表达（`POST /sessions/delete`）。`GET` 不写盘（唯一例外：`/chat/state` 会隐式初始化会话，见 `docs/ARCHITECTURE.md` §3）。
- **恒回 200**，业务失败看信封 `code`；`code` 是数字字符串。

```json
{ "code": "0", "message": "", "detail": "可选原话", "data": { } }
```

`message` 直接给用户看，`detail` 只进诊断面板。未知接口回 `1404`。鉴权失败是唯一例外：回 **401 且不带信封**。

## 2. 参数与出参约定

- 路径参数：没有路径段变量，一律查询参数（`?path=` `?session=` `?hash=` `?limit=` `?q=` `?name=` `?since=`）。
- 查询参数缺省值由 handler 兜底：`limit` 50（`/git/file-history` 缺省 40 上限 200、`/git/history` 缺省 50 上限 500、`/logs` 500、`/files/search` 缺省 50 上限 200）。
- 整数查询参数解析失败按缺省值走，不报错（`queryInt`）。
- POST body 一律 JSON；解析失败回 `1000`，`detail` 是解析器原话（`bind`）。
- 出参字段名一律 snake_case + 业务前缀；时间戳 millisecond int64（会话文件内的 ISO 串是落盘格式，不进 DTO）。
- 只有成功标志的接口回 `{"ok": true}`（`OkRESP`）。

## 3. 路由表

### 3.1 引导 / 设置 / 偏好（MODULES §12）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/bootstrap` | — | `app_version,app_contract,engines[],active_engine,workspace,settings{},layouts[],workspaces[],runtime,prefs` | 首屏总包，`app_contract` 当前为 3 |
| GET | `/settings` | — | 键值 map | 全部设置 |
| POST | `/settings/save` | 键值 map | 键值 map | 只对 `ui.mention_sessions` 做 1~20 收敛 |
| POST | `/settings/workspace` | `workspace` | `workspace` | 会话绑定随之失效 |
| GET | `/workspaces` | — | `workspace,workspace_rows[],active_engine` | 行含目录存在/git/分支/改动数/会话数 |
| POST | `/settings/workspace/forget` | `workspace_path` | `workspace,workspace_rows[],active_engine` | 只去历史，不删磁盘 |
| GET | `/prefs` | — | `pref_values,auto_start,restore_session,last_session_file,close_action,data_dir,log_dir` | `auto_start` 以注册表为准 |
| POST | `/prefs/autostart` | `pref_enabled` | `AppPrefsVO` | 写 HKCU Run 项 |
| POST | `/prefs/restore` | `pref_enabled` | `AppPrefsVO` | 只记设置，执行者是前端 |
| POST | `/prefs/close-action` | `close_action` | `AppPrefsVO` | 非 `quit` 一律按 `tray` |
| POST | `/prefs/data-dir` | — | `{ok}` | 资源管理器打开数据目录 |
| GET | `/logs?lines=` | `lines` | `lines[]` | 当日日志尾部 |

### 3.2 引擎 / 应用 / 健康 / 系统（MODULES §14）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/engines` | — | `engines[],layouts[],runtime,instance,detected_at` | `instance` 现造：`engine_kind,running,busy,session_file,work_dir,started_at` |
| POST | `/engines/detect` | — | `runtime,apps[]` | 广播 `engine:detected` |
| POST | `/engines/repair` | — | `runtime,apps[]` | 与 detect 完全等价 |
| POST | `/engines/restart` | — | `{ok}` | `Chat.Stop()` + 广播 `engine:ready` |
| GET | `/health/engines` | — | `health_engines[],active_engine,workspace,runtime,total_sessions,total_skills,total_mcp,live_engines,memory_used_mb,detected_at` | 概览页首屏；`health_engines[]` 实际填的是 `engine_kind,engine_label,engine_version,installed,running,busy,session_count,skill_count,mcp_count,config_files,caps`，`engine_exe/process_id/started_at/call_count/last_error` 与顶层 `live_engines` 恒为零值（进程内没有子进程可报） |
| GET | `/apps` | — | `apps[]` | 行：`app_key,app_name,app_version,app_path,installed,app_source,install_note?`；`required`/`can_locate`/`blocker?` 恒为零值（探测只报装没装与装在哪） |
| GET | `/config/files` | — | `entries[]` | 行：`config_name,config_label,file_path,file_exists,size_bytes` |
| GET | `/config/text?name=` | `name`（`settings`\|`mcp`） | `file_path,file_text,truncated,found` | 白名单，不接受外部路径 |
| POST | `/system/reveal` | `target_path` | `{ok}` | 资源管理器定位 |
| POST | `/system/open` | `target_path` | `{ok}` | 系统默认程序打开 |

### 3.3 会话（MODULES §3）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/sessions?workspace=&q=` | `workspace`、`q` | `sessions[]` | 行：`session_id,session_path,session_title,session_cwd,session_model,updated_at,size_bytes,message_count,total_tokens,cost_usd`；`message_count` 是消息条数（用户 + 助手 + 工具结果各算一条），不是对话轮数 |
| GET | `/sessions/detail?path=` | `path` | `session,items[]` | 条目：`item_kind,item_role,item_text,item_model,tool_name,tool_call_id,tool_is_error,created_at,item_tokens,cost_usd` |
| POST | `/sessions/delete` | `session_path` | `{ok}` | 一并清 `chat.hide:<路径>` 键 |
| POST | `/sessions/rename` | `session_path,session_title` | `{ok}` | 追加 `session_info` 条目；被引擎打开时 5002 |

### 3.4 对话（MODULES §1）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/chat/state?session=` | `session` | `ChatStateRESP`（见下） | 无会话时会隐式建会话并写 `prefs.last_session_file` |
| POST | `/chat/send` | `session_file,message,attachments[]` | `run_id,disposition` | `disposition` = `started`/`queued`/`handled` |
| POST | `/chat/stop` | — | `{ok}` | 只 cancel，不等待 |
| POST | `/chat/steer` | `message,attachments` | `{ok}` | 进 steering 队，本轮收尾后注入 |
| POST | `/chat/followup` | `message,attachments` | `{ok}` | 进 follow_up 队，本轮结束后另起一轮 |
| POST | `/chat/clear-queue` | — | `steering[],follow_up[]` | 返回被清掉的文本 |
| POST | `/chat/remove-queued` | `queue_kind,queue_index` | `steering[],follow_up[]` | `queue_kind` = `steering`\|`followup` |
| POST | `/chat/compact` | `session_file,instructions` | `{ok}` | 同步总结上游（120s）；Busy 时 5002 |
| POST | `/chat/answer` | `session_file,ask_id,value,confirmed,cancelled,answers[]` | `{ok}` | `answers[]` 项：`question_id,selected_options[],custom_input` |
| POST | `/chat/new` | — | `session_file` | Busy 时 5002 |
| POST | `/chat/clone` | — | `session_file` | 只复制消息条目 |
| POST | `/chat/fork` | `session_file,turn_id` | `session_file,fork_text?` | 复制到分叉点（含该条用户消息） |
| POST | `/chat/export-html` | `session_file` | `file_name,file_content` | 自包含 HTML |
| POST | `/chat/model` | `provider_name,model_id` | `{ok}` | 写 `model.active_*`，广播 `chat:model` |
| POST | `/chat/thinking` | `thinking_level` | `{ok}` | `off/minimal/low/medium/high`，写 `model.thinking_level` |
| POST | `/chat/autocompact` | `auto_compaction` | `{ok}` | 写 `model.auto_compaction` |
| POST | `/chat/plan` | `plan_enabled` | `plan_enabled,blocked_tools[],plan_message` | 状态落会话文件 `custom` 条目 |
| POST | `/messages/hide` | `session_file,turn_id` | `{ok}` | 台账在设置文件，会话文件不动 |
| POST | `/messages/unhide` | `session_file,turn_id` | `{ok}` | 同上 |

`ChatStateRESP`：`engine_kind,session_file,work_dir,engine_running,streaming,model,thinking_level,thinking_levels[],context_usage,context_warn,auto_compaction,queued,stats,history[],todos[],plan_mode,plan_state,commands[],caps,bind_err`。
`context_usage`：`usage_tokens,context_window,usage_percent`；`stats`：`session_id,user_messages,assistant_messages,tool_calls,total_messages,total_tokens,context_usage`。`input_tokens`/`output_tokens`/`cache_read_tokens`/`cache_write_tokens`/`cost_usd` 恒为零值——聚合只累加总 token 数，明细读数要看会话里的每条消息。
`queued`：`steering[],follow_up[]`；`plan_state`：`plan_enabled,blocked_tools[]`；`model`：`model_id,model_name,provider_name,api_protocol,supports_reasoning,context_window,max_tokens,input_kinds`。
`commands[]`：`command_name,command_source,command_desc,command_body`；`history[]`：`turn_role,turn_blocks[],turn_model,turn_usage,turn_id`。
`ChatAttachmentVO`：`attachment_kind(file|image|session),file_path,file_name,image_base64,image_mime,attachment_body`。

### 3.5 模型服务商（MODULES §4）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/models` | — | `models[]` | 行：`provider_name,model_id,model_name,api_protocol,context_window,max_tokens,supports_reasoning,input_kinds` |
| GET | `/providers` | — | `providers[],config_path,read_error?` | 行：`provider_name,base_url,api_protocol,auth_kind,api_key_masked,has_key,models[],model_count` |
| POST | `/providers/upsert` | `provider_name,base_url,api_protocol,auth_kind,api_key,models[]` | `ProvidersRESP` | `models` 非 nil 即整表替换对账 |
| POST | `/providers/delete` | `provider_name` | `ProvidersRESP` | 连带模型并清活跃/默认指针 |
| POST | `/providers/model/save` | `provider_name,model{…}` | `ProvidersRESP` | 按 `model_id` 定位，存在即改 |
| POST | `/providers/model/delete` | `provider_name,model_id` | `ProvidersRESP` | 指针被指向时一并清 |
| POST | `/providers/models` | `provider_name,base_url,api_protocol,api_key` | `probe_ok,probe_message,model_count,models[]` | 列模型即连通性测试 |
| POST | `/providers/test` | 同上 | 同上 | 同一 handler |
| GET | `/models/defaults` | — | `default_provider,default_model,default_thinking_level,settings_file` | 发送链路不消费这三键 |
| POST | `/models/defaults` | `default_provider,default_model,default_thinking_level` | 同上 | 服务商与模型必须成对填写 |

模型条目 `ProviderModelEntryVO`：`model_id,model_name,api_protocol,context_window,max_tokens,supports_reasoning,input_kinds[]`。

### 3.6 技能（MODULES §5）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/skills` | — | `skills[],recommended_total,recommended_installed,available[]` | 三区磁盘扫描 + 未装推荐 |
| POST | `/skills/install` | `skill_dir_name` | `{ok}` | 只认推荐清单里的目录名 |
| GET | `/skills/content?path=` | `path` | `file_text` | 必须是 SKILL.md |
| POST | `/skills/save` | `skill_path,file_text` | `{ok}` | 同上 |
| POST | `/skills/toggle` | `skill_path,skill_enabled` | `{ok}` | 搬目录即开关 |
| POST | `/skills/create` | `skill_name,skill_desc,skill_body` | `skill_path` | 建 `data\skills\<name>` |
| POST | `/skills/import` | `skill_from_path` | `skill_dir_name,skill_path` | 整目录复制 |
| POST | `/skills/delete` | `skill_path` | `{ok}` | 传目录路径一律拒 |
| POST | `/skills/reset` | `skill_path` | `{ok}` | 从随包原件覆盖 |

`skills[]` 行：`skill_name,skill_dir_name,skill_desc,skill_path,skill_dir,skill_source,skill_group,skill_requires,skill_enabled,skill_editable,skill_modified`；
`available[]` 行：`skill_dir_name,skill_group,skill_desc,skill_requires`。

### 3.7 模板（MODULES §13）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/templates` | — | `rows[]` | 行：`template_name,template_desc,template_body,template_args` |
| POST | `/templates/save` | `template_name,template_desc,template_body,template_args` | `{ok}` | 名字清洗后为空即拒 |
| POST | `/templates/delete` | `template_name` | `{ok}` | 幂等 |

### 3.8 MCP（MODULES §6）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/mcp` | — | `servers[],user_path,project_path?,read_errors?` | 用户级 + 项目级合并，同名项目级覆盖 |
| POST | `/mcp/save` | `server_name,command_text,server_url,transport_kind,mcp_desc,work_dir,env_list[],mcp_timeout,mcp_scope` | `McpListRESP` | `mcp_scope` = `user`\|`project`；后台重建工具段 |
| POST | `/mcp/toggle` | `server_name,mcp_enabled` | `McpListRESP` | 同上触发重建 |
| POST | `/mcp/probe` | `server_name` | `probe_ok,probe_message,probe_tools` | 真握手（stdio 拉进程 / http 发请求） |
| POST | `/mcp/remove` | `server_name` | `McpListRESP` | 只删界面显示的那条 |

`servers[]` 行：`server_name,transport_kind,command_line,server_url,mcp_desc,mcp_work_dir,mcp_env_text,mcp_timeout,mcp_enabled,scope_kind,config_path`。

### 3.9 工作区文件（MODULES §7）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
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

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/git/status` | — | `GitStatusRESP` | `porcelain -z` 解析 |
| GET | `/git/diff?path=` | `path` | `file_path,truncated,dropped,diff_lines[]` | 行：`line_kind,old_no,new_no,line_text`；上限 4000 行 |
| GET | `/git/history?limit=` | `limit` | `commits[]` | 行：`commit_hash,short_hash,commit_subject,author_name,commit_time,file_count,insertions,deletions` |
| GET | `/git/commit?hash=` | `hash` | `commit,commit_body,parent_hashes[],changed_files[],diff` | 单提交详情 |
| GET | `/git/file-history?path=&limit=` | `path,limit` | `commits[]` | 同 history 形状 |
| GET | `/git/file-diff?hash=&path=` | `hash,path` | `GitDiffRESP` | 单提交里某文件的差异 |
| GET | `/git/branches` | — | `[]GitBranchVO`（裸数组，不套对象） | 行：`branch_name,is_current,remote_name?,ahead,behind`；本地与远端分支同列 |
| GET | `/git/stash/list` | — | `stashes[]` | 行：`stash_ref,stash_subject`，最新在前 |
| POST | `/git/stage` `/git/unstage` `/git/discard` `/git/resolve` | `file_paths[]` | `GitStatusRESP` | 四者共用 `GitPathsREQ` |
| POST | `/git/conflict/resolve` | `file_path,resolve_side` | `GitStatusRESP` | `resolve_side` = `ours`\|`theirs`\|`""`（我改完了） |
| POST | `/git/commit` | `commit_message,is_amend` | `status,commit_hash` | — |
| POST | `/git/branch` `/git/branch/delete` `/git/switch` | `branch_name` | `GitStatusRESP` | — |
| POST | `/git/merge` | `branch_name,is_rebase` | `GitStatusRESP` | 冲突落 `repo_state` |
| POST | `/git/state` | `state_action` | `GitStatusRESP` | `continue`\|`abort` |
| POST | `/git/init` | `branch_name` | `GitStatusRESP` | — |
| POST | `/git/remote` | `remote_name,remote_url` | `GitStatusRESP` | — |
| POST | `/git/push` | `remote_name,branch_name,force_mode,set_upstream,confirm` | `GitStatusRESP` | `force_mode` = `""`/`none`/`with-lease`；`with-lease` 需 `confirm` 等于分支名，裸 `--force` 不提供 |
| POST | `/git/pull` | `remote_name,branch_name,is_rebase` | `GitStatusRESP` | — |
| POST | `/git/fetch` | `remote_name` | `GitStatusRESP` | 只更新远端引用 |
| POST | `/git/stash/push` | `stash_message` | `GitStatusRESP` | — |
| POST | `/git/stash/pop` | `stash_ref` | `GitStatusRESP` | — |
| POST | `/git/stash/drop` | `stash_ref,confirm` | `GitStatusRESP` | `confirm` 必须等于 `stash_ref` |

### 3.11 预索引（MODULES §9）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/index/status` | — | `index_workspace,index_doc_count,index_built,index_building,index_built_at` | 状态卡数据源 |
| POST | `/index/search` | `search_query,search_limit` | `search_hits[],index_status` | 命中行：`doc_path,doc_kind,doc_symbol,doc_symbol_kind,doc_line,hit_score,hit_reason` |
| POST | `/index/rebuild` | — | `IndexStatusVO` | 同步重建，`building` 期间幂等返回 |

### 3.12 联网（MODULES §10）

| 方法 | 路径 | 入参 | 出参 data | 语义 |
|---|---|---|---|---|
| GET | `/web/status` | — | `web_kind,web_has_key,web_message,web_kinds[]` | 未配密钥时 `web_message` 给指引 |
| POST | `/web/settings` | `web_kind,web_api_key` | 同上 | `web_api_key` 空串 = 保留原值 |
| POST | `/web/search` | `search_query,search_limit` | `search_results[],search_provider` | 行：`result_title,result_url,result_snippet,result_source` |
| POST | `/web/fetch` | `fetch_url` | `fetch_url,fetch_text,fetch_char_count` | 按 Content-Type 分流 |

### 3.13 事件（MODULES §2）

| 方法 | 路径 | 入参 | 出参 | 语义 |
|---|---|---|---|---|
| GET | `/events?since=&token=` | `since`（上次 seq）、`token` | `text/event-stream` | 首帧 `stream:open` 带当前 seq（不占新号）；心跳是 `: ping` 注释行，不占 seq |

## 4. SSE 事件全表

帧：`{seq,event,data,sent_at}`，`event` 在帧体内（SSE 的 event 字段恒为 `message`），`data` 是 map。
chat 帧额外带 `engine_kind:"native"` 与 `run_id`。

| 事件 | 载荷字段 | 发射点 | 前端反应 | 后端不发 / 前端不接 |
|---|---|---|---|---|
| `stream:open` | seq | routes_events.go | 忽略 | — |
| `chat:start` | started_at,run_id | chat_stream.go | streaming=true，清错，重置读数与秒表 | — |
| `chat:live` | phase,tool_name,tool_call_id,output_tokens | chat_stream.go | 活动条 / tokens/s | — |
| `chat:patch` | mode,index,kind,delta,block | chat_stream.go | `applyPatch` | — |
| `chat:turn` | user_message,turn_blocks | chat_stream.go | 覆盖块表 | — |
| `chat:turn_end` | turn_usage | chat_stream.go | `setUsage` | — |
| `chat:todos` | todos[] | chat_stream.go | 待办面板 | — |
| `chat:ask` | ask_id,ask_method,ask_title,ask_message,ask_placeholder,ask_prefill,ask_options | chat_service.go | 追加提问卡 | — |
| `chat:error` | message | chat_stream.go | 标错 + toast + 解流式态 | — |
| `chat:settled` | aborted | chat_stream.go | 只解流式态 | — |
| `chat:done` | aborted | chat_stream.go | finishTurn + 重拉 state/列表 + 工作区刷新 | — |
| `chat:compaction` | state(start/done),engine_kind | chat_actions.go | `compacting` 真假 + done 提示 | — |
| `chat:queue` | steering[],follow_up[] | chat_service.go | 队列镜像替换 | — |
| `chat:model` | provider_name,model_id | chat_session.go | 落 default 只 console.warn | 前端不接 |
| `chat:plan` | plan_enabled,session_id | plan_mode.go | 同上 | 前端不接 |
| `workspace:changed` | workspace | settings.go | 同上 | 前端不接 |
| `mcp:changed` | — | mcp.go | 同上 | 前端不接 |
| `models:changed` | — | models.go | 重拉模型下拉 | — |
| `session:info` | — | chat_service.go / chat_session.go / chat_state.go | 重拉会话列表 | — |
| `session:title` | title | chat_session.go | 重拉会话列表 | — |
| `skill:status` | skill_dir_name（必有）；另有 skill_enabled / skill_reset / skill_deleted / skill_imported 四个动作标志位之一 | skills.go ×6（安装/开关/重置/导入/新建/删除各一处） | `bumpSkills` | — |
| `engine:detected` | runtime | app.go | 重拉运行时 | — |
| `engine:ready` | — | routes_core.go | 重拉运行时 + 空闲重拉 state | — |
| `app:close_requested` | close_action | shell/window.go | 弹关闭选择框 | — |
| `engine:status` | — | — | 重拉运行时 | **后端不发** |
| `engine:exit` | — | — | 复位 streaming + 报错 toast | **后端不发** |
| `engine:commands` | commands | — | 覆盖命令清单 | **后端不发** |
| `app:progress` | progress_kind,progress_action,progress_phase,progress_line,progress_message | — | 安装日志面板 | **后端不发** |
| `chat:blocks` | run_id,turn_blocks | — | 覆盖块表 | **后端不发** |
| `chat:notify` | level,message | — | 过程提示条 | **后端不发** |
| `chat:status` | key,text | — | 状态文字 | **后端不发** |
| `chat:widget` | key,lines[] | — | 读数小组件 | **后端不发** |
| `chat:retry` | attempt,max,delay_ms | — | 重试进度 | **后端不发** |
| `chat:retry_end` | — | — | 清重试进度 | **后端不发** |
| `chat:editor_text` | text | — | 反向预填输入框 | **后端不发** |

幽灵事件的分支留在 `stores/events.ts` 里，命中即无反应；排障时不要据此判断后端故障。
`AppProgressVO` 类型仍在 `domain/core.go`，但没有任何发射点。

## 5. 错误码

### 5.1 按段位（`backend/pkg/codes.go`）

| 码 | 常量 | 含义 | 复用方 / 备注 |
|---|---|---|---|
| `0` | CodeOK | 成功 | 唯一不视为异常的值 |
| `1000` | CodeBadRequest | 入参不合法 | 全部模块的 body 解析失败、空值与不发散枚举 |
| `1001` | CodeFileNotFound | 文件或目录不存在 | files、system 相关动作 |
| `1002` | CodePathOutside | 路径越界 | files 的 `guard`、web 的内网地址拒（同码不同义，看 message） |
| `1003` | CodeFileIO | 文件读写失败 | files、templates、skills、searcher、system |
| `1100` | CodeGitMissing | git 不可用 | 仅 git |
| `1101` | CodeGitFailed | git 命令失败 | 仅 git，`detail` 是 stderr |
| `1102` | CodeGitAuth | 凭证被拒 / 无法输入 | 仅 git |
| `2002` | CodeStorage | 本地存储失败 | `repo` 的 JSON 台账与 SQLite |
| `3000` | CodeProviderBad | 服务商配置不合法 | models、chat 的缺密钥 |
| `3001` | CodeNoProvider | 没有可用服务商 / 模型 | chat 的 `resolveModel` |
| `5000` | CodeSessionBind | 未绑定会话或工作区 | **files / git / searcher 的「请先选择工作区」也用它** |
| `5001` | CodeSessionIO | 会话文件读写失败 | agent 的 `Session.append` 与 parser 两处都用 |
| `5002` | CodeEngineBusy | 有任务占用引擎 | chat 的切会话/新建/压缩/克隆/分叉、sessions 的改名 |
| `5003` | CodeNotFound | 目标不存在 | chat（分叉点、无运行）、models、web（PDF 无文字） |
| `7000` | CodeProvision | 外部进程 / 上游调用失败 | web 抓取、压缩总结、子进程启动 |
| `8000` | CodeSkillBad | 技能路径或内容不合法 | skills |
| `8001` | CodeSkillSave | 技能保存失败 | skills |
| `8002` | CodeSkillDelete | 技能删除失败 | skills |
| `9100` | CodeMcpRead | `mcp.json` 读取失败 | mcp |
| `9101` | CodeMcpWrite | `mcp.json` 写入失败 | mcp，也用于条目不存在 |
| `9102` | CodeMcpCall | MCP 工具调用失败 | mcp_client `tools/call` |
| `9103` | CodeMcpServer | MCP 服务进程不可达 | `pkg.StartPersistent` 写不进 stdin 时也回它；启动失败是 7000 |
| `9104` | CodeMcpTimeout | MCP 请求超预算 | mcp_client |
| `1404` | CodeUnknownRoute | 未知路由 | `NoRoute` |
| `9999` | CodeInternal | 未归类内部错误 | 兜底 |

### 5.2 按模块

| 模块 | 可能出现的码 |
|---|---|
| 引导 / 设置 | 1000, 1001, 1003, 2002 |
| 引擎 / 系统 | 1000, 1001, 1003, 1404, 9999 |
| 会话 | 1000, 5000, 5001, 5002 |
| 对话 | 1000, 3000, 3001, 5000, 5001, 5002, 5003, 7000 |
| 模型 | 2002, 3000, 3001, 5003 |
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
