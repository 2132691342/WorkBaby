# 模块手册

按功能域排障用。每章六段：①功能与页面入口 ②后端链路 ③前端链路 ④HTTP 接口 ⑤SSE 事件 ⑥错误码与排障。
接口完整 schema 见 `docs/API.md`，为什么这样搭见 `docs/ARCHITECTURE.md`。
约定：`/api/v1` 前缀，只用 GET/POST，信封恒 200（`{code,message,detail?,data}`），`code != "0"` 即失败；鉴权 `X-Desktop-Token`，SSE 走 `?token=`。

---

## 1. 对话（计划模式 · 附件 · 斜杠命令 · 队列 · 提问卡）

**最容易踩的坑**：`/chat/send` 有三种回执（`started`/`queued`/`handled`），只有 `started` 起运行；把 `queued` 当 `started` 会让界面卡在「正在回答」。

**① 入口**：`/chat`（`views/ChatView.vue`）——左栏会话列表、主区轮次流、底部输入区。一轮最多 24 步（`agent/loop.go maxSteps`）。

**② 后端链路**：`api/routes_chat.go` + `server.go mount`（/chat/*、/messages/*）→ `service/chat_service.go`（Send/Stop/Steer/FollowUp/ClearQueue/ask/resolveModel）→ `chat_actions.go`（`compose` 折附件、`Compact`/`compactNow`/`summarize`、`Answer`、`RemoveQueued`、`Export`）→ `chat_session.go`（克隆/分叉/改名/切模型/切思考档/Warm）→ `chat_state.go`（`State`、`usageSnapshot`、`contextWarnText`、`nativeCommands`、`HandleCommand`）→ `chat_stream.go`（引擎事件→SSE）→ `plan_mode.go` → `agent/loop.go`、`provider_openai.go`、`tool*.go`、`policy.go`、`tidy.go`、`session.go`。
发送判定顺序：空白消息 1000 → 无附件时先认斜杠命令（被消费回 `handled`）→ `Busy()` 则转 `Steer`（回 `queued`）→ `BindSession` → `resolveModel` → `Stream.Begin` → goroutine。goroutine 内先判自动压缩（开关非 `"0"`、`context_window>0`、`ContextUsage()/window>0.8` 三者齐备 → `compactNow`，失败只 Warn 不挡本轮），再 `runner.Run`，最后逐条派发 `FollowUpQueued()`，每条 `Stream.Begin` 后各起一轮。
引擎的轮次闭环在 `agent/loop.go`：调模型 → 模型要工具就顺序跑完 → 工具结果落进会话 → 回到顶部再调一次模型（`history()` 从会话文件把工具结果读回来喂给模型）→ 模型不再要工具才收尾。一轮最多 24 步（`maxSteps`），步数用满时明确报错而不是静默截断。
压缩是真压缩：上游同步总结（120s、不带工具、不发思考档）→ 落带摘要的 `compaction` 条目；`Runner.history()` 以最后一条带摘要的压缩条目为分界，之前的全部折成一条摘要引导消息。`Steer` 进 `queue`（本轮收尾后注入），`FollowUp` 进 `followUp`（本轮结束后另起一轮），两条队独立，`ClearQueued` 两条一起清。

**③ 前端链路**：`views/ChatView.vue` → `stores/chat.ts` + `stores/events.ts` → `useMessageFlow`（历史/增量/用量）、`useRunStatus`（活动条）、`useComposerDraft`/`Menu`/`Actions`、`useAttachments`、`useBuiltinCommands`、`useEngineModels`、`useTurnClock`、`useFilePeek` → `components/chat/*`：`HistoryList`/`MessageTurn`/`TurnBlocks`/`ToolCallCard`/`ThinkingBlock`/`AskCard`/`TodoPanel`/`ChatComposer`/`ComposerBar`/`ComposerStatus`/`ComposerMenu`/`MentionBrowser`/`ModelPicker`/`ContextMeter`/`ActivityBar`/`ChatNotices`/`FilePeekDrawer`；左栏复用 `components/sessions/SessionList.vue`。
贴底由 `schedulePin` 用 rAF 合帧（同帧只滚一次，距底 <2px 不推）；思考块展开态在 `ThinkingBlock.vue` 用 Map 当 LRU 记；发送失败留 `lastFailedSend`，横幅「重试发送」走 `retrySend`。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/chat/state?session=` | `session`（可空） | `session_file,work_dir,engine_running,streaming,model,thinking_level,thinking_levels,context_usage,context_warn,auto_compaction,queued{steering,follow_up},stats,history,todos,plan_mode,plan_state{plan_enabled,blocked_tools},commands[]{command_name,command_source,command_desc,command_body},caps,bind_err` |
| POST | `/chat/send` | `session_file,message,attachments[]{attachment_kind,file_path,file_name,image_base64,image_mime,attachment_body}` | `run_id,disposition` |
| POST | `/chat/steer`、`/chat/followup` | `message,attachments` | `{ok}` |
| POST | `/chat/stop`、`/chat/thinking`(`thinking_level`)、`/chat/autocompact`(`auto_compaction`)、`/chat/model`(`provider_name,model_id`)、`/messages/hide`、`/messages/unhide`(`session_file,turn_id`) | 见括号 | `{ok}` |
| POST | `/chat/clear-queue` / `/chat/remove-queued` | 后者：`queue_kind(steering\|followup),queue_index` | `{steering,follow_up}`（前者是被清掉的文本） |
| POST | `/chat/compact` / `/chat/answer` | 前者：`session_file?,instructions?`；后者：`session_file,ask_id,value?,confirmed?,cancelled?,answers[]{question_id,selected_options,custom_input}` | `{ok}` |
| POST | `/chat/new`、`/chat/clone` / `/chat/fork` | 后者：`session_file,turn_id` | `session_file`（fork 另带 `fork_text?`） |
| POST | `/chat/export-html` / `/chat/plan` | 前者：`session_file`；后者：`plan_enabled` | `file_name,file_content` / `plan_enabled,blocked_tools,plan_message` |

附件三类：`file` 只在正文追加 `[附件] <绝对路径>`（让模型自己读）；`image` 走内容块（唯一二进制通道）；`session` 把 `attachment_body` 整段折进正文（会话文件在工作区外，读工具读不到）。带附件不认斜杠命令。思考档：后端接受 `off/minimal/low/medium/high`，`ChatStateRESP.thinking_levels` 只给 `off/low/medium/high`（`minimal` 界面不列）；`low/medium/high` 由 `provider_openai.go` 填进 `reasoning_effort`，`off`/`minimal` 不发该字段，值存 `model.thinking_level`。
斜杠命令双轨：后端 `nativeCommands` 列 `/new` `/compact` `/export` `/plan`（`/export` 后端不处理，由前端本地命令执行）；前端 `useBuiltinCommands.ts` 另有 `/model` `/thinking` `/copy` `/exporthtml` `/reload` `/clone`，按能力位过滤；模板命令以 `command_source="prompt"` 并进同一菜单。

**⑤ SSE**：`chat:start`、`chat:live`、`chat:patch`、`chat:turn`、`chat:turn_end`、`chat:todos`、`chat:ask`、`chat:error`、`chat:settled`+`chat:done`、`chat:compaction`、`chat:queue`；`chat:model`/`chat:plan` 前端不接。载荷见 §2。

**⑥ 错误码与排障**

| 码 | 现象 → 先查哪 |
|---|---|
| 1000 | 消息/插话/追加为空：只发附件时 `compose` 后正文为空即拒 |
| 3000 | 服务商缺密钥：`/providers` 的 `has_key`，日志 `服务商缺密钥 code=3000` |
| 3001 | 没选模型或模型已删：错误同时 `DropActiveIf` 清指针，日志 `解析活跃模型失败 code=3001` |
| 5000 | 未选工作区：`BindSession` 第一行即判 |
| 5002 | 有运行在跑：切会话/新建/压缩/克隆/分叉全拒，先 `/chat/stop` |
| 5003 | 分叉点不在活动分支，或没有运行可插话/移队 |

失败都有 `pkg.Warn` 日志（发布版无控制台，这是唯一入口）：`聊天发送绑定会话失败`、`聊天发送取模型失败`、`聊天发送排队失败`、`聊天运行失败`、`压缩总结失败`。如实记的边界：`Chat.AskPending()` 维护着在等的提问清单，但 `/chat/state` 不带这个字段，界面刷新后提问卡不回来；`context_warn` 后端会填（`>0.8` 且自动压缩开），前端不消费。

---

## 2. SSE 事件协议（`GET /events`）

**最容易踩的坑**：前端 11 个分支后端从不发，后端 4 个事件前端没有分支；「界面没反应」先确认这帧有没有人发、有没有人接。

**① 机制**：界面实时状态的唯一通道。`sse/hub.go` 发号与补帧，`api/routes_events.go` 长连接。

**② 后端链路**：`service/*` 调 `Hub.Broadcast`；chat 帧经 `sse.EngineTag` 补 `engine_kind:"native"`，`chat_stream.go emit` 再补 `run_id`。帧 `{seq,event,data,sent_at}`；`seq` 只由 hub 发；保留环 1024 帧；每订阅者一条 64 缓冲队列 + 发送泵（补帧先入队、实时帧后入队，结构性排除乱序），写满即断开该连接；心跳 25s 一行注释（`: ping`）不占 seq。

**③ 前端链路**：`stores/app.ts connect()` 建 `EventSource(streamURL(lastSeq))` → `utils/sse.ts readFrame` → `stores/events.ts routeFrame`。seq 跳号即 `reconcile()` 用 `chat/state` 对账；断线指数退避 1.2s→15s；`stream:open` 不占新号。

**④ HTTP**：`GET /events?token=&since=`，`text/event-stream`；鉴权失败回裸 401（无信封）。事件名在帧体的 `event` 字段。

**⑤ 事件表**

| 事件 | 载荷 | 发射点 | 前端反应 |
|---|---|---|---|
| `stream:open` | seq | routes_events.go | 忽略 |
| `chat:start` | started_at,run_id | `Stream.Begin` | streaming=true，清错、重置读数、起秒表 |
| `chat:live` | phase(thinking/text/tool),tool_name,tool_call_id,output_tokens | TextDelta/ThinkingDelta/ToolStart | 活动条与 tokens/s，按 run_id 过滤别轮 |
| `chat:patch` | mode(delta/block),index,kind,delta?,block? | `emitPatch` | `applyPatch` 改块表 |
| `chat:turn` / `chat:turn_end` | 前者：user_message,turn_blocks；后者：turn_usage{input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,cost_usd} | `AssistantMessage` | 快照覆盖块表（已收尾的 run 丢弃）/ `setUsage(run_id,…)` |
| `chat:todos` | todos[]{todo_id,todo_subject,todo_status,todo_active_form} | `emitTodos`（仅 todo 工具） | 待办面板实时更新 |
| `chat:ask` | ask_id,ask_method,ask_title,ask_message,ask_placeholder,ask_prefill,ask_options | `Chat.ask` | 追加提问卡（按 id 去重） |
| `chat:error` / `chat:settled` / `chat:done` | message（ANSI 已清）/ aborted / aborted | `Fail`、`Finish`（settled+done 连发） | 标错+toast 并解流式态 / 只解流式态 / finishTurn + 重拉 state 与列表 + bumpWorkspace |
| `chat:compaction` | state(start/done),engine_kind | `compactNow`（start 1 + done 3） | `compacting = state==='start'`，done 补提示 |
| `chat:queue` | steering[],follow_up[] | `ClearQueue`/`refreshQueue` | 队列镜像整体替换 |
| `chat:model` / `chat:plan` | provider_name,model_id / plan_enabled,session_id | `SetModel` / `SetPlanMode` | **前端不接**（落 default 只 console.warn） |
| `models:changed` / `mcp:changed` | — | `models.changed` / `mcp.notifyChanged` | 重拉模型下拉并纠正被删模型 / **前端不接** |
| `session:info` / `session:title` | 后者带 title | 新建/克隆/分叉/改名 | 重拉会话列表 |
| `skill:status` | skill_dir_name + enabled/reset/deleted/imported 之一 | skills.go ×6 | `bumpSkills` → 技能页重拉 |
| `workspace:changed` | workspace | `SetWorkspace` | **前端不接**（发起方自己更新） |
| `engine:detected` / `engine:ready` | runtime / — | `App.Redetect` / `engines/restart` | 重拉运行时；ready 另在空闲时重拉 state |
| `app:close_requested` | close_action | shell/window.go | 弹「收进托盘 / 退出」 |

**后端从不发、前端有分支**（幽灵，命中即无反应）：`engine:status`、`engine:exit`、`engine:commands`、`app:progress`、`chat:blocks`、`chat:notify`、`chat:status`、`chat:widget`、`chat:retry`、`chat:retry_end`、`chat:editor_text`。后果：安装日志面板（`app:progress`）永远空转；`useRunStatus` 的提示/状态/小组件只能由前端本地 `pushNotice` 等填。

**⑥ 排障**：「卡在运行中」→ 查是否收到 `chat:settled`/`chat:done`；「多窗口不同步」→ 查上表「前端不接」三行；「切回来少一段」→ 查 seq 跳号与 `reconcile()`。SSE 不走信封，没有业务码。

---

## 3. 会话历史与会话页

**最容易踩的坑**：页面顶部合计恒为「当前目录」口径，不随列表范围变化；`/sessions/delete` 入参是 `session_path`，不是 `path`。

**① 入口**：`/sessions`（`views/SessionsView.vue`）——范围「当前目录/全部目录」+ 汇总 + 列表；列表复用对话页左栏。

**② 后端链路**：`api/routes_data.go`（/sessions*）+ `routes_chat.go`（/messages/*）→ `service/sessions.go`（List/Detail/Delete/Rename/History/Todos/PlanMode/Hide/Unhide/Export/Count）→ `service/parser/`（`history.go` 活动分支回放、`summary.go` 单行摘要、`entries.go`、`plan.go`）→ `agent/session.go`（`Store.Open`/`Append*` 只追加不改行）。`List` 按工作区编码目录扫 `.jsonl`（无 workspace 扫全部子目录），`updated_at` 降序；`q` 先比标题，未命中才读全文；坏文件跳过。`Rename` 追加 `session_info` 条目，被引擎打开的那份直接拒。

**③ 前端链路**：`views/SessionsView.vue`（`refreshSeq` 竞态守卫）→ `components/sessions/SessionList.vue`（首屏 120 行 + 「显示更多」逐页放开；搜索 400ms 防抖走后端；`loadSeq` 序号守卫）→ `stores/app.ts refreshSessions`。打开会话 = 路由 `/chat?session=<path>`。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/sessions?workspace=&q=` | `workspace`（空=全部目录）、`q` | `sessions[]{session_id,session_path,session_title,session_cwd,session_model,updated_at,size_bytes,message_count,total_tokens,cost_usd}` |
| GET | `/sessions/detail?path=` | `path` | `session{…},items[]{item_kind,item_role,item_text,item_model,tool_name,tool_call_id,tool_is_error,created_at,item_tokens,cost_usd}` |
| POST | `/sessions/delete` / `/sessions/rename` | `session_path` / `session_path,session_title` | `{ok}` |
| POST | `/messages/hide`、`/messages/unhide` | `session_file,turn_id` | `{ok}` |

**⑤ SSE**：`session:info`/`session:title` 触发列表重拉；隐藏/取消隐藏不发事件，前端本地摘除或重拉 state。

**⑥ 错误码**：5000 未选工作区；5001 会话读删失败或路径越界（`parser.GuardSession` 要求落在 `data\sessions` 下）；5002 改名被拒（会话正被引擎打开，先切走）；1000 标题为空。真相全在文件：标题取改名条目优先、否则首条用户消息；`History` 剔隐藏轮；`Todos` 取活动分支最近一次 todo 调用；`PlanMode` 取最后一条 `custom`（`plan-mode`）。删会话会顺带清 `chat.hide:<会话文件>` 键（失败只 Warn）。

---

## 4. 模型服务商

**最容易踩的坑**：`api_protocol` 的 `openai/anthropic/google` 只影响**探测列模型**；聊天流式只有 OpenAI 兼容适配器，配成 anthropic/google 能「测试连接通过」而一聊就失败。

**① 入口**：`/models`（`views/ModelsView.vue`）——服务商列表、弹窗编辑、模型表、默认模型卡；对话页下拉吃同一份清单。

**② 后端链路**：`api/routes_data.go`（/models、/providers*）→ `service/models.go`（Upsert/Delete/SaveModel/DeleteModel/Providers/Options/Lookup/DropActiveIf/SaveDefaults/maskKey）→ `model_probe.go`（按协议拼列模型端点与鉴权头，20s）→ `repo/provider_repo.go`（SQLite `provider`/`provider_model`）。Upsert 对账：`req.Models != nil` 时整表替换——先 upsert 请求行，再删库里多出的行，每删一行 `DropActiveIf(provider,modelId)` 清活跃/默认指针；`req.Models == nil` 时保留原清单（改地址不清模型）。`api_key` 空串 = 保留原值；`maskKey`：空→空，≤8 字符→`••••`，否则前 4 + `••••` + 后 4。

**③ 前端链路**：`views/ModelsView.vue`（AbortController + `loadSeq`、表单就地校验、删除 `ElMessageBox` 二次确认带后果文案）→ `components/models/{ProviderList,ProviderDialog,ModelTable,DefaultModelCard}.vue` → `composables/useProviderForm.ts`；对话页下拉走 `useEngineModels.ts`，靠 `models:changed` 重拉。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/models` | — | `models[]{provider_name,model_id,model_name,api_protocol,context_window,max_tokens,supports_reasoning,input_kinds}` |
| GET | `/providers` | — | `providers[]{provider_name,base_url,api_protocol,auth_kind,api_key_masked,has_key,models[],model_count}`,config_path,read_error? |
| POST | `/providers/upsert` | `provider_name,base_url,api_protocol,auth_kind,api_key,models[]{model_id,model_name,api_protocol,context_window,max_tokens,supports_reasoning,input_kinds}` | `ProvidersRESP` |
| POST | `/providers/delete`、`/providers/model/save`(`provider_name,model{…}`)、`/providers/model/delete`(`provider_name,model_id`) | 见括号 | `ProvidersRESP` |
| POST | `/providers/models`、`/providers/test` | `provider_name,base_url,api_protocol,api_key`（空项取已保存值） | `probe_ok,probe_message,model_count,models[]` |
| GET/POST | `/models/defaults` | POST：`default_provider,default_model,default_thinking_level` | `default_provider,default_model,default_thinking_level,settings_file` |

**⑤ SSE**：`models:changed`（增删改后广播，前端重拉下拉）。

**⑥ 错误码**：3000 配置不合法（名/地址为空、默认项只填一半、未知思考档）；3001 没有可用的服务商或模型（没选模型，或选的那个已被删掉——报错时顺手清掉坏指针）；5003 `Lookup` 未命中。协议字段不校验：非空即原样存下，探测时只有 `anthropic`/`google` 走各自的分支，其余一律按 OpenAI 兼容处理。顺序：先看 `/providers` 的 `has_key` 与 `models[]`，再 `POST /providers/models` 验连通，最后看日志 `解析活跃模型失败 code=3001` / `服务商缺密钥 code=3000`。默认三键（`model.default_*`）只被本页读写，发送链路不消费。

---

## 5. 技能

**最容易踩的坑**：「开关」是搬目录不是改配置；传目录路径一律拒，动作对象是 `SKILL.md` 所在的**目录**。

**① 入口**：`/skills`（`views/SkillsView.vue`）——启用/停用/项目三区 + 推荐清单（10 条，未装可一键装）+ 抽屉编辑。

**② 后端链路**：`api/routes_data.go`（/skills*）→ `service/skills.go`（Install/Toggle/Reset/Import/Create/Delete/guardPath/sanitizeSkillName）→ `skills_scan.go`（三区扫描、按目录名去重、按 `skill_group` 字母序）、`skills_edit.go`、`skill_catalog.go`、`skill_fs.go`（`copyTree`/`copyTreeAtomic`）→ `pkg/frontmatter.go`。目录：启用 `data\skills`、停用 `data\skills_disabled`、项目 `<工作区>\.pi\skills`（只读，开关与删除都拒）、随包原件 `<assets>\skills`。`Toggle` 搬完写 `skill_toggles` 台账，**写失败只 Warn 不回滚**；`Reset` 先复制到 `<dir>.resetting` 再换名并保住原分区；`Install` 只认推荐 `skill_dir_name`，已存在拒。

**③ 前端链路**：`views/SkillsView.vue`（AbortController + `loadSeq`、重置/删除二次确认）→ `components/skills/{SkillRow,SkillDrawer,SkillCreateDialog}.vue` → `stores/refresh.ts bumpSkills`。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/skills` | — | `skills[]{skill_name,skill_dir_name,skill_desc,skill_path,skill_dir,skill_source,skill_group,skill_requires,skill_enabled,skill_editable,skill_modified}`,recommended_total,recommended_installed,available[]{skill_dir_name,skill_group,skill_desc,skill_requires} |
| POST | `/skills/install` | `skill_dir_name` | `{ok}` |
| GET | `/skills/content?path=` | `path`（SKILL.md） | `file_text` |
| POST | `/skills/save` / `/skills/toggle` | `skill_path,file_text` / `skill_path,skill_enabled` | `{ok}` |
| POST | `/skills/create` | `skill_name,skill_desc,skill_body` | `skill_path` |
| POST | `/skills/import` | `skill_from_path` | `skill_dir_name,skill_path` |
| POST | `/skills/delete`、`/skills/reset` | `skill_path` | `{ok}` |

**⑤ SSE**：`skill:status`，载荷 `skill_dir_name` + `skill_enabled`/`skill_reset`/`skill_deleted`/`skill_imported` 之一；前端 `bumpSkills` 让技能页重拉。

**⑥ 错误码**：8000 路径越界（必须在三个托管根内）、名字清洗后为空、已存在、不认识的推荐技能、所选目录无 SKILL.md、非内置不能重置；8001 目录不可写或搬移/复制/保存失败；8002 删除失败（含传目录路径、删项目技能）；1000 `skill_path` 不指向 SKILL.md。现象「技能页有、模型不认」→ 可能只是扫到了禁用区同名目录，比对 `data\skills\<dir>\SKILL.md` 与 `skill_toggles`；`skill_modified` 为真表示内容与随包原件不同。

---

## 6. MCP

**最容易踩的坑**：引擎侧装载只认 `command`（stdio）；只配 `url` 的条目能保存、能探测，但不会变成模型可调的工具。

**① 入口**：`/mcp`（`views/McpView.vue`）——合并清单（用户级 + 项目级）、弹窗编辑、开关、测试连接、删除。

**② 后端链路**：`api/routes_data.go`（/mcp*）→ `service/mcp.go`（List/Save/Toggle/Remove/notifyChanged/findEntry）→ `mcp_probe.go`（stdio 拉真进程 initialize+tools/list；http 发真请求）→ `mcp_runtime.go`（`Rebuild` 按 enabled 的 stdio 条目整段重建注册表）→ `agent/mcp_client.go`（stdio JSON-RPC 行协议：initialize → notifications/initialized → tools/list → tools/call；单请求 30s、握手 60s，条目 `timeout`（秒）覆盖请求预算）+ `agent/tool_mcp.go`（工具名 `mcp_{server}_{tool}`，schema 透传）。真相是两份 JSON：`data\mcp.json` 与 `<工作区>\.pi\mcp.json`，同名时项目级覆盖用户级，`enabled` 缺省视为启用。`Save` 先整份继承旧条目再覆盖表单字段（手配 headers 不丢）；服务名只允许字母数字 `_` `-`。`notifyChanged` 广播 `mcp:changed` 并 `go Runtime.Rebuild()`；单服务装载失败只 Warn 跳过；`rebuildMu`（容量 1 channel）串行化重建。

**③ 前端链路**：`views/McpView.vue`（`loadSeq` + AbortController、删除二次确认）→ `components/mcp/{McpTable,McpDialog}.vue`。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/mcp` | — | `servers[]{server_name,transport_kind,command_line,server_url,mcp_desc,mcp_work_dir,mcp_env_text,mcp_timeout,mcp_enabled,scope_kind,config_path}`,user_path,project_path?,read_errors? |
| POST | `/mcp/save` | `server_name,command_text,server_url,transport_kind,mcp_desc,work_dir,env_list[],mcp_timeout,mcp_scope(user\|project)` | `McpListRESP` |
| POST | `/mcp/toggle` / `/mcp/probe` | `server_name,mcp_enabled` / `server_name` | `McpListRESP` / `probe_ok,probe_message,probe_tools` |
| POST | `/mcp/remove` | `server_name` | `McpListRESP` |

**⑤ SSE**：`mcp:changed`（前端无分支，仅提示后端已重建工具段）。

**⑥ 错误码**：9100 `mcp.json` 读取/解析失败（原因进 `read_errors`，坏文件不影响另一侧）；9101 写入失败、服务名非法、project 作用域未选工作区、条目不存在；9102 `tools/call` 被拒或 `isError`；9103 服务进程没起来/已退出（stderr 进 Detail）；9104 预算内无应答（看 `timeout` 与 npx 冷启动）。现象「测试通过但模型不会用」→ 该条目是 `url` 型或 `enabled=false`；日志看 `MCP 服务 %s 已装载 %d 个工具` / `装载失败`。计划模式下 MCP 工具按写工具拦。未做：OAuth、SSE/HTTP 传输接入引擎、resources/prompts。

---

## 7. 工作区 · 文件

**最容易踩的坑**：路径先过 `Files.guard` 归一，相对路径按**工作区根**拼（不是进程 CWD）；越界是 1002 不是 1001。

**① 入口**：`/workspace`（`views/WorkspaceView.vue`）左/中栏——文件树、面包屑、文本编辑、二进制预览；对话内预览走 `FilePeekDrawer`。

**② 后端链路**：`api/routes_files.go` → `service/files.go`（guard/List/Search/Read/Save/Mkdir/Delete/Rename）。上限：列目录 500、搜索 200、读 1MB（超限 `truncated`）、写 5MB、可预览二进制 32MB。`Search` 只按**文件名**包含匹配，跳过 `node_modules`/`.git`/`dist`/`build`/`.next`/`target`/`__pycache__`/`.venv`；`Read` 用 NUL 字节或非 UTF-8 判二进制，`.pdf`/`.xlsx|.xls|.xlsm`/`.docx`/图片回 `file_base64`。

**③ 前端链路**：`views/WorkspaceView.vue`（`WbSkeleton`）→ `components/workspace/{FilePanel,FilePreview,DiffView}.vue`、`components/common/DiffRows.vue`、`components/chat/FilePeekDrawer.vue`（`useFilePeek`）。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/files?path=` | `path`（空=根） | `dir_path,entries[]{entry_name,entry_is_dir,size_bytes,modified_at},truncated` |
| GET | `/files/search?query=&limit=` | `query,limit`（默认 50，上限 200） | `search_query,hits[]{file_path,entry_name},truncated` |
| GET | `/files/read?path=` | `path` | `file_path,file_text,size_bytes,is_binary,file_kind,file_base64?,truncated` |
| POST | `/files/save` | `file_path,file_text` | `{ok}` |
| POST | `/files/mkdir`、`/files/delete` | `file_path`（delete 递归，根拒删） | `{ok}` |
| POST | `/files/rename` | `file_path,new_file_path` | `{ok}` |

**⑤ SSE**：无专属事件；`chat:done` 后 `bumpWorkspace` 让工作区页重看一眼。

**⑥ 错误码**：5000「请先选择工作区」（复用会话段码，不是会话问题）；1001 不存在（含「这是一个目录」）；1002 路径不在工作区内；1003 读失败、父目录创建失败、写失败、超 5MB、mkdir/rename 目标已存在。现象「预览空白」→ 看 `is_binary`/`file_kind`/`file_base64` 是否为空、文件是否 >32MB；文本没渲染先看 `truncated`。

---

## 8. Git

**最容易踩的坑**：写命令走 `runWriteOrFail` 独占 `writeMu`；并发直连 `git` 只会得到「抢不到 index.lock」这类看不出原因的失败。凭证走 GCM，`GIT_TERMINAL_PROMPT=0` 让问询立即失败而不是卡 120s。

**① 入口**：`/workspace` 右栏——`GitBar`（分支/推拉/远端）、`GitChangesPane`、`GitCommitPanel`、`HistoryPanel`、冲突解决与 stash。

**② 后端链路**：`api/routes_git.go`（`mountGit`）→ `service/git.go`（Status/Diff/ProbeOf/runIn/runOrFail/runWriteOrFail/gitFail/gitAuthHint）→ `git_write.go`（Stage/Unstage/Discard/Resolve/ResolveConflict/Commit/CreateBranch/SwitchBranch/DeleteBranch/Merge/State/Init/AddRemote）→ `git_history.go`（History/CommitDetail/FileHistory/FileDiff）→ `git_sync.go`（Push/Pull/Fetch）→ `git_stash.go`。`runIn` 默认目录取当前工作区、超时 120s；`Status` 用 `porcelain -z` 防乱码；diff 上限 4000 行（`dropped` 记被截条数）；失败时 stderr 进 `detail`，凭证类翻成 1102 + 指引。

**③ 前端链路**：`views/WorkspaceView.vue` → `components/workspace/{GitBar,GitChangesPane,GitCommitPanel,HistoryPanel,GitFileRows}.vue` → `composables/useGitActions.ts`（危险操作 `ElMessageBox` 二次确认，文案写明后果）。

**④ HTTP 接口**（写接口一律回最新 `GitStatusRESP`）

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/git/status` | — | `git_available,unavailable_reason?,is_repo,workspace,repo_root,branch_name,upstream_ref,ahead,behind,repo_state,repo_state_detail?,remotes[],changed_files[]{file_path,change_code,is_staged,has_conflict,insertions,deletions},has_conflict,git_version` |
| GET | `/git/diff?path=`、`/git/file-diff?hash=&path=` | 见路径 | `file_path,truncated,dropped,diff_lines[]{line_kind,old_no,new_no,line_text}` |
| GET | `/git/history?limit=`、`/git/file-history?path=&limit=` | `limit`（默认 50/40） | `commits[]{commit_hash,short_hash,commit_subject,author_name,commit_time,file_count,insertions,deletions}` |
| GET | `/git/commit?hash=` | `hash` | `commit{…},commit_body,parent_hashes[],changed_files[],diff{…}` |
| GET | `/git/branches`、`/git/stash/list` | — | `branches[]{branch_name,is_current,remote_name?,ahead,behind}` / `stashes[]{stash_ref,stash_subject}` |
| POST | `/git/stage`、`/git/unstage`、`/git/discard`、`/git/resolve` | `file_paths[]` | `GitStatusRESP` |
| POST | `/git/conflict/resolve` | `file_path,resolve_side(ours\|theirs\|"")` | `GitStatusRESP` |
| POST | `/git/commit` | `commit_message,is_amend` | `status,commit_hash` |
| POST | `/git/branch`、`/git/branch/delete`、`/git/switch`、`/git/merge`(`branch_name,is_rebase`)、`/git/state`(`state_action`)、`/git/init`(`branch_name`)、`/git/remote`(`remote_name,remote_url`) | 分支名，见括号 | `GitStatusRESP` |
| POST | `/git/push` | `remote_name,branch_name,force_mode(""\|none\|with-lease),set_upstream,confirm` | `GitStatusRESP` |
| POST | `/git/pull`(`remote_name,branch_name,is_rebase`)、`/git/fetch`(`remote_name`) | 见括号 | `GitStatusRESP` |
| POST | `/git/stash/push`、`/git/stash/pop`、`/git/stash/drop` | `stash_message` / `stash_ref` / `stash_ref,confirm`（须等于 `stash_ref`） | `GitStatusRESP` |

**⑤ SSE**：无 git 事件；`chat:done` 后 `bumpWorkspace` 提示重拉。

**⑥ 错误码**：1100 找不到 `git`（`pkg.LookPath` 空）；1101 命令失败（`detail` 是 stderr，`repo_state` 看是否在 merge/rebase）；1102 凭证被拒或无法输入（GCM 登录一次或换带 token 的远端）；1000 提交信息为空、分支名空/非法、强制推送未确认分支名、stash 丢弃未确认引用名；5000 未选工作区。

---

## 9. 工作区预索引（索引页 / 代码检索）

**最容易踩的坑**：索引是**进程内内存态**，`search_doc` 表只写不读；重启后第一次搜索惰性重建，frecency 同时归零。

**① 入口**：`/index`（`views/IndexView.vue`）——状态卡、检索框、重建；同一套检索也给模型（`index_search` 工具）。

**② 后端链路**：`api/routes_index.go` → `service/searcher.go`（Status/Rebuild/EnsureIndexed/Search/Kit）→ `index/index.go`（`Build` 单遍历收文件与符号；跳过依赖目录与二进制；单文件 ≤512KB、每文件正文 ≤8000 字符、单次 ≤20000 文档）→ `index/search.go`（`matchScore` 分档 + `Frecency` 频次对数 × 7 天半衰期）→ `repo/provider_repo.go SearchRepo`（`ReplaceWorkspace`/`BumpHit`/`Count`）。`Search` 先 `EnsureIndexed`（当前工作区无索引就现建），全程读内存；`Rebuild` 用 `building` 标志去重。

**③ 前端链路**：`views/IndexView.vue`（`searchSeq` 竞态守卫；`index_built` 为假时提示先建索引）→ `api.indexStatus/indexRebuild/indexSearch(term,30)`。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/index/status` | — | `index_workspace,index_doc_count,index_built,index_building,index_built_at` |
| POST | `/index/search` | `search_query,search_limit`（0 → 20） | `search_hits[]{doc_path,doc_kind,doc_symbol,doc_symbol_kind,doc_line,hit_score,hit_reason},index_status` |
| POST | `/index/rebuild` | — | `IndexStatusVO` |

**⑤ SSE**：无；重建同步返回。

**⑥ 错误码**：5000 未选工作区；1003 `index.Build` 失败（成功有日志 `索引完成：工作区=… 文档=…`）。现象「刚建完还是搜不到」→ 命中口径是文件名/符号名/正文包含；「重启后第一次搜索慢」→ 惰性重建属正常；「排序每次不同」→ 同分按路径排，frecency 归零后必然变化。

---

## 10. 联网

**最容易踩的坑**：默认搜索服务是免密钥的 `duckduckgo`；`Kit()` 每次调用按设置重建适配器，改服务即时生效；抓取对环回/私网/链路本地/CGNAT 一律拒（1002 不是 1000）。

**① 入口**：`/web`（`views/WebView.vue`）——搜索服务配置、搜索、抓取；模型侧对应 `web_search`/`web_fetch`。

**② 后端链路**：`api/routes_web.go` → `service/web.go`（Status/Search/Fetch/SaveSettings/SettingsRESP/Kit/config）→ `web/search.go`（duckduckgo/brave/exa/tavily → `SearchResult`）、`search_parse.go`（DDG 解析与跳转还原）、`client.go`（超时、`ValidateURL` 字面校验 + `safeDial` 连接前复核 IP、重定向逐跳校验）、`extract.go`、`pdf.go`、`inflate.go`。`Fetch` 按 Content-Type 分流：`pdf`→抽文字（空则 5003）、`json`→原样、`html`/`text`→可读正文（抽不出给原文）。装配时无条件 `AddWebTools`，可用性取决于上游配置而非装配条件。

**③ 前端链路**：`views/WebView.vue`（`searchSeq`/`fetchSeq` 守卫；`needsKey` 时才显示密钥框；留空=保留原值）。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/web/status` | — | `web_kind,web_has_key,web_message,web_kinds[]` |
| POST | `/web/settings` | `web_kind,web_api_key`（空串=保留原值） | 同上 |
| POST | `/web/search` | `search_query,search_limit` | `search_results[]{result_title,result_url,result_snippet,result_source},search_provider` |
| POST | `/web/fetch` | `fetch_url` | `fetch_url,fetch_text,fetch_char_count` |

**⑤ SSE**：无；保存设置不广播，下次调用时重建适配器。

**⑥ 错误码**：1000 不认识的搜索服务、地址为空/非法/非 http(s)/无主机名；1002 内网或重定向跳到内网；5003 PDF 无文字；7000 抓取失败、上游非 200、重定向过多、读响应失败。现象「搜索零结果」→ 换 `brave`/`tavily` 并确认 `web_has_key`；DDG 结果页结构变了会让解析为空。

---

## 11. 用量统计

**最容易踩的坑**：用量没有独立账本，全部来自会话 JSONL 扫描聚合；SQLite 只有 `provider`/`provider_model`/`search_doc` 三张表。

**① 入口**：`/usage`（`views/UsageView.vue`）——时间窗合计、按天趋势、按模型占比。概览页另有一份图表组件，两页的数据范围不同：用量页读全部目录，概览页读当前工作区。

**② 后端链路**：`service/parser/summary.go Summarize` 逐条扫会话，累加 `MessageUsage`/`EntryUsage`（`total_tokens`、`cost_usd`、`message_count`、`session_model`），`updated_at` 取最大条目时间；`sessions.go List` 装进 `SessionRowVO` 由 `/sessions` 返回，**没有独立聚合接口**。`message_count` 数的是消息条数（用户 + 助手 + 工具结果各一条），不是对话轮数。`session_model` 取值：最后一条 `model_change` → 最后一条助手消息的 `message_model`。

**③ 前端链路**：`views/UsageView.vue`（`scoped` 按 `updated_at` 落窗，`byDay`/`byModel` 内存分桶，自己 `echarts.init()` 建两个图实例、主题变化重绘）；数据源 `api.sessions()`（不带参数，全部目录）。概览页走另一条路：`views/OverviewView.vue` → `components/overview/UsageCharts.vue`（同样内嵌 echarts），数据源是 `api.sessions(app.workspace)`（当前工作区）。

**④ HTTP**：无专属接口，读 `GET /sessions?workspace=`（见 §3），字段 `total_tokens`/`cost_usd`/`message_count`/`session_model`/`updated_at`。

**⑤ SSE**：无；`chat:done` → `refreshSessions()` 间接更新。

**⑥ 排障**：无专属码，聚合失败回空数组。「数字为 0」→ 会话里没有 usage（上游未回用量，如部分自建网关），或会话不在当前工作区目录；「合计与列表不符」→ 合计只算当前时间窗与当前工作区快照。

---

## 12. 设置 · 偏好 · 工作区切换

**最容易踩的坑**：`/settings/save` 对 `ui.mention_sessions` 做入口收敛（1~20），非法值被静默改成边界值而不报错。

**① 入口**：`/settings`（`views/SettingsView.vue`）——外观、行为、引擎、工作区、配置原文、模板、诊断、关于。工作区切换三处入口：设置页、对话页输入框底栏的 `WorkspaceChip`、托盘菜单。

**② 后端链路**：`api/routes_core.go`（/bootstrap /settings* /workspaces /prefs* /logs）→ `service/settings.go`（键常量、EnsureDefaults、Save/normalize、MentionSessions、SetWorkspace/ForgetWorkspace/Workspaces/WorkspaceList、Prefs/SetRestoreSession/SetCloseAction/CloseAction/TrayHint*/RememberSession）→ `repo/store.go`（`workbaby.json` 原子整写 + 互斥）→ `service/autostart.go` + `service/platform/autostart_windows.go`（HKCU Run 项）。键：`workspace.current`、`workspace.history`（JSON 数组 ≤24）、`prefs.last_session_file`、`prefs.restore_session`、`prefs.close_action(tray|quit)`、`prefs.tray_hint_shown`、`chat.hide:<会话文件>`、`model.active_provider`、`model.active_model`、`model.thinking_level`、`model.auto_compaction`、`model.default_*`、`web.search_kind`、`web.search_key`、`ui.mention_sessions`、`ui.font_scale`、`ui.reduce_motion`、`ui.locale`、`ui.theme`。`SetWorkspace`：目录必须存在 → 写当前值 + 历史（同路径去重、超 24 截断）→ 广播 `workspace:changed`；`App.SetWorkspace` 随后 `Chat.OnWorkspaceChanged`（有运行先 Stop，再丢绑定）。

