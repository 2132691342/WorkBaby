# 模块手册

按功能域组织，排障与接手都从这份文件进。每节七段固定骨架：**① 定位**（给谁用、在哪儿用）→ **② 设计与理由**（怎么搭的、为什么）→ **③ 实现链路**（后端 `service`→`agent`/`repo` 与前端的真实符号）→ **④ 接口与事件** → **⑤ 错误码** → **⑥ 不变量与坑** → **⑦ 取舍**（换来什么、代价是什么）。

字段级入参出参与错误码总表只有一份，在 `docs/API.md`（本文件每节的 ④ 指向它对应的 §3.x）；存储形态见 `docs/DATA-MODEL.md`，分层与依赖方向见 `docs/ARCHITECTURE.md`，工程规范见 `AGENTS.md`。
面向使用者的任务向导在 `docs/USER-GUIDE.md`；按症状与按码的排查在 `docs/TROUBLESHOOTING.md`（本文件各节的 ⑤ 是它的素材来源）；
工具清单与硬上限在 `docs/TOOL-REFERENCE.md`；`api_protocol` 在探测与聊天两条路上的差异在 `docs/PROVIDERS.md`。
全局约定：前缀 `/api/v1`，只用 GET / POST，动词靠路径段；信封恒 HTTP 200、体为 `{code,message,detail?,data}`，`code != "0"` 即业务失败；HTTP 鉴权走 `X-Desktop-Token`，SSE 走 `?token=`，鉴权失败是唯一不带信封的出口（裸 401）。业务码定义在 `backend/pkg/codes.go`，界面直接显示后端 `message`。

依赖取舍（开发期）：`@types/dompurify`——dompurify 3.1.6 不自带类型声明，无它则 `utils/md.ts` 的消毒出口 TS7016 报隐式 any。替代方案是自己写一份 `declare module` 垫片；代价是多一个 devDependency，不进运行时产物。消毒本身是安全出口，宁可要类型检查也别绕过它。

## 1. 对话（计划模式 · 附件 · 斜杠命令 · 队列 · 提问卡）

**① 定位**：`/chat`（`views/ChatView.vue`）——左栏会话列表、主区轮次流、底部输入区、右侧工作台，承载一轮问答的全部编排。

**② 设计与理由**

- 引擎在进程内：一次运行 = 一个 `agent.Runner`，`Chat` 只持 `running` / `runner` / `cancel` / `queued` / `waiters`。没有拉起、握手、崩溃重启，也没有冷启动等待——有会话文件就能画出状态。
- 轮次闭环在 `agent/loop.go`：`Run` 先把用户消息落进会话文件，再进 `loop`。每步 `streamOnce` 调模型 → 要工具就 `runTools` 执行并落盘 → 回到顶部由 `history()` 把工具结果读回去再调一次模型 → 不要工具时 `drainQueue` 有插话就继续、没有就收尾。上限 `maxSteps = 24`，用满时明确报错而不是静默截断。
- 工具顺序执行而非并发：工具会改同一批文件，并发写同一份文件的结果不可预测。
- 上下文以压缩条目为分界：`Runner.history()` 取最后一条 `entry_summary` 非空的 `compaction`，它之前（含它本身）全部折成一条摘要引导消息（`summaryMessage`），之后原样带上，`system` 角色跳过。原文一条不删，回放仍看得清发生过什么。
- 中止保留半截：`ctx` 取消时 `persistPartial` 把已吐出的内容以 `stop_reason="aborted"` 落盘——用户看过的字不该在重开应用后少一段。
- 计划模式闸门挂在工具注册表上（`agent/policy.go` 的 `ToolPolicy` 与 `Registry.GuardPolicy`），一处实现管住全部写工具；逐工具各判一次等于给每个新工具留一个「忘了判」的口子。`IsMutatingTool` 先查只读白名单 `readOnlyTools`（`read` `grep` `find` `ls` `todo` `ask` `web_search` `web_fetch`），再查 `mutatingTools`，两份都没登记的工具按「会改」处理。拦住时 `planDeniedResult` 说清「现在是计划模式 + 下一步做什么」，而不是一句禁止。
- 开关状态住在会话文件里：`SetPlanMode` 追加 `custom` 条目（`customType="plan-mode"`），横幅与闸门都从它派生（`Sessions.PlanMode` + `applyPlanState`）；分成两份内存状态必有一份先过期。
- 队列两条、语义不同：`Steer` 进 `Runner.queue`，本轮收尾前由 `drainQueue` 折进上下文；`FollowUp` 进 `followUp` 队，本轮结束后上层逐条 `Stream.Begin` 另起一轮。正在跑时点发送走前者——报错让用户重打一遍不是「插话」的语义。
- 提问是阻塞式工具调用：`Chat.ask` 把 channel 挂进 `waiters`、广播 `chat:ask`，然后 `select` 等 `/chat/answer` 回填或 `ctx` 取消，模型在等用户回话，这一轮不会往下走。
- 附件三类、成本不同：`file` 只在正文追加 `[附件] <绝对路径>`，让模型按需自己 `read`；`image` 走内容块（唯一的二进制通道）；`session` 整段折进正文——被引用的会话文件在工作区外，读工具读不到它。

**③ 实现链路**