**③ 前端链路**：`views/SettingsView.vue` → `components/settings/{Appearance,Behavior,Engine,Workspace,Config,Templates,Diag,About}Panel.vue` → `stores/app.ts`（`saveSettings` 即时应用主题/字号/语言/动效；`setWorkspace` 后 `chat.reset()` 并重开会话）。「启动恢复上次会话」由前端 `restoreLastSession()` 执行（读 prefs → `chat.refresh` → 路由跳转），不是壳行为。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/bootstrap` | — | `app_version,app_contract(3),engines[],active_engine,workspace,settings{},layouts[],workspaces[],runtime,prefs` |
| GET/POST | `/settings`、`/settings/save` | POST：键值 map | 全量键值 map |
| POST | `/settings/workspace` | `workspace` | `workspace` |
| GET | `/workspaces` | — | `workspace,workspace_rows[]{workspace_path,workspace_exists,workspace_is_repo,workspace_branch,workspace_dirty,session_count,updated_at}` |
| POST | `/settings/workspace/forget` | `workspace_path` | `workspace,workspace_rows[],active_engine` |
| GET | `/prefs` | — | `pref_values,auto_start,restore_session,last_session_file,close_action,data_dir,log_dir` |
| POST | `/prefs/autostart`、`/prefs/restore` | `pref_enabled` | `AppPrefsVO` |
| POST | `/prefs/close-action` / `/prefs/data-dir` | `close_action` / — | `AppPrefsVO` / `{ok}` |
| GET | `/logs?lines=` | `lines`（默认 500） | `lines[]` |

**⑤ SSE**：`workspace:changed`（前端无分支，发起方自己更新）；`models:changed`（保存默认模型后）。

**⑥ 错误码**：1000 工作区路径为空；1001 工作区不存在或不是目录；2002 台账/SQLite 读写失败；1003 数据目录不可写。现象「切了工作区会话还在旧目录」→ 看 `Chat.OnWorkspaceChanged` 与前端 `chat.refresh('')`；「关窗直接退出」→ `prefs.close_action=quit`；「关窗没提示」→ `prefs.tray_hint_shown` 已置 1（只问一次）。

---

## 13. 提示词模板

**最容易踩的坑**：模板是 `data\templates\<name>.md` 文件；名字会清洗（小写字母/数字/连字符），清洗后为空直接拒；坏文件跳过。

**① 入口**：设置页「模板」面板管理；对话页斜杠菜单与 Ctrl+K 命令面板选中后把正文填进输入框。

**② 后端链路**：`api/routes_templates.go` → `service/templates.go`（List/readOne/Commands/Save/Delete/sanitizeTemplateName）→ `pkg/frontmatter.go`（YAML 头 `name`/`description`/`args`）→ `chat_state.go commands()` 以 `command_source="prompt"` 追加，`command_body` 带正文；`args` 非空时 desc 拼成 `args · desc`。

**③ 前端链路**：`components/settings/TemplatesPanel.vue` → `composables/useComposerMenu.ts`（prompt 类填正文）→ `components/common/CommandPalette.vue`（Ctrl+K：导航/动作/最近会话/命令）。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/templates` | — | `rows[]{template_name,template_desc,template_body,template_args}` |
| POST | `/templates/save` | `template_name,template_desc,template_body,template_args` | `{ok}` |
| POST | `/templates/delete` | `template_name` | `{ok}` |

**⑤ SSE**：无；下次 `chat/state` 的 `commands[]` 自然带上新模板。

**⑥ 错误码**：1000 名字为空或清洗后为空；1003 目录创建/写入/删除失败。现象「菜单里没有我的模板」→ 先看 `/templates` 是否有行（扩展名必须 `.md`、frontmatter 没写坏），再看 `chat/state` 的 `commands[]` 里 `command_source` 是否为 `prompt`。

---

## 14. 系统 · 托盘 · 壳 + 引导流程

**最容易踩的坑**：`/engines/restart` 只是「中止当前运行 + 广播 `engine:ready`」，进程内引擎没有可重启的进程；`/engines/detect` 与 `/engines/repair` 完全等价。

**① 入口**：托盘（显示窗口 / 最近工作目录 ≤5 / 打开数据目录 / 退出；单击图标唤回）、设置页引擎/诊断/关于面板、`/apps`（git/node/python 探测）。引导：`main.go` → `shell.Run` → 前端 `getEndpoint()` → `api.bootstrap()` → 建 SSE → `Ready()` 显窗。

**② 后端链路**：`main.go`（embed 前端产物 + 版本）→ `shell/app.go`（`Run`：`resolveLayout` → `pkg.SetLogDir` → Wails 应用 + 单实例 `com.workbaby.desktop` → 隐藏建窗 → 建托盘 → `ServiceStartup`→`assemble`（开 store/db/hub/token → `service.New` → `api.New.Start`）→ `go Chat.Warm()`）；`shell/window.go`（Ready/showWindow/toTray/askCloseDecision/ApplyCloseDecision/requestQuit）、`tray.go`（菜单重建、最近目录）、`endpoint.go`、`windows.go`（DPI 缩放、致命弹框）。`--hidden` 只上托盘不显窗；`WB_DEV_TOKEN` 覆盖令牌。配置原文白名单只有 `workbaby.json` 与 `mcp.json`。`GET /engines` 的 `instance` 现造（`started_at` 是当前时刻，不是启动时刻）。