- 路由 `api/routes_chat.go`（`/chat/*`、`/messages/*`，挂在 `api/server.go mount`）→ 编排 `service/chat_service.go`：`Send` / `Stop` / `Steer` / `FollowUp` / `ClearQueue` / `refreshQueue` / `ask` / `handler` / `resolveModel` / `autoCompactionDue` / `BindSession` / `NewSession` / `Busy` / `CurrentFile`。
- 动作 `chat_actions.go`：`compose` / `Compact` / `compactNow` / `summarize` / `compactionPrompt` / `Answer` / `appendAnswer` / `RemoveQueued` / `Export` / `SetAutoCompaction`；会话处置 `chat_session.go`：`CloneSession` / `Fork` / `Rename` / `SetModel` / `SetThinking` / `Warm` / `Shutdown` / `OnWorkspaceChanged`。
- 状态总装 `chat_state.go`：`State` / `usageSnapshot` / `contextWarnText` / `modelInfo` / `commands` / `nativeCommands` / `emptyState` / `HandleCommand` / `togglePlan`；计划模式 `plan_mode.go`：`PlanMode` / `SetPlanMode` / `applyPlanState` / `planDeniedToolNames` / `currentPlanState` / `TogglePlan`。
- 事件翻译 `chat_stream.go`：`Stream.Begin` / `TextDelta` / `ThinkingDelta` / `appendText` / `ToolStart` / `ToolEnd` / `emitTodos` / `AssistantMessage` / `Usage` / `Fail` / `Finish` / `emitPatch` / `emit` / `turnUsage`。引擎只发 domain 词汇，翻成 SSE 是这一层的事。
- 上游与工具：`agent/provider_openai.go` 是唯一适配器（POST `<base_url>/chat/completions` 流式；`low`/`medium`/`high` 填 `reasoning_effort`，`off`/`minimal` 不发），`agent/tool.go`（`Registry` / `NewRegistry` / `AddWebTools`）+ `tool_{fs,shell,search,index,todo,ask,web,mcp}.go` + `tidy.go` 输出折叠 + `session.go`。现役工具名：`ask` `read` `write` `edit` `ls` `grep` `find` `bash` `todo` `index_search` `web_search` `web_fetch`，外加 MCP 的 `mcp_{server}_{tool}`。
- 发送判定顺序（`Send`）：`compose` 后正文空白 → 1000 → 无附件时先 `HandleCommand`，被消费回 `handled` → `Busy()` 转 `Steer` 回 `queued` → `BindSession` → `resolveModel` → `Stream.Begin` 回 `started` + `run_id` → goroutine 里判自动压缩、`runner.Run`、逐条派发 `FollowUpQueued()`（`ctx` 已取消就不再派发）。
- 自动压缩三个条件缺一不可：`model.auto_compaction != "0"`、`context_window > 0`、`ContextUsage()/window > 0.8`；失败只 `pkg.Warn` 不挡本轮。水位口径见 `Session.ContextUsage()`：以最后一条 `compaction` 重新起算，取其后最后一条带 usage 的助手消息的 `input+output+cache_read`，一条都没有就按字符数 / 4 估。
- 压缩本体 `compactNow`：`resolveModel` → `compactionPrompt`（无正文 → 1000）→ 广播 `start` → `summarize`（120s 同步、不带工具、不发思考档）→ `AppendCompaction` → 广播 `done`。三条出口都补 `done`，界面不会卡在「压缩中」。
- 中止（`POST /chat/stop`）把被作废的两条排队话术原样回家：`Stop` 先取队再 cancel，返回 `QueuedMessagesVO`。用户刚打的字不会无声消失。
- 思考档存 `model.thinking_level`，后端接受 `off/minimal/low/medium/high`，`thinking_levels` 只列 `off/low/medium/high`（`minimal` 不在界面出现）。
- 生效档的取法在 `resolveThinkingLevel`（`service/chat_service.go`）：当前档优先，用户没选过就用 `model.default_thinking_level`，两处都缺省才落到常量 `fallbackThinkingLevel`（`medium`）。新会话建立时（`BindSession` / `NewSession`）重取一次，所以设置里改默认档对新会话真的生效。
- 模型定义里的 `max_tokens` 随 `Runner.MaxTokens` 进上游请求体：不传的话上游按自己的默认值给，用户在模型页设的上限就永远对不上。
- 斜杠命令双轨：后端 `nativeCommands` 声明 `new`/`compact`/`export`/`plan`，`HandleCommand` 只处理 `/new` `/compact` `/plan`（`/plan off` 退出，不带参数按当前状态取反），`/export` 由前端本地执行；前端 `useBuiltinCommands.ts` 的 `BuiltinKey` 另列 `model` `thinking` `copy` `exporthtml` `reload` `clone`，按 `caps` 过滤（`exporthtml`→`export_html`、`clone`→`session_clone`，不满足就不进菜单）；模板命令以 `command_source="prompt"` 并进同一菜单。带附件时不认斜杠命令。
- 前端：`views/ChatView.vue` → `stores/chat.ts`（`send` / `retrySend` / `steer` / `followUp` / `answer` / `compact` / `stop` / `clearQueue` / `removeQueued` / `newChat` / `cloneChat` / `forkChat` / `setModel` / `setThinking` / `setAutoCompaction` / `setPlanMode` / `hideTurn` / `unhideTurn` / `refresh` / `applyState` / `finishTurn` / `reset`）+ `stores/events.ts routeFrame` → composables `useMessageFlow`（`applyPatch` / `beginTurn` / `applyHistory` / `setUsage` / `turnPresence`）、`useRunStatus`、`useComposerDraft` / `useComposerMenu` / `useComposerActions`、`useAttachments`、`useBuiltinCommands`、`useEngineModels`、`useTurnClock`、`useWorkPanel`。
- 组件：`components/chat/` 下 `HistoryList` `MessageTurn` `TurnBlocks` `ToolCallCard` `ThinkingBlock` `AskCard` `TodoPanel` `ChatComposer` `ComposerBar` `ComposerStatus` `ComposerMenu` `MentionBrowser` `ModelPicker` `ContextMeter` `ActivityBar` `ChatNotices` `TurnChanges` `WorkPanel`；左栏复用 `components/sessions/SessionList.vue`。
- 右侧工作台 `WorkPanel.vue` → `workspace/FileInspect.vue`：常驻、可拖宽、多标签并排留存一次会话看过的文件；每个标签是「差异 / 文件」双标签（`WbSeg`），差异问 `api.gitDiff(相对路径)` 拿工作区真实那份给 `DiffView`，文件问 `api.fileRead(绝对路径)`，文本走 `pre`、二进制走 `FilePreview`。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/chat/state` | 聊天页首屏总装：会话实况、回放历史、待办、计划模式、命令表、用量与水位 |
| POST | `/chat/send` | 发一句话：起一轮运行、转成插话排队，或当场消费掉斜杠命令 |
| POST | `/chat/steer` | 往正在跑的这一轮插一句话 |
| POST | `/chat/followup` | 排一句追加话，本轮结束后另起一轮 |
| POST | `/chat/stop` | 中止当前运行 |
| POST | `/chat/clear-queue` | 清空插话队与追加队 |
| POST | `/chat/remove-queued` | 摘掉队列里的指定一条 |
| POST | `/chat/compact` | 手动压缩上下文 |
| POST | `/chat/answer` | 回答提问卡，唤醒阻塞中的那一步 |
| POST | `/chat/new` | 开一份新会话 |
| POST | `/chat/clone` | 克隆当前会话 |
| POST | `/chat/fork` | 从一条用户消息分叉出新会话 |
| POST | `/chat/export-html` | 把会话导出成自包含 HTML |
| POST | `/chat/model` | 切换当前模型 |
| POST | `/chat/thinking` | 切换思考档 |
| POST | `/chat/autocompact` | 开关自动压缩 |
| POST | `/chat/plan` | 开关计划模式，返回生效后的状态与被拦工具 |
| POST | `/messages/hide` | 隐藏一轮 |
| POST | `/messages/unhide` | 取消隐藏一轮 |

字段级入参出参见 `docs/API.md` §3.4。事件（载荷见 `docs/API.md` §4，触发时机见 §2）：`chat:start` `chat:live` `chat:patch` `chat:turn` `chat:turn_end` `chat:todos` `chat:ask` `chat:error` `chat:settled` `chat:done` `chat:compaction` `chat:queue` `chat:model` `chat:plan` `session:info`。

**⑤ 错误码**

| 码 | 现象 → 先查哪 |
|---|---|
| 1000 | 消息 / 插话 / 追加 / 回答为空（只发附件时 `compose` 后正文为空即拒）；无话可压缩；队列类型不合法；要移除的下标越界 |
| 3000 | 服务商缺密钥：看 `/providers` 的 `has_key`，日志 `服务商缺密钥 code=3000` |
| 3001 | 没选模型或所选模型已删（同时 `DropActiveIf` 清坏指针），日志 `解析活跃模型失败 code=3001` |
| 5000 | 未选工作区：`BindSession` 第一行即判 |
| 5001 | 会话条目落盘失败（`agent/session.go append`），经 `OnError` → `chat:error` |
| 5002 | 有运行在跑：压缩 / 克隆 / 分叉全拒，先 `/chat/stop` |
| 5003 | 分叉点不在这份会话的条目里；没有运行可插话 / 追加 / 移队；当前无会话可导出 |
| 7000 | 压缩总结调不通上游、被拒、返回空内容（日志 `压缩总结失败`） |

**⑥ 不变量与坑**

- `disposition` 有三种，只有 `started` 起运行。`stores/chat.ts send` 对非 `started` 显式把 `streaming` 置回 false——当成 `started` 会让界面永远卡在「正在回答」；`handled` 再重拉一次 state（`/plan` 这类命令往会话里写了东西）。
- 消息渲染的三条稳定性约束，改这里会坏那里：
  - `.msg` 不使用 `content-visibility`（`content-messages.css`）——它让离屏节点的 `el` 在 Vue 块树补丁里变 null，一次崩溃冻住整片消息区；DOM 规模由 `HistoryList` 的 `PAGE = 40` 分页（首屏 40 轮 + 向上补页）约束。
  - `utils/activity.ts groupBlocks` 跳过未到位的下标：增量补丁可能先落到靠后块，读 `undefined.block_kind` 会崩掉整轮。
  - 收尾去重判据是「历史里最后一条用户消息就是这句、且它后面已有助手回复」：`useMessageFlow.applyHistory` 据此撤直播位，`HistoryList.liveUserVisible` / `liveAssistantVisible` 同判据（`turnPresence`）。逐块文本比对会漏判——工具块字段在直播位与历史位常不一致，漏判的结果是整轮画两遍并卡在「正在回答」。
- 贴底由 `ChatView.vue` 的 `schedulePin` 用 rAF 合帧：同帧只滚一次，距底 < 2px 不再推。
- 工具块的 `file_path` 必须随结果落盘并在回放时投影：`parser/history.go fillToolResult` 读 `MessageDetails["file_path"]` 与 `["diff"]` → `block.file_path` / `block.diff_lines`。只投 diff 不投路径，工具卡的「看改动」与轮尾 `TurnChanges` 芯片会一起消失。
- `useWorkPanel` 是模块单例（`tabs` / `activeId` / `width` 在模块作用域）：切页再回来标签还在；`openWorkFile` 按路径去重并激活，`closeWorkTab` 把激活位回退到相邻标签，关到空自动收起。宽度键 `wb.workpanel.width`，`clampWidth` 夹 300~720、默认 420。
- `Chat.AskPending()` 没有调用方，`/chat/state` 也不带待答清单字段：提问期间刷新界面，卡片回不来。`chat:ask` 载荷里没有超时字段（`AskRequestVO.AskTimeoutMs` 不下发），`ask` 只等回答与 `ctx`，不会自动超时作废。
- `context_warn` 后端会填（> 0.8 且自动压缩开着），前端不消费，也不影响其它字段。
- `/chat/remove-queued` 的 `queue_kind` 只校验取值，`Runner.RemoveQueued(index)` 摘的是插话队那一条。
- 回答一条已经不在等的提问不会被丢：`Answer` 走 `appendAnswer` 当普通用户消息追加进会话，取消则追加「（用户取消了那次提问）」。
- 阅读带宽度走 `--wb-col`（`themes.css`：`min(calc(880px * var(--wb-vw-scale)), 100%)`），消息列、输入区、活动条共用同一刻度；轮尾用量带 token 组成条（`MessageTurn.vue`，缓存 / 输入 / 输出按占比分段）。空态三枚引导建议（`chat.suggest1..3`）只填进输入框、不直发。
- 发布版没有控制台，失败必须有日志：`聊天发送绑定会话失败`、`聊天发送取模型失败`、`聊天发送排队失败`、`聊天运行失败`、`压缩总结失败`、`会话初始化失败`。

**⑦ 取舍**：换来一轮一运行、状态可脱离 HTTP 直接读文件、中止不丢半截回复、写工具准入只有一处判定；代价是同进程只跑一轮（`Busy()` 就转插话）、引擎与壳同生死、未登记的只读工具会被计划模式误拦（保守方向）。

## 2. SSE 事件协议（`GET /events`）

**① 定位**：界面实时状态的唯一通道——一轮回答的增量、待办、队列、各处清单变更都靠它推。

**② 设计与理由**

- `seq` 由 hub 统一发号（`Hub.Broadcast` 里 `h.seq++`），调用方不许自定：事件顺序是契约，界面靠 `seq` 判断有没有漏帧。帧形如 `{seq,event,data,sent_at}`。
- 断线补帧：`Hub` 保留最近 1024 帧（`shell/app.go` 的 `sse.New(1024)`；`New` 对 ≤ 0 兜底 1000），`Subscribe(since)` 把 `since` 之后的帧先入队。补不上的缺口不假装补齐，交给前端 `reconcile()` 用 `chat/state` 全量对账。
- 每订阅者一条 64 缓冲队列 + 发送泵：补帧先入队、实时帧后入队，乱序被结构性排除；入队不阻塞，写满直接 `sub.close()` 摘掉该连接——一个卡住的窗口既不拖住别的客户端，也不拖住广播本身。
- 心跳 25s 一行 SSE 注释 `: ping`（`sse/heartbeat.go` 的 `Heartbeat(0)` 与 `PingLine`），不占 `seq`。帧与心跳来自两个 goroutine 却写同一个 `ResponseWriter`，`api/routes_events.go` 用 `writeMu` 串行化——交错写会把一帧劈成两半，前端静默丢帧。
- 首帧 `stream:open` 带当前 `seq`，客户端据此决定要不要补帧。
- 聊天帧经 `sse.EngineTag` 补 `engine_kind:"native"`，`Stream.emit` 再补 `run_id`：界面要能一眼看出这帧属于谁、属于哪一轮。`chat:compaction` 内联 `engine_kind`，不走 `EngineTag`。

**③ 实现链路**

- 后端：`sse/hub.go`（`New` / `Subscribe` / `deliver` / `Broadcast` / `Seq` / `EngineTag` / `Encode`）、`sse/heartbeat.go`、`api/routes_events.go`（长连接、首帧、心跳、`?since=`）。生产者只有 `Hub.Broadcast`，`service/*` 不关心谁在听。
- 前端：`stores/app.ts connect()` 建 `EventSource(streamURL(lastSeq))` → `utils/sse.ts readFrame` → `stores/events.ts routeFrame`。`frame.seq > lastSeq+1` 就 `reconcile()`（`chat.refresh(chat.sessionFile)`）；`onerror` 关流后定时重连，退避 1200ms 逐次 ×2、上限 15000ms，`onopen` 复位；`isChatScoped` 只把 `chat:*` 按运行过滤，`engine:* / app:* / session:*` 是全局事实，不能被某轮过滤掉。

**④ 接口与事件**：`GET /events?token=&since=`，`text/event-stream`；鉴权失败回裸 401。事件名在帧体的 `event` 字段，SSE 记录本身用默认 `message`；载荷字段见 `docs/API.md` §4。

| 事件 | 触发时机 | 界面反应 |
|---|---|---|
| `stream:open` | 连接建立后的首帧 | 忽略 |
| `chat:start` | `Stream.Begin` 登记一次运行 | `streaming=true`，清错、重置读数、秒表从 `started_at` 起跑 |
| `chat:live` | 正文 / 思考增量、工具开始 | 活动条与 tokens/s（`sampleSpeed` 按 run 取样）；不是当前轮的 `run_id` 就丢 |
| `chat:patch` | `emitPatch`：文本追加、整块替换 | `applyPatch`：`delta` 追加尾部、`block` 替换那一块 |
| `chat:turn` | 一条助手消息已落盘 | 权威快照覆盖块表；已收尾的 `run_id` 丢掉（否则尾部多一条重复回复） |
| `chat:turn_end` | 同上一条，随后 | `setUsage(run_id,…)`，取最后一次非零帧而不是累加 |
| `chat:todos` | `todo` 工具的结果落地后 | 待办面板实时更新 |
| `chat:ask` | 模型发起一次提问 | 追加提问卡，按 `ask_id` 去重 |
| `chat:error` | 一次运行失败（`Stream.Fail`） | 标错 + toast，并解流式态 |
| `chat:settled` | 运行收尾（`Stream.Finish`） | 只解流式态 |
| `chat:done` | 与 `chat:settled` 连发 | `finishTurn` + 重拉 state 与列表 + `bumpWorkspace` |
| `chat:compaction` | 压缩开始与结束（1 次 start + 3 次 done） | `compacting = state==='start'`，`done` 补提示 |
| `chat:queue` | 入队、清空、刷新队列镜像 | 队列整体替换 |
| `chat:model` / `chat:plan` | 切模型 / 开关计划模式 | 无分支（落 `default` 只 `console.warn`） |
| `models:changed` / `mcp:changed` | 模型增删改 / MCP 条目变更 | 重拉模型下拉并纠正被删模型 / 无分支 |
| `session:info` | 新建 / 克隆 / 分叉 / `/new` | 重拉会话列表 |
| `skill:status` | 技能安装、开关、重置、导入、新建、删除（6 处） | `bumpSkills` → 技能页重拉 |
| `workspace:changed` | 切换工作区 | 无分支（发起方自己更新） |
| `engine:detected` / `engine:ready` | 重新探测外部工具 / `/engines/restart` | 重拉运行时；`ready` 另在空闲时重拉 state |
| `app:close_requested` | 第一次点关窗按钮 | 弹「收进托盘 / 退出」 |

**⑤ 错误码**：SSE 不走信封，没有业务码；只有鉴权失败的 HTTP 401。

**⑥ 不变量与坑**

- 后端发了、前端没有分支的 4 个：`chat:model` `chat:plan` `mcp:changed` `workspace:changed`。后果是多窗口 / 多页面之间不会自动同步，只有发起动作的那一份会更新。
- 前端有分支、后端从不发的 11 个（命中即无反应）：`engine:status` `engine:exit` `engine:commands` `app:progress` `chat:blocks` `chat:notify` `chat:status` `chat:widget` `chat:retry` `chat:retry_end` `chat:editor_text`。后果：安装日志面板（`components/apps/AppProgress.vue`）永远空转；`useRunStatus` 的提示只能由前端本地 `pushNotice` 等填。
- `session:title` 后端从不发：唯一的发射点 `Chat.Rename`（`service/chat_session.go:103`）没有调用方，`/sessions/rename` 走 `Sessions.Rename`，它只追加改名条目不广播；改名后的列表刷新由前端本地承担。
- 排障顺序：「卡在运行中」查有没有收到 `chat:settled` / `chat:done`；「切回来少一段」查 `seq` 跳号与 `reconcile()`；「界面没反应」先确认这帧有没有人发、有没有人接；「SSE 反复重连」说明某条连接的 64 队列被写满过。
- 未知事件只 `console.warn` 一次（`warnedEvents`）：后端加了新事件而这张表没接时，控制台里只有一行。

**⑦ 取舍**：换来单向广播 + 断线可补 + 顺序有保证，界面不必轮询；代价是补帧窗口固定 1024 帧（长断线必须靠全量快照对账），每连接一条 goroutine 与一个 channel（广播成本随连接数线性增长），慢消费者被直接断连而不是降级丢帧。

## 3. 会话历史与会话页

**① 定位**：`/sessions`（`views/SessionsView.vue`）——范围「当前目录 / 全部目录」+ 汇总 + 列表；列表组件与对话页左栏共用。

**② 设计与理由**

- 一个工作区一个目录：`data\sessions\<pkg.EncodedWorkspace(工作区)>\SES_<ULID>.jsonl`。`EncodedWorkspace` 把 `:` `/` `\` 空格统一换成 `-`、前后加 `--`，Windows 路径才能安全当目录名。
- 只追加不改行：`agent.Store.Open` / `Append*` 是唯一写入口，每条带 `EntryParentId = 上一条目 ID`，`parser.ActivePath` 从最后一条沿 parent 回溯再反转得到活动分支。整份重写在断电时会把唯一真相写坏，会话头因此只写一次。
- 一切派生：标题（改名条目优先，否则首条用户消息截断 80 字）、`session_model`、`updated_at`、`total_tokens`、`cost_usd`、`message_count` 都由 `parser.Summarize` 扫条目得出，不存第二份。
- 清单口径：给了 `workspace` 只扫它的目录（不存在给空），否则扫全部子目录后按 `updated_at` 降序混排；`q` 先比标题、未命中才读全文；单个坏文件跳过而不是让整张清单报错。
- 隐藏轮不碰文件：台账键 `chat.hide:<会话文件>` 存被隐藏的轮 ID，`filterHidden` 在回放末尾剔除。
- 待办与计划模式都是回放产物：`Todos` 取活动分支上最近一次 `todo` 工具结果的 `details.tasks`（`status="deleted"` 的行剔除），`PlanMode` 取活动分支上最后一条 `custom`（`customType="plan-mode"`）的 `data.active`，没有这种条目就是关。
- 分叉三条纪律（`Chat.Fork`）：`turn_id` 为空 → 1000；有运行在跑 → 5002；`turn_id` 不在这份会话的条目里 → 5003。复制只取消息条目、到分叉点（含）为止，原会话一个字节不动，新会话落在同一工作区目录并 `RememberSession`。`CloneSession` 同规则但不截断，且只复制消息条目、不复制模型切换与用量等元信息条目。

**③ 实现链路**

- 后端：`api/routes_data.go`（`/sessions*`）+ `api/routes_chat.go`（`/messages/*`）→ `service/sessions.go`（`List` / `matches` / `scanDirs` / `Detail` / `Delete` / `Rename` / `History` / `Todos` / `PlanMode` / `Hide` / `Unhide` / `Export` / `Count` / `hiddenSet` / `hideKey`）→ `service/parser/`（`entries.go ReadEntries`/`ActivePath`、`history.go BuildHistory`/`HistoryFromEntries`/`blocksOf`/`applyToolResult`/`fillToolResult`/`filterHidden`/`GuardSession`/`DetailItems`、`summary.go Summarize`、`plan.go`、`todos.go`、`diff.go`、`roles.go`、`stat.go`、`time.go`）→ `agent/session.go`（`Create` / `Open` / `AppendMessage` / `AppendUsage` / `AppendCompaction` / `AppendCustom` / `Rename` / `Entries` / `ContextUsage` / `titleFromEntries`）；HTML 导出在 `service/session_html.go renderSessionHTML`。
- 前端：`views/SessionsView.vue`（`refreshSeq` 竞态守卫，`scope` 切范围就重拉）→ `components/sessions/SessionList.vue`（首屏 `PAGE_SIZE*2 = 120` 行、「显示更多」每次再放 120；搜索 400ms 防抖后交给后端 `q`；`loadSeq` 序号守卫；watch `refresh.sessions`）→ `stores/app.ts refreshSessions`（更新全局清单后 `bumpSessions()`，信号在 `stores/refresh.ts`）。打开会话 = `router.push({name:'chat',query:{session:path}})`。
- 刷新链路：`chat:done` → `refreshSessions()` → `bumpSessions()` → `SessionList` 重拉自己那一页；删除 / 改名由组件自己 `load()` 并 `emit('changed')`，`SessionsView.onChanged` 再 `app.refreshSessions()`。只靠 mount 时那一次加载，聊完一轮侧栏会停在「（空会话）· 0 条」。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/sessions` | 会话清单，可限定当前目录、可按关键词过滤 |
| GET | `/sessions/detail` | 单个会话的清单行 + 逐条消息 |
| POST | `/sessions/delete` | 删除会话文件，并清掉它的隐藏台账 |
| POST | `/sessions/rename` | 改名：追加一条改名条目，不改写文件 |

字段级入参出参见 `docs/API.md` §3.3（隐藏轮的两条在 §3.4）。事件：`session:info` 触发列表重拉；`chat:done` 也走 `refreshSessions` + `bumpSessions`；隐藏 / 取消隐藏 / 改名 / 删除都不发事件。

**⑤ 错误码**：5000 未选工作区；5001 路径越界或非法、会话删除失败（`parser.GuardSession` 要求落在 `data\sessions` 下）；5003 会话文件不存在；5002 改名或删除被拒（这份会话正被引擎打开，先切走）；1000 标题为空、隐藏缺 `session_file` 或 `turn_id`。

**⑥ 不变量与坑**

- 页面顶部合计恒为「当前目录」口径：`counts` 读 `app.sessions`，与列表的 `scope` 无关。
- `/sessions/delete` 入参是 `session_path`，不是 `path`。
- `History` / `Todos` / `PlanMode` / `Detail` 走同一个 `GuardSession`：两条路径对同一个文件用不同规矩，等于给越界读取留后门。
- `Summarize` 扫全部条目而非活动分支：`session_model` 取文件里更靠后的那条事实——`model_change` 条目写 `provider/model_id`，助手消息写 `message_model`，谁后谁生效；标题同理，最后一条带 `name` 的改名条目赢过首条用户消息。
- `Count` 只数文件名、不解析内容：健康面板每次引擎事件都会刷新它，走 `List` 会把每份会话全文读两遍。
- 删会话会顺带清它的 `chat.hide:<会话文件>` 键，写失败只 `pkg.Warn`。
- 删除避让引擎正在打开的那份会话（`Active` 判定，回 5002）：删掉之后它还在追加，界面会继续显示一份读不回来的会话。

**⑦ 取舍**：换来文件即真相——可备份、可手改、重启零状态，分支与隐藏都不需要额外表；代价是清单要扫盘（于是有分页、`Count` 不解析、搜索先比标题），派生字段散在解析层，改口径必须同时改回放。

## 4. 模型服务商

**① 定位**：`/models`（`views/ModelsView.vue`）——服务商列表、弹窗编辑、模型表、默认模型卡；对话页下拉吃同一份清单。

**② 设计与理由**

- 持久化在 SQLite 两张表 `provider` / `provider_model`（`repo/provider_repo.go`，`repo/db.go` 的 `AutoMigrate` 只增列不改列）。设置台账只存指针：`model.active_provider`、`model.active_model`、`model.thinking_level`、`model.auto_compaction`，外加三个默认键 `model.default_*`。
- 密钥不回显：读出来恒为掩码（`maskKey`：空 → 空，≤ 8 字符 → `••••`，否则前 4 + `••••` + 后 4）；入参空串的语义是「保留原值」——界面回传的必然是掩码，当成新值存会把密钥写坏。
- 整表替换式对账：`Upsert` 在 `req.Models != nil` 时先 upsert 请求行、再删库里多出的行，每删一行调 `DropActiveIf(provider,modelId)`；`== nil` 时保留原清单，改地址不该把模型清空。`DropActiveIf` 按名字判并同时清 `model.active_*` 与 `model.default_*`——留着坏指针，下一句话就撞同一句「模型不存在」。
- `api_protocol` 只影响「探测列模型」：`probeRequest` 对 `anthropic` / `google` 各有端点与鉴权头，其余一律按 OpenAI 兼容；协议字段不校验，非空即原样存下。
- 默认三键只被本页读写：发送链路只读 `model.active_*`，避免「默认」与「当前」互相覆盖。

**③ 实现链路**：`api/routes_data.go`（`/models*`、`/providers*`）→ `service/models.go`（`Providers` / `Options` / `Lookup` / `Upsert` / `SaveModel` / `DeleteModel` / `Delete` / `DropActiveIf` / `ProviderOf` / `Defaults` / `SaveDefaults` / `resolveProbe` / `changed` / `maskKey` / `modelEntryFromDO` / `modelDOFromVO` / `splitKinds` / `joinKinds`）→ `service/model_probe.go`（`Probe`、`probeTimeout = 20s`、`probeRequest`、`parseModelList`、`modelEntryFromList`）→ `repo/provider_repo.go`（`Provider` / `Providers` / `Upsert` / `Delete` / `Models` / `AllModels` / `FindModel` / `UpsertModel` / `DeleteModel` / `SortedProviderNames`）。前端 `views/ModelsView.vue`（`AbortController` + `loadSeq`、表单就地校验、删除用 `ElMessageBox` 二次确认并写明后果）→ `components/models/{ProviderList,ProviderDialog,ModelTable,DefaultModelCard}.vue` → `composables/useProviderForm.ts`；对话页下拉走 `composables/useEngineModels.ts`，靠 `models:changed` 重拉。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/models` | 全部服务商下所有模型的扁平清单（下拉数据源） |
| GET | `/providers` | 服务商与模型定义，密钥恒为掩码 |
| POST | `/providers/upsert` | 保存服务商；带清单就整表对账 |
| POST | `/providers/delete` | 删服务商及其全部模型，并清相关指针 |
| POST | `/providers/model/save` | 单条模型新增或修改 |
| POST | `/providers/model/delete` | 删一条模型并清相关指针 |
| POST | `/providers/models` | 按协议向上游探测模型清单 |
| POST | `/providers/test` | 同一条探测逻辑，给「测试连接」用 |
| GET | `/models/defaults` | 读新会话起步用的默认三键 |
| POST | `/models/defaults` | 写默认三键，空串即删键 |

字段级入参出参见 `docs/API.md` §3.5。事件：`models:changed`（增删改与默认值变更后广播，界面重拉下拉）。

**⑤ 错误码**：3000 服务商名为空、地址为空、`Lookup` 找不到服务商、`SaveModel` / `DeleteModel` 的服务商不存在、默认服务商与默认模型只填一半、未知思考档；3001 `resolveModel` 没选模型或指向已删的模型；5003 `Lookup` 找不到该模型、`DeleteModel` 未找到该条。排障顺序：`/providers` 的 `has_key` 与模型数 → `POST /providers/models` 验连通 → 日志 `解析活跃模型失败 code=3001` / `服务商缺密钥 code=3000`。

**⑥ 不变量与坑**

- 配成 `anthropic` / `google` 能「测试连接通过」而一聊就失败：聊天流式只有 `agent/provider_openai.go`，它无条件 POST `<base_url>/chat/completions`。
- `Probe` 失败不报错，而是回「没通过」加一句原因：连通问题是可见结果，不是异常。
- 空串在两条路径上语义相反：保存服务商的密钥空串 = 不动原值，`SaveDefaults` 的空串 = 删键。
- `Defaults` 回带的设置文件路径是台账位置，用来告诉用户这几个键存在哪儿。

**⑦ 取舍**：换来一张表两类读法（`Lookup` 要「这个模型长什么样」、`ProviderOf` 要「上游在哪」，各查各的）与整表对账的可预期结果；代价是协议字段不校验，界面存得下永远聊不通的组合，真连通只能靠探测与一次实际发送。

## 5. 技能

**① 定位**：`/skills`（`views/SkillsView.vue`）——启用 / 停用 / 项目三区列表 + 推荐清单 + 抽屉编辑。

**② 设计与理由**

- 四个目录：启用区 `data\skills`、停用区 `data\skills_disabled`、项目区 `<工作区>\.pi\skills`（只读）、随包原件 `<assets>\skills`（`builtinSource()`）。
- 开关 = 搬目录，不是改配置：目录搬移对所有情况成立，且不需要任何配合。搬完再写 `skill_toggles` 台账（`repo/store.go SetSkillEnabled`）记一次意图；写失败只 Warn、不回滚——真相在目录里，台账只是备注。
- 动作对象一律是 `SKILL.md` 所在的目录，入参必须是 `SKILL.md` 路径：`Toggle` 传别的回 1000、`Delete` 传目录路径回 8002。深层任意文件算出的目录会把别的技能搬走，而传技能目录正好能清空整个内置技能区。
- 三区扫描 `skills_scan.go`：`dedupeByDirName` 同名只留启用区那份（启用区没有才留禁用区那份），按 `skill_group` 字母序稳定排序；`skill_modified` 由 `isModified` 与随包原件比对得出；项目技能 `skill_editable=false`。
- 启用技能的索引进系统提示：`SKILL.md` 的正文动辄几千字，整段塞进每轮提示等于拿无关长文挤掉上下文，所以只作索引——`PromptSkills` 扫启用区交出 `名字 / 一句话说明 / SKILL.md 绝对路径`，`SystemPrompt` 列成「## 可用技能」并写明「相关才用 read 读正文，不相关不要读」。清单空时整节不出现；超过 40 条按清单顺序截断（`maxPromptSkills`），省下的上下文比多列 3 条更值钱。
- 索引每轮开跑前整段重取（`Chat.refreshSkills` → `Registry.SetSkills`），不在六个写接口里各推一次：漏推一个就长期不一致，而重扫一次启用区只是读几个小文件。开关因此不需要重启。
- 技能正文对工具是只读的：`read` 能读到（`Registry.resolve` 的越界守卫对「当前启用的技能文件」放行），`write` / `edit` 会被 `Run` 拒掉，改内容只走技能页——技能有原件、副本、台账三份状态，工具写哪一份都说不清。
- 四个入口的约束：`Install` 只认推荐清单（`skill_catalog.go` 固定 10 条）的目录名，目标已存在直接拒；`Import` 是复制不是引用（`copyTreeAtomic`），源目录之后删改都不影响副本；`Create` 的名字过 `sanitizeSkillName`（字母 / 数字保留，其它字符换成 `-`，首尾 `-` 去掉）；`Reset` 先复制到 `<目录>.resetting` 再换名，并回写原来的分区（`wasEnabled`）。

**③ 实现链路**：`api/routes_data.go`（`/skills*`）→ `service/skills.go`（`Install` / `Toggle` / `Reset` / `Import` / `Create` / `Delete` / `guardPath` / `builtinSource` / `projectDir` / `Count` / `sanitizeSkillName`）→ `skills_scan.go`（`List` / `scanDir` / `dedupeByDirName` / `sourceOf` / `isModified` / `skillDescOf` / `PromptSkills`）、`skills_edit.go`（`Content` / `Save`）、`skill_catalog.go`（`RecommendedSkillOf`）、`skill_fs.go`（`copyTree` / `copyTreeAtomic`）→ `pkg/frontmatter.go` 解析 YAML 头。进提示这一段：`app.go` 把 `skills.PromptSkills` 接给 `Chat.SkillSource` → 每轮 `Chat.refreshSkills` → `agent.Registry.SetSkills` → `agent.SystemPrompt` 的「## 可用技能」。前端 `views/SkillsView.vue`（`AbortController` + `loadSeq`、重置 / 删除二次确认）→ `components/skills/{SkillRow,SkillDrawer,SkillCreateDialog}.vue` → `stores/refresh.ts bumpSkills`。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/skills` | 三区合并清单 + 推荐位统计与未装清单 |
| POST | `/skills/install` | 安装一条推荐技能 |
| GET | `/skills/content` | 读某份 `SKILL.md` 原文 |
| POST | `/skills/save` | 写回 `SKILL.md` |
| POST | `/skills/toggle` | 启用 / 停用（在两个区之间搬目录） |
| POST | `/skills/create` | 新建技能目录与 `SKILL.md` |
| POST | `/skills/import` | 从外部目录复制导入为托管技能 |
| POST | `/skills/delete` | 删除整个技能目录 |
| POST | `/skills/reset` | 把内置技能重置回随包原件 |

字段级入参出参见 `docs/API.md` §3.6。事件：`skill:status`（`Install` / `Create` 只带目录名，其余再带一个动作字段）；前端 `bumpSkills` 让技能页重拉。

**⑤ 错误码**：8000 路径越界（不在三个托管根内）、`SKILL.md` 读失败、名字清洗后为空、技能已存在、不认识的推荐技能、所选目录无 `SKILL.md`、非内置技能不能重置；8001 目录不可写、搬移 / 复制 / 保存失败、开关项目技能；8002 删除失败（含传目录路径、删项目技能）；1000 `skill_path` 不指向 `SKILL.md`。

**⑥ 不变量与坑**

- 列表里有某个技能不等于它启用了：`dedupeByDirName` 之后禁用区同名目录仍会占一行，只是 `skill_enabled=false`。核对以 `data\skills\<目录>\SKILL.md` 是否存在为准，`skill_toggles` 只解释「用户为什么关过它」。
- `skill_modified` 为真表示内容与随包原件不同，是 `Reset` 唯一的提示信号。
- 项目技能属于那个仓库：壳里既不能开关也不能删除，只能读。
- `Reset` 中途失败宁可保内容不回保状态：换名失败时把暂存目录留在原地并 `pkg.Warn`。
- 提示里的技能索引是「本轮开跑那一刻」的快照：运行中途开关技能只影响下一轮。这不是懒，是刻意——同一轮里提示与工具列表必须是同一份现实，中途换会让模型按新清单做事而工具侧还没跟上。
- 项目技能（`<工作区>\.pi\skills`）不进提示：引擎只认托管启用区，那条链路只服务技能页展示。要让它生效得先导入成托管技能。

**⑦ 取舍**：换来启用 / 停用语义与引擎无关、技能可整体拷走备份、损坏可重置回原件；代价是开关是文件系统动作（被占用时会失败并报 8001），且台账与目录可能不一致，判定必须以目录为准。

## 6. MCP

**① 定位**：`/mcp`（`views/McpView.vue`）——用户级 + 项目级合并清单、弹窗编辑、开关、测试连接、删除。

**② 设计与理由**

- 真相是两份 JSON：用户级 `data\mcp.json`、项目级 `<工作区>\.pi\mcp.json`；同名时项目级覆盖用户级；`enabled` 缺省视为启用（判据 `entry["enabled"] != false`）。
- 引擎侧装载只认条目里的 `command`（stdio）：`mcpServerDefOf` 没有 `command` 就直接跳过。只配 `url` 的条目能保存、能被 `Probe` 探到工具数，但不会变成模型可调的工具。
- 行协议在 `agent/mcp_client.go`：`initialize` → `notifications/initialized` → `tools/list` → `tools/call`，按 LF 切帧；单请求预算 30s（`mcpRequestBudget`）、握手 60s（`mcpHandshakeBudget`），条目 `timeout`（秒）覆盖请求预算。工具名 `McpToolName` = `mcp_{server}_{tool}`，入参 schema 原样透传（`agent/tool_mcp.go`）。
- `Save` 先整份继承旧条目、再覆盖表单字段：手配的 `headers` 一类字段不该因为界面上没这栏就丢。服务名只允许字母、数字、`_`、`-`——它是拼工具名的原料。
- `notifyChanged` 广播 `mcp:changed` 并 `go Runtime.Rebuild()`：写盘立刻返回，拉进程不挡界面。`rebuildMu` 是容量 1 的 channel，串行化重建避免半新半旧；单个服务装载失败只 Warn 跳过，其余服务照常可用。

**③ 实现链路**：`api/routes_data.go`（`/mcp*`）→ `service/mcp.go`（`List` / `Save` / `Toggle` / `Remove` / `Count` / `notifyChanged` / `findEntry` / `projectPath` / `readMcpFile` / `writeMcpFile` / `toServerRow` / `envLinesOf` / `parseEnvList`）→ `mcp_probe.go`（`Probe`、`mcpProbeTimeout = 10s`、`probeHttp`、`probeStdio`、`mcpHttpPost`、`mcpTimeoutOf`、`mcpInitializeParams`）→ `mcp_runtime.go`（`Rebuild` / `enabledEntries` / `mcpServerDefOf` / `reset`）→ `agent/mcp_client.go`（`DialMcp` / `handshake` / `ListTools` / `Call` / `Close` / `StderrText` / `Exited`）+ `agent/tool_mcp.go`（`LoadMcpTools` / `mcpTool`）。前端 `views/McpView.vue`（`loadSeq` + `AbortController`、删除二次确认）→ `components/mcp/{McpTable,McpDialog}.vue`。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/mcp` | 两级配置合并后的服务清单（含读取失败原因） |
| POST | `/mcp/save` | 新增或修改一个服务条目（可选用户级 / 项目级） |
| POST | `/mcp/toggle` | 开关一个服务并重建工具段 |
| POST | `/mcp/probe` | 测一个服务能否连通、能看到几个工具 |
| POST | `/mcp/remove` | 删除一个服务条目并重建工具段 |

字段级入参出参见 `docs/API.md` §3.8。事件：`mcp:changed`（前端没有分支）。

**⑤ 错误码**：9100 `mcp.json` 读取 / 解析失败（原因进 `read_errors`，坏文件不影响另一侧）；9101 写入失败、服务名为空或非法、`url` 型配了命令行、`command` 型命令行也为空、两者都空、project 作用域未选工作区、条目不存在（含 `Probe` 找不到该服务）；9102 `tools/call` 被拒、应答带 `isError`、缺 `result` 字段、请求编码失败；9103 服务进程没起来或已退出（stderr 进 `detail`）；9104 预算内没有应答。

**⑥ 不变量与坑**

- 「测试通过但模型不会用」的两种原因：该条目是 `url` 型，或 `enabled=false`；`Probe` 对停用条目直接回不通过并说明「先启用再测」。
- 装载结果只看 `command`：界面显示的 `transport_kind` 是派生展示字段，不参与判定。
- 计划模式下 MCP 工具按写工具拦（未登记进只读白名单）。
- 三套预算各管各的：装载握手 60s、请求 30s、探测 10s，条目 `timeout` 覆盖后两者；npx 冷启动慢时先怀疑预算而不是配置。
- 排查看日志 `MCP 服务 %s 已装载 %d 个工具` 与 `MCP 服务 %s 装载失败：%s（详情：%s）`。
- 未实现：OAuth、SSE / HTTP 传输接入引擎、`resources` 与 `prompts`。

**⑦ 取舍**：换来「文件即配置」——两份 JSON 可手改、项目级可随仓库分发；代价是只支持 stdio 装载（HTTP 只能测不能调），真相分散在两个文件加进程内注册表三处，改一处要重建另外两处。

## 7. 工作区 · 文件

**① 定位**：`/workspace`（`views/WorkspaceView.vue`）左 / 中栏——文件树、面包屑、文本编辑、二进制预览；对话页右侧工作台复用同一套读取接口。

**② 设计与理由**

- 路径先过 `Files.guard` 归一：相对路径一律按**工作区根**拼（绝不拿进程 CWD 解析），`Join` 自带 `Clean` 把 `src/../../etc` 折成能被 `Inside` 拒掉的形状，越界直接 1002。所有接受外部路径的接口都必须先过它。
- 守卫判两次：字符串形状（`Inside`）与真实位置（`InsideResolved` → `pkg.RealPath`，经 `GetFinalPathNameByHandleW` 解开符号链接与 junction）。工作区里放一个指向系统目录的链接不能成为第二条出口；尚未存在的新文件也按最长存在祖先的真实路径判定。
- 上限按用途分开：列目录 500 条、搜索 200 条命中、读 1MB（超限 `truncated`）、写 5MB、可预览二进制 32MB（再大不回 base64）。
- `Search` 只按**文件名**包含匹配，命中到量就 `SkipAll`，并跳过 `node_modules` / `.git` / `dist` / `build` / `.next` / `target` / `__pycache__` / `.venv`（`isNoiseDir`）；权限不足的目录 `SkipDir` 而不是整体失败。
- `Read` 用 NUL 字节或非 UTF-8 判二进制，再按扩展名分渲染类（`previewKind`：`pdf` / `sheet`(`.xlsx|.xls|.xlsm`) / `doc`(`.docx`) / `image`(`.png .jpg .jpeg .gif .webp .bmp .svg`)），命中且体积 ≤ 32MB 才回 base64。

**③ 实现链路**：`api/routes_files.go` → `service/files.go`（`guard` / `List` / `Search` / `Read` / `Save` / `Mkdir` / `Delete` / `Rename` / `isNoiseDir` / `isBinary` / `previewKind` / `readAtMost`）。前端 `views/WorkspaceView.vue`（`WbSkeleton` 占位、watch `refresh.workspace`）→ `components/workspace/{FilePanel,FilePreview,DiffView,WorkspaceHead}.vue` + `components/common/DiffRows.vue`；对话页一侧是 `components/chat/WorkPanel.vue` → `components/workspace/FileInspect.vue`（见 §1）。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/files` | 列一个目录（目录优先，有条数上限） |
| GET | `/files/search` | 按文件名搜索工作区 |
| GET | `/files/read` | 读文本文件，或给出可预览的二进制 |
| POST | `/files/save` | 写文本文件（顺带建父目录） |
| POST | `/files/mkdir` | 建目录 |
| POST | `/files/delete` | 递归删除（工作区根拒删） |
| POST | `/files/rename` | 改名或移动 |

字段级入参出参见 `docs/API.md` §3.9。事件：无专属事件；`chat:done` 后 `bumpWorkspace` 让工作区页重看一眼。

**⑤ 错误码**：5000「请先选择工作区」（复用会话段码，不是会话问题）；1001 不存在（含「这是一个目录」）；1002 路径不在工作区内；1003 读失败、父目录创建失败、写失败、超 5MB、`mkdir` / `rename` 目标已存在、搜索遍历失败。

**⑥ 不变量与坑**

- 越界是 1002 而不是 1001：一个是「不让看」，一个是「没有这东西」，混起来会把权限问题查成路径问题。
- `GET` 路由不写盘（AGENTS §2.8）：`/files/read` 什么都不落，写动作只在自己的 POST 路由里。
- `Save` 会顺手建父目录，但同名目录不能当文件写（1003）。
- 预览空白的排查顺序：是否二进制 → 渲染类型 → 是否给了 base64 → 是否 > 32MB；文本没渲染先看截断标志。

**⑦ 取舍**：换来一套 `guard` 就封住整个文件面（界面与工具同一判据）、上限明确；代价是搜索只看文件名（要内容得走 §9 索引或 `grep` 工具），超限一律静默截断、由截断标志承担提示责任。

## 8. Git

**① 定位**：`/workspace` 右栏——`GitBar`（分支 / 推拉 / 远端）、`GitChangesPane`、`GitCommitPanel`、`HistoryPanel`、冲突解决与 stash。

**② 设计与理由**

- 全部命令走 `pkg/proc`（隐藏窗口、超时必杀、argv 直传不进 shell），默认目录是当前工作区、超时 120s（`runIn`）。
- 子进程环境只追加 `GIT_TERMINAL_PROMPT=0`：凭证问询立即失败而不是卡满 120s——图形程序里永远等不到答案；凭证本身交给系统 credential manager。
- 写命令独占 `writeMu`（`runWriteOrFail`）：并发写会撞 `index.lock`，得到的是一句看不出原因的失败。
- `Status` 用 `--porcelain=v1 -z`：`-z` 让路径不被 `quotepath` 转义，中文文件名才不会被读成转义串；`fillChangedFiles` 带 `--untracked-files=all`，`fillRepoState` 从 `.git` 下的标记文件判断是否处于 merge / rebase。
- diff 上限 4000 行（`maxDiffLines`），被截掉的条数记在 `dropped`；失败时 stderr 进 `detail`，凭证类翻成 1102 + 指引（`gitAuthHint`）。
- 强制推送只接受 `--force-with-lease`，且要求界面把分支名原样回敲一遍（`confirm`）：`--force` 会把远端未拉到的提交静默抹掉。

**③ 实现链路**：`api/routes_git.go mountGit` → `service/git.go`（`Status` / `fillBranch` / `fillRemotes` / `fillChangedFiles` / `fillLineCounts` / `fillRepoState` / `Diff` / `cutDiff` / `parseDiff` / `runIn` / `run` / `runOrFail` / `runWriteOrFail` / `gitFail` / `gitAuthHint` / `gitEnv` / `ProbeOf` / `gitVersion`）、`git_write.go`（`Stage` / `Unstage` / `Discard` / `Resolve` / `ResolveConflict` / `Commit` / `CreateBranch` / `SwitchBranch` / `DeleteBranch` / `Merge` / `State` / `Init` / `AddRemote` / `validateBranchName`）、`git_history.go`（`History` / `CommitDetail` / `FileHistory` / `FileDiff` / `Branches` / `parseDiffShared`）、`git_sync.go`（`Push` / `Pull` / `Fetch`）、`git_stash.go`（`StashPush` / `StashPop` / `StashList` / `StashDrop`）。前端 `views/WorkspaceView.vue` → `components/workspace/{GitBar,GitChangesPane,GitCommitPanel,HistoryPanel,GitFileRows}.vue` → `composables/useGitActions.ts`（危险操作 `ElMessageBox` 二次确认，文案写明后果）。

**④ 接口与事件**（写接口一律回最新状态）

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/git/status` | 工作区快照：分支、领先落后、改动文件、仓库状态、远端 |
| GET | `/git/diff` | 某文件相对工作区的差异 |
| GET | `/git/history` | 提交历史 |
| GET | `/git/commit` | 单条提交的详情与改动清单 |
| GET | `/git/file-history` | 某个文件的提交历史 |
| GET | `/git/file-diff` | 某次提交里某文件的差异 |
| GET | `/git/branches` | 分支清单及各自领先落后 |
| GET | `/git/stash/list` | stash 条目清单 |
| POST | `/git/stage` | 加入暂存 |
| POST | `/git/unstage` | 移出暂存 |
| POST | `/git/discard` | 丢弃工作区改动（未跟踪文件直接删） |
| POST | `/git/resolve` | 把冲突文件标记为已解决 |
| POST | `/git/conflict/resolve` | 按 ours / theirs 取一侧后再标记 |
| POST | `/git/commit` | 提交，可 amend |
| POST | `/git/branch` | 新建分支 |
| POST | `/git/branch/delete` | 删除分支 |
| POST | `/git/switch` | 切换分支 |
| POST | `/git/merge` | 合并或变基 |
| POST | `/git/state` | 中止 / 继续进行中的 merge、rebase |
| POST | `/git/init` | 初始化仓库 |
| POST | `/git/remote` | 增加远端 |
| POST | `/git/push` | 推送，可控 force-with-lease 与 upstream |
| POST | `/git/pull` | 拉取，可 rebase |
| POST | `/git/fetch` | 抓取远端 |
| POST | `/git/stash/push` | 把当前改动压进 stash |
| POST | `/git/stash/pop` | 弹出最近一条 stash |
| POST | `/git/stash/drop` | 丢弃指定 stash 条目 |

字段级入参出参见 `docs/API.md` §3.10。事件：无 git 事件；`chat:done` 后 `bumpWorkspace` 提示重拉。

**⑤ 错误码**：1100 找不到 `git`（`pkg.LookPath` 返回空）；1101 命令失败（`detail` 是 stderr，配合 `repo_state` 看是否处于 merge / rebase）；1102 凭证被拒或无法输入（用 credential manager 登录一次，或换成带 token 的远端地址）；1000 提交信息为空、分支名为空或非法、强制推送未回敲分支名、stash 丢弃未回敲引用名；5000 未选工作区。

**⑥ 不变量与坑**

- 界面按钮不要直连 `git`：独占 `writeMu` 的是 `runWriteOrFail`，绕开它就回到「抢不到 index.lock」。
- `git` 缺失时 `Status` 不报错，而是回一份带不可用原因的快照，界面才有话可说。
- `ProbeOf` 是给工作区行用的轻量判定（是否仓库、分支、脏文件数），不要拿 `Status` 批量扫目录。
- 危险动作的确认词是「原样回敲」而不是勾选框：推送比对分支名，丢弃 stash 比对引用名。

**⑦ 取舍**：换来真实 git 语义（不用库、不猜状态，一切以 `git` 输出为准）与写操作串行化；代价是每次刷新都要起子进程（所以有 `ProbeOf`），错误可读性依赖 stderr 原文，界面只能贴不能翻译。

## 9. 工作区预索引（索引页 / 代码检索）

**① 定位**：`/index`（`views/IndexView.vue`）——状态卡、检索框、重建；同一套检索也交给模型（`index_search` 工具）。

**② 设计与理由**

- 检索全程读内存：`Searcher` 持当前工作区的 `docs` 与 `Frecency`；`search_doc` 表只在重建时整表替换（`ReplaceWorkspace`）、命中时累加次数（`BumpHit`），`Count` 给状态卡看行数——`SearchRepo.Search` 没有调用方。派生数据可以整表重建，就不必让磁盘参与热路径。
- 索引是派生物：重建失败只影响检索质量，所以用 `building` 标志去重、并发触发直接返回当前状态。
- `Build` 单遍历收文件与符号：跳过依赖目录与二进制，单文件 ≤ 512KB（`MaxIndexedFileBytes`）、每文件正文 ≤ 8000 字符（`MaxBodyChars`）、单次 ≤ 20000 文档（`MaxDocs`）；符号由一组正则抽出（`extractSymbols`），每个符号一条文档 + 每个路径一条文档。
- 打分分两半：`matchScore` 六档（符号名完全匹配 100 / 符号名包含 60 / 文件名完全匹配 50 / 文件名前缀 40 / 路径包含 30 / 正文包含 10），再叠加 `Frecency.Score = (1+ln 次数) × (1 + 1/(1+天数/7))`——文本相关度决定「像不像」，frecency 决定「该不该优先」。同分按路径排，保证顺序稳定。
- `Search` 开头 `EnsureIndexed`：当前工作区没有索引就现建一次（惰性重建），界面不必先点「重建」；`limit <= 0` 兜底 20。

**③ 实现链路**：`api/routes_index.go` → `service/searcher.go`（`Status` / `Rebuild` / `EnsureIndexed` / `Search` / `Kit` / `docToDO`）→ `index/index.go`（`Build` / `isIndexable` / `isBinary` / `truncateBody` / `extractSymbols` / `lineOf` / `docID` / `ModifiedAt`）、`index/search.go`（`Search` / `matchScore` / `Frecency.Record` / `Score`）→ `repo/provider_repo.go SearchRepo`（`ReplaceWorkspace` / `BumpHit` / `Count` / `Clear` / `Search`）。模型侧经 `agent/tool_index.go` 的 `index_search`，出口是 `Searcher.Kit()`。前端 `views/IndexView.vue`（`searchSeq` 竞态守卫，`index_built` 为假时提示先建索引）→ `api.indexStatus()` / `api.indexRebuild()` / `api.indexSearch(term,30)`。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/index/status` | 当前工作区的索引状态与文档数 |
| POST | `/index/search` | 检索工作区（缺索引时先惰性重建） |
| POST | `/index/rebuild` | 为当前工作区整表重建索引 |

字段级入参出参见 `docs/API.md` §3.11。事件：无；重建是同步请求（界面要立刻看到新行数）。

**⑤ 错误码**：5000 未选工作区（`Rebuild` 与 `Search` 入口判定）；1003 `index.Build` 失败（成功有日志 `索引完成：工作区=%s 文档=%d`）。

**⑥ 不变量与坑**

- 索引与 frecency 都是进程内内存态：重启后第一次搜索触发惰性重建，命中次数同时归零，排序必然与重启前不同。
- 命中口径只有「包含」：文件名 / 符号名 / 正文子串，没有分词与模糊；搜不到先确认命中串在这些范围内。
- 上限截断是静默的：超 512KB 的文件、超 20000 条文档不进索引，文档数少于文件数属正常。
- 换工作区必须重建：`Searcher` 只保留一份 `docs`，工作区与 `docs` 不匹配时结果无意义。

**⑦ 取舍**：换来毫秒级检索、无外部依赖、可整表丢弃；代价是重启即失忆、大仓库受上限约束、无增量索引（改了文件要整表重建）。

## 10. 联网

**① 定位**：`/web`（`views/WebView.vue`）——搜索服务配置、搜索、抓取；模型侧对应 `web_search` / `web_fetch`。

**② 设计与理由**

- 适配器按设置现造：`Kit()` 每次调用都重建（`config()` 读 `web.search_kind` / `web.search_key`），改服务不必重启；缺省服务是免密钥的 `duckduckgo`，缺省单次 8 条。
- SSRF 两道门：`ValidateURL` 做字面校验（协议、主机名、`IsPrivateHost` 对字面 IP 判段），`safeDial` 在真正连接前用解析结果复核——域名可以指向 `127.0.0.1`，只看字符串拦不住。拦环回、私有段、链路本地、未指定地址与 CGNAT `100.64.0.0/10`。
- 重定向逐跳校验（`MaxRedirects = 5`），跳进内网同样 1002。
- `Fetch` 按 Content-Type 分流：PDF 抽文字（抽不出 → 5003「可能是扫描件」）、JSON 原样、HTML / 文本走可读正文抽取（`extract.go`），抽不出正文给原文；gzip 由 `inflate.go` 处理。
- 装配时无条件 `AddWebTools`：可用性取决于上游配置而非装配条件，模型看到的工具集因此不随设置变化。

**③ 实现链路**：`api/routes_web.go` → `service/web.go`（`Status` / `Search` / `Fetch` / `SearchRESP` / `FetchRESP` / `SaveSettings` / `SettingsRESP` / `Kit` / `config` / `reload`）→ `web/search.go`（`NewSearchProvider` 与 `duckduckgo` / `brave` / `exa` / `tavily` 四个适配器、`SearchResult`）、`web/search_parse.go`（DDG 解析与跳转还原）、`web/client.go`（`Client` / `ValidateURL` / `IsPrivateHost` / `isPrivateIP` / `safeDial` / `MaxRedirects`）、`web/extract.go`、`web/pdf.go`、`web/inflate.go`；模型侧在 `agent/tool_web.go`。前端 `views/WebView.vue`（`searchSeq` / `fetchSeq` 守卫；只有需要密钥的服务才显示密钥框，留空 = 保留原值）。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/web/status` | 当前搜索服务、是否已配密钥、可选服务种类 |
| POST | `/web/settings` | 保存搜索服务与密钥 |
| POST | `/web/search` | 用当前服务搜一次 |
| POST | `/web/fetch` | 抓取一个地址并抽成正文 |

字段级入参出参见 `docs/API.md` §3.12。事件：无；保存设置不广播，下次调用时按新配置重建适配器。

**⑤ 错误码**：1000 不认识的搜索服务、地址为空 / 非法 / 非 http(s) / 无主机名、请求体编码失败；1002 内网或重定向跳到内网；5003 PDF 没抽出文字；7000 抓取失败、上游非 200、重定向过多、重定向地址不合法、读响应失败。

**⑥ 不变量与坑**

- 越界类失败是 1002 而不是 1000：一句「地址不合法」会把内网限制查成拼写问题。
- 搜索密钥只在写接口出现，读接口只回「有没有配」。
- DDG 结果页结构变了会让解析结果为空而不是报错；零结果先换 `brave` / `tavily` 并确认已配密钥。
- 上游非 200 归 7000（外部动作失败），不要按业务码去改设置校验。

**⑦ 取舍**：换来免配置可用（默认 DDG）、四种服务同一接口、SSRF 有最终闸门；代价是 DDG 解析依赖对方面向浏览器的页面结构（脆），密钥与其它本地设置同样明文存在台账里。

## 11. 用量统计

**① 定位**：`/usage`（`views/UsageView.vue`）——时间窗合计、按天趋势、按模型占比；概览页另有一份图表组件。

**② 设计与理由**

- 没有独立账本，也没有独立聚合接口：全部数字来自扫描会话 JSONL 得到的会话行（token、成本、消息数、模型、更新时间），时间窗、按天、按模型的分桶都在前端内存里做。少一套数据就少一处会不一致。
- 两个页面范围不同是刻意的：用量页不带参数（全部目录，看总花费），概览页带当前工作区（看手头项目）。

**③ 实现链路**：`parser/summary.go Summarize` 逐条累加 `MessageUsage` / `EntryUsage`，`updated_at` 取头时间与条目时间的最大值 → `service/sessions.go List` 装成会话行 → `GET /sessions`。前端 `views/UsageView.vue`（`span` 按 `updated_at` 落窗，`byDay` / `byModel` 分桶，自己 `echarts.init()` 建两个图实例、watch `ui.theme` 重绘）；概览页 `views/OverviewView.vue` → `components/overview/UsageCharts.vue`（同样内嵌 echarts，数据由父层传入）。

**④ 接口与事件**：无专属路由，读 §3 的 `GET /sessions`（字段级出参见 `docs/API.md` §3.3）；无事件，`chat:done → refreshSessions()` 间接更新。

**⑤ 错误码**：无专属码；聚合读不到数据回空数组而不是报错。

**⑥ 不变量与坑**

- `message_count` 是消息条数（用户 + 助手 + 工具结果各一条），不是对话轮数。
- 「数字为 0」的两种原因：会话里没有 usage（上游未回用量，如部分自建网关），或会话不在当前范围。
- 「合计与列表不符」的两种原因：两页范围不同，或本页时间窗只算了窗口内的会话。
- SQLite 只有 `provider` / `provider_model` / `search_doc` 三张表，用量没有任何落地的中间态。

**⑦ 取舍**：换来零额外状态、改口径只改一处解析；代价是每次进页面都要扫全部会话文件，且无法回答跨窗口的历史查询。

## 12. 设置 · 偏好 · 工作区切换

**① 定位**：`/settings`（`views/SettingsView.vue`）——外观、行为、引擎、工作区、配置原文、模板、诊断、关于；工作区切换三处入口：设置页、对话页输入框底栏的 `WorkspaceChip`、托盘菜单。

**② 设计与理由**

- 设置台账是一份 JSON（`data\workbaby.json`），`repo/store.go` 原子整写 + 互斥：单机十来个键，不值得为它开一张表。键名集中在 `service/settings.go` 的常量里——散在调用处迟早会出现「设置页写了 A，聊天读的是 B」。
- 界面类设置首启补默认（`EnsureDefaults` 只补缺键，用户改过的一个字不动），界面读到的是明确的值，不必各自兜底。
- 入口收敛而不报错：`normalize` 把 `ui.mention_sessions` 夹到 1~20，读侧 `MentionSessions()` 再兜一次（空 / 非法 / < 1 → 5，> 20 → 20）。手改出来的坏值按边界收敛比抛错好用。
- `Save` 拒收保留键（`reservedSettings`）：`workspace.current` / `workspace.history` 只能经 `/settings/workspace` 写。工作区是文件接口的根，绕过它就等于把 1002 路径守卫连根拔掉——拒而不静默丢弃，界面才会显示「没改上」而不是以为改上了。
- `All()` 在出口把密钥类键换成掩码（`web.search_key`）：读接口是界面唯一的设置来源，原样回显等于把密钥交给渲染进程；存储里仍是原文，掩码只发生在出口。
- 工作区历史有容量（`maxWorkspaces = 24`，同路径去重、最新在前）：工作区不是标签页，留太多就变成一份需要搜索的长名单。`Workspaces()` 一次扫描把「在不在、是不是仓库、分支、脏文件数、会话数」全算齐，避免每行各发一次 git。
- 关窗行为以设置为准（`prefs.close_action`），且第一次收托盘必须问一次（`prefs.tray_hint_shown` + `MarkTrayHintShown` / `TrayHintPending`）。

**③ 实现链路**：`api/routes_core.go`（`/bootstrap` `/settings*` `/workspaces` `/prefs*` `/logs`）→ `service/settings.go`（`EnsureDefaults` / `All` / `Get` / `Save` / `normalize` / `SaveSettings` / `MentionSessions` / `Workspace` / `SetWorkspace` / `ForgetWorkspace` / `Workspaces` / `WorkspaceList` / `sessionCountOf` / `Prefs` / `SetRestoreSession` / `SetCloseAction` / `CloseAction` / `MarkTrayHintShown` / `TrayHintPending` / `RememberSession` / `LastSession`）→ `repo/store.go`；开机自启 `service/autostart.go` → `service/platform/autostart_windows.go`（HKCU `Run` 项）。键：`workspace.current`、`workspace.history`、`prefs.last_session_file`、`prefs.restore_session`、`prefs.close_action(tray|quit)`、`prefs.tray_hint_shown`、`chat.hide:<会话文件>`、`model.active_provider`、`model.active_model`、`model.thinking_level`、`model.auto_compaction`、`model.default_provider`、`model.default_model`、`model.default_thinking_level`、`web.search_kind`、`web.search_key`、`ui.mention_sessions`、`ui.font_scale`、`ui.reduce_motion`、`ui.locale`，另有 `ui.theme`（只由界面读写、无后端默认值，读空按 light 处理）。前端 `views/SettingsView.vue` → `components/settings/{Appearance,Behavior,Engine,Workspace,Config,Templates,Diag,About}Panel.vue` → `stores/app.ts`（`saveSettings` 即时应用主题 / 字号 / 语言 / 动效；`setWorkspace` 后 `chat.reset()` 并重开会话）。「启动恢复上次会话」由前端 `restoreLastSession()` 执行（读 prefs → `chat.refresh` → 路由跳转），不是壳的行为。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/bootstrap` | 首屏一次性装配：版本、契约、引擎、目录布局、工作区、设置、偏好 |
| GET | `/settings` | 读全量设置键值（密钥类键为掩码） |
| POST | `/settings/save` | 写设置，界面键在入口夹成合法范围；工作区键直接拒（1000） |
| POST | `/settings/workspace` | 切换工作区并记进历史 |
| GET | `/workspaces` | 工作区历史，每行带仓库与会话概况 |
| POST | `/settings/workspace/forget` | 从历史里去掉一个目录（不删磁盘） |
| GET | `/prefs` | 偏好快照，含数据目录与日志目录 |
| POST | `/prefs/autostart` | 开关开机自启（写注册表） |
| POST | `/prefs/restore` | 开关「启动时恢复上次会话」 |
| POST | `/prefs/close-action` | 设关窗行为：收托盘或直接退出 |
| POST | `/prefs/data-dir` | 在资源管理器里打开数据目录 |
| GET | `/logs` | 读日志尾部若干行 |

字段级入参出参见 `docs/API.md` §3.1。事件：`workspace:changed`（前端没有分支，发起方自己更新）；`models:changed`（保存默认模型后）。

**⑤ 错误码**：1000 工作区路径为空、往 `/settings/save` 写了工作区保留键；1001 工作区不存在或不是目录；1003 数据目录不可写；2002 台账 / SQLite 读写失败。现象：「切了工作区会话还在旧目录」→ 看 `Chat.OnWorkspaceChanged` 与前端 `chat.refresh('')`；「关窗直接退出」→ `prefs.close_action=quit`；「关窗没提示」→ `prefs.tray_hint_shown` 已置位（只问一次）。

**⑥ 不变量与坑**

- `Settings.SaveSettings` 是内部通道，不过 `normalize` 也不查保留键；只有 `/settings/save` 走 `Save`。业务代码写自己的键不会被入口夹值。
- 工作区变更只有一条路（`SetWorkspace`）：校验目录、写历史、广播 `workspace:changed`。绕过它写键，界面两页会各说一套。
- 开机自启的真实状态来自注册表（`autoStartEnabled()`），台账里的记录只用于展示：手删注册表项，界面回读会说没开。
- `ForgetWorkspace` 只从历史里去掉，不碰磁盘；`Workspaces()` 对已不存在的目录给「存在性为假」而不是删条目。

**⑦ 取舍**：换来一台机器一份人可读的配置、改完立即生效、无迁移负担；代价是全部设置都是平面键值（无类型校验，靠入口夹值），并发写靠整写加锁，「空串 = 删键」这类语义要调用处各自记住。

## 13. 提示词模板

**① 定位**：设置页「模板」面板管理；对话页斜杠菜单与 Ctrl+K 命令面板选中后把正文填进输入框。

**② 设计与理由**

- 文件即数据：一份模板是 `data\templates\<name>.md`，带 YAML 头（`name` / `description` / `args`，由 `pkg/frontmatter.go` 解析），正文在闭合 `---` 之后。和技能同一套思路——能用编辑器直接改，也能整个目录拷走备份。
- 名字要能当文件名：`sanitizeTemplateName` 只留小写字母 / 数字 / 连字符并去掉首尾 `-`，清洗后为空直接 1000；`description` / `args` 过 `oneLine` 压成一行，否则会把头部结构写坏。
- 坏文件跳过而不是报错：一份手改坏的模板不该拖垮整张清单（`readOne` 返回 false）。
- 与斜杠命令同一入口：`Templates.Commands()` 以 `command_source="prompt"` 并进 `chat_state.go commands()`，`args` 非空时描述拼成 `args · desc`，`command_body` 带正文——界面只认一份命令清单。

**③ 实现链路**：`api/routes_templates.go` → `service/templates.go`（`List` / `readOne` / `Commands` / `Save` / `Delete` / `sanitizeTemplateName` / `templateBody` / `oneLine`）→ `pkg/frontmatter.go`。前端 `components/settings/TemplatesPanel.vue` → `composables/useComposerMenu.ts`（`prompt` 类填正文）→ `components/common/CommandPalette.vue`（Ctrl+K：导航 / 动作 / 最近会话 / 命令）。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/templates` | 列出全部模板（按名字排序，坏文件跳过） |
| POST | `/templates/save` | 新建或覆盖一份模板 |
| POST | `/templates/delete` | 删一份模板（不存在不算错） |

字段级入参出参见 `docs/API.md` §3.7。事件：无；下一次 `chat/state` 的命令表自然带上新模板。

**⑤ 错误码**：1000 名字为空或清洗后为空；1003 目录创建 / 写入 / 删除失败。现象「菜单里没有我的模板」→ 先看 `/templates` 有没有这一行（扩展名必须 `.md`、frontmatter 没写坏、清洗后名字非空），再看命令表里它的来源是否为 `prompt`。

**⑥ 不变量与坑**

- 清单里的名字以 frontmatter 的 `name` 为准，缺失才回落成文件名；而 `save` 写的是「请求名清洗后」的那份文件，两者不同名时删除要用对口径。
- 模板是「填进输入框」而不是直接发送：正文通常带占位参数，需要用户补。

**⑦ 取舍**：换来零迁移、可版本化的提示词库（拷目录即备份）；代价是除清洗与压行之外没有质量校验，且每次 `chat/state` 多一轮目录扫描。

## 14. 系统 · 托盘 · 壳 + 引导流程

**① 定位**：窗口 / 托盘 / 单实例 / 关窗行为 / 外部工具探测，以及「双击 exe 到看见界面」的全部步骤。

**② 设计与理由**

- 装配顺序定死首屏体验：`main.go`（内嵌前端产物 + 版本号）→ `shell.Run` → `resolveLayout` → `pkg.SetLogDir` → 建 Wails 应用（单实例 `com.workbaby.desktop`，第二次启动只唤回首个实例的窗口）→ **隐藏建窗**（装配期显形会让用户看到一帧白屏）→ 先建托盘（用户可能一启动就把窗口关掉）→ `ServiceStartup → assemble`（开台账 store → 开 SQLite → `sse.New(1024)` → 生成令牌 → `service.New` → `api.New.Start`）→ `go Chat.Warm()`。窗口显示由前端首屏完成后的 `Ready()` 负责。
- 建窗尺寸按屏幕缩放换算成物理像素声明，`Ready()` 再用逻辑值设一次并居中：v3 把 `Width/Height` 当物理像素用，200% 缩放下声明 1280 只得到 640 的 CSS 空间；居中不用 `Center()`（它对无边框窗口按客户区算会偏）。
- 令牌是启动时生成的随机串（`WB_DEV_TOKEN` 可覆盖），端口是 127.0.0.1 上的随机端口，前端经 `Endpoint()` 同时取两者；`Endpoint()` 最多等 20s，超时算装配失败——拿到端口 0 会被前端当成永久失败。
- WebView2 缓存放进 `data\webview`：默认落在 `%APPDATA%\<exe 名>`，卸载时没人清理。
- 关窗：`WindowClosing` 钩子按 `prefs.close_action` 分岔。`quit` 走 `requestQuit`（置标志后异步 `Quit`——`TrackPopupMenuEx` 还没返回，在它栈上销毁托盘会让消息循环在已释放的菜单上继续处理命令）；`tray` 先取消关闭，第一次由 `askCloseDecision` 广播 `app:close_requested` 让界面弹选择（窗口正要隐藏，系统通知用户看不见），选完经 `ApplyCloseDecision(action,remember)` 回后端并 `MarkTrayHintShown`。`--hidden` 只上托盘不显窗。
- 引擎在进程内，所以这类动作只剩语义映射：`/engines/restart` = `Chat.Stop()` + 广播 `engine:ready`；`/engines/detect` 与 `/engines/repair` 都是 `App.Redetect`（重新探测 git / node / python）。
- 配置原文白名单只有 `workbaby.json` 与 `mcp.json`（`configFiles` / `ConfigText`）：能看能拷，其余目录不开放。

**③ 实现链路**：`backend/shell/app.go`（`Run` / `resolveLayout` / `windowPhysicalSize` / `ServiceStartup` / `startup` / `assemble` / `Endpoint` / `setStartErr` / `startMessage` / `closeAction` / `hasArg` / `newToken`）、`shell/window.go`（`Ready` / `centerOnPrimaryScreen` / `showWindow` / `toTray` / `askCloseDecision` / `ApplyCloseDecision` / `requestQuit` / `WindowMinimize` / `WindowToggleMaximize` / `WindowClose` / `OpenExternal` / `PickFolder` / `PickFiles` / `shutdown`）、`shell/tray.go`（`buildTrayMenu` / `fillRecentWorkspaces` / `refreshTrayMenu` / `recentWorkspaces` / `shortPath`，`trayRecentMax = 5`）、`shell/endpoint.go`、`shell/windows.go`（DPI 缩放、`fatalDialog`）。前端 `main.ts`（Pinia / Router / i18n / Element Plus）→ `App.vue`（自绘标题栏 + `AppRail` + 主区；`app:close_requested` 弹选择后 `applyCloseDecision`；首屏画完 `notifyReady()`）→ `stores/app.ts load()`（`getEndpoint()` → `api.bootstrap()` → 应用主题 / 字号 / 语言 / 动效 → `connect()` 建 SSE → 重拉会话与引擎 → `restoreLastSession()`）。

**④ 接口与事件**

| 方法 | 路径 | 这一条做什么 |
|---|---|---|
| GET | `/engines` | 引擎身份、目录布局、运行时状态与当前实例 |
| POST | `/engines/detect` | 重新探测外部工具链 |
| POST | `/engines/repair` | 与上一条等价的全面刷新 |
| POST | `/engines/restart` | 中止当前运行（进程内引擎没有进程要重启） |
| GET | `/health/engines` | 概览页的健康快照 |
| GET | `/apps` | git / node / python 的探测行与缺失后果 |
| GET | `/config/files` | 可查看的配置文件白名单 |
| GET | `/config/text` | 读某一份配置原文 |
| POST | `/system/reveal` | 在资源管理器里定位一个路径 |
| POST | `/system/open` | 用系统默认程序打开一个路径 |

字段级入参出参见 `docs/API.md` §3.2。事件：`engine:detected`（重新探测）、`engine:ready`（仅 restart）、`app:close_requested`（首次关窗询问）、`stream:open`（连接首帧）。

**⑤ 错误码**：1000 请求体不合法（`detail` 是解析错原文）；1001 目标路径不存在、数据目录不可写（`resolveLayout` 的 `EnsureDir` 失败）；1003 配置原文读取失败；2002 台账 / SQLite 打开失败；1404 未知路由；9999 未归类内部错误。引导失败顺序：`Endpoint()` 的装配错误（20s 超时也算失败）→ `data\logs` 里的装配日志（`/logs`）→ 数据目录可写性。

**⑥ 不变量与坑**

- `GET /engines` 的实例信息是现造的：`started_at` 是当前时刻（`App.EngineInstance`），不是进程启动时刻。
- `--hidden` 下 `Ready()` 仍会被调用（用于刷新托盘菜单），但窗口不出现；「窗口不出现」先查这两件事。
- 托盘「最近工作目录」最多铺 5 条，条目来自 `Settings.Workspaces()`；一条都没有时铺一条禁用说明项而不是空菜单。
- 装配失败必须 `fatalDialog`：这个阶段的 SSE 与日志界面都不存在，弹框是用户唯一能看到原因的地方。
- 单实例在 `application.New` 内部完成「通知首个实例 → 退出」，第二个进程走不到建窗，也就不会抢端口与令牌。

**⑦ 取舍**：换来双击即用、无控制台也有反馈、窗口出现前一定有一致的端口与令牌；代价是首屏要等同步装配（端口就绪前 `Endpoint()` 会等，最坏 20s），关窗路径分成三条（托盘 / 退出 / 首次询问）都得测。