**③ 前端链路**：`main.ts`（Pinia/Router/i18n/ElementPlus）→ `App.vue`（自绘标题栏 + `AppRail` + 主区；`app:close_requested` 弹选择后 `applyCloseDecision(action, remember)` 回后端；首屏画完调 `notifyReady()`）→ `stores/app.ts load()`（bootstrap → 应用主题/字号/语言/动效 → `connect()` → 重拉会话与引擎 → `restoreLastSession()`）。

**④ HTTP 接口**

| 方法 | 路径 | 入参 | 出参 |
|---|---|---|---|
| GET | `/engines` | — | `engines[]{engine_kind,engine_label,engine_exe,engine_script?,engine_source,engine_version,installed,caps},layouts[]{engine_kind,agent_dir,sessions_dir,skills_dir,settings_file,dir_exists},runtime,instance,detected_at` |
| POST | `/engines/detect`、`/engines/repair` | — | `runtime,apps[]`（等价，均广播 `engine:detected`） |
| POST | `/engines/restart` | — | `{ok}`（`Chat.Stop()` + 广播 `engine:ready`） |
| GET | `/health/engines` | — | `health_engines[]{engine_kind,engine_label,engine_version,installed,running,busy,session_count,skill_count,mcp_count,config_files,caps},active_engine,workspace,runtime,total_sessions,total_skills` |
| GET | `/apps` | — | `apps[]{app_key,app_name,app_version,app_path,installed,required,app_source,can_locate,blocker?,install_note?}` |
| GET | `/config/files` / `/config/text?name=` | `name`（`settings`\|`mcp`） | `entries[]{config_name,config_label,file_path,file_exists,size_bytes}` / `file_path,file_text,truncated,found` |
| POST | `/system/reveal`、`/system/open` | `target_path` | `{ok}` |

**⑤ SSE**：`engine:detected`（重新探测）、`engine:ready`（仅 restart）、`app:close_requested`（首次关窗询问）、`stream:open`（连接首帧）。

**⑥ 错误码**：1000 请求体不合法（`detail` 是解析错原文）；1001 目标路径不存在；1003 数据目录不可写或配置原文读取失败；1404 未知路由；9999 未归类内部错误（看 `detail` 与当日日志）。引导失败顺序：`Endpoint()` 的装配错误（20s 超时也算失败）→ `data\logs`（`/logs?lines=`）里的「业务装配失败 [码]」→ 数据目录可写性。「托盘没有最近目录」→ `workspace.history` 为空或目录已不存在（`Workspaces` 会过滤）；「窗口不出现」→ `--hidden` 与 `Ready()` 是否被调用。
