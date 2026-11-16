# 真实运行时回归报告（2026-11-16）

> 目的：用户反馈「很多基础功能用不了」，但 `go test ./tests` 全绿。本轮把后端真实跑起来，
> 逐一打真实 HTTP 路由、验证 SSE 链路，并静态核对前后端契约。**只收集问题，不修复。**
>
> 方法：`go build -o %TEMP%\wb-regression\WorkBaby.exe .`（当前 main = 5ebed66），
> 以 `--hidden` + `WB_DEV_TOKEN` 启动（同 `scripts/backend.ps1` 的方式），数据目录
> `%LOCALAPPDATA%\WorkBaby-dev`，监听 `127.0.0.1:50611`。共发出 **188 个真实请求**，
> 全部请求/响应留档于 `%TEMP%\wb-regression\results.json`。
> 写操作全部在临时工作区 `%TEMP%\wb-regression\ws`（含临时 git 仓库与本地裸远端）与
> 新建临时会话/服务商/技能/模板/MCP 上进行；结束后已还原设置台账、删除全部临时数据、
> 杀掉后端进程树。仓库业务代码零改动。

---

## 一、结论总览（三态）

| # | 功能 | 结论 | 一句话 |
|---|---|---|---|
| F1 | 服务商连通性探测 URL 拼接 | **不可用（按界面引导填写时）** | 探测拼 `/v1/models`、聊天拼 `/chat/completions`，前端占位符引导带 `/v1` → 探测必打成 `/v1/v1/models` |
| F2 | 当前会话改名 | **不可用** | 引擎打开的会话一律拒改名（5002），而用户正在看的会话恰好就是被打开的那份 |
| F3 | 当前会话删除 | **不可用（当前会话）** | 同上守卫；先切走才能删，界面按行内按钮直接删当前行必失败 |
| F4 | 斜杠命令 `/export` | **不可用** | 后端清单里宣告了它，但 `HandleCommand` 没有分支 → 当作普通消息发给模型 |
| F5 | `chat:patch` 块类型字段 | **存疑（契约不一致，影响有限）** | 后端发 `kind`，前端读 `block_kind`；流式功能靠 block 载荷兜住，字段本身失配 |
| F6 | 4 个后端 SSE 事件无人接管 | **存疑** | `workspace:changed` / `chat:model` / `chat:plan` / `mcp:changed` 前端只打 console.warn |
| F7 | 计划模式拦 `index_search` | **可用（设计如此，易误报）** | 文档明说是保守登记的后果，非缺陷；但用户会感知「检索用不了」 |
| F8 | `@` 最近会话条数配置键 | **可用（前端读）/ 存疑（后端死代码）** | 键真实读写生效；后端 `MentionSessions()` 无人调用 |
| F9 | 工作区级 MCP/技能目录名 `.pi` | **存疑（品牌残留）** | 与产品数据目录 WorkBaby 不一致，疑为 pi 迁移残留；功能本身自洽 |
| — | 其余全部 HTTP 路由 / SSE 主链路 / 文件 / git / 技能 / 模板 / MCP / 联网 / 索引 | **可用** | 详见下文逐项证据 |

**与「很多基础功能用不了」最相关的是 F1（配了服务商却探测不通）、F2/F3（会话改名删除必报错）、F4（斜杠命令没反应）。**

---

## 二、失败/存疑项详情

### F1. 服务商探测与聊天的 URL 约定互相矛盾（高优先）

- 探测（`/providers/models`、`/providers/test` 同一 handler）：
  `backend/service/model_probe.go:61,74` → `strings.TrimSuffix(baseUrl,"/") + "/v1/models"`。
- 聊天：`backend/agent/provider_openai.go:167` → `TrimSuffix(BaseUrl,"/") + "/chat/completions"`。
- 前端占位符：`frontend/src/components/models/ProviderDialog.vue:136` → `placeholder="https://api.example.com/v1"`。
- 文档口径：`docs/PROVIDERS.md:33`「地址里不要自带 /v1 之后的路径」、`:48` 聊天为 `<base_url>/chat/completions`。

即：**同一个 base_url 字段，探测要求不带 `/v1`，聊天要求带 `/v1`（OpenAI 兼容上游的标准路径是 `/v1/chat/completions`），而界面引导用户带 `/v1`。**

实测证据（假服务商 `regress-prov`，base_url=`http://127.0.0.1:9/v1`）：

```
POST /providers/models → 200 code=0 probe_ok=false
probe_message: 连不上：Get "http://127.0.0.1:9/v1/v1/models": dial tcp ... refused
POST /chat/send      → chat:error 事件
message: 连不上接口: Post "http://127.0.0.1:9/v1/chat/completions": dial tcp ... refused
```

两条路对同一 base_url 拼出不同前缀（`/v1/v1/models` vs `/v1/chat/completions`）。
按占位符填 `https://api.openai.com/v1`：聊天正确、探测 404；按文档填 `https://api.openai.com`：探测正确、聊天 404。
**无论怎么填，「测试连接」与「真正聊天」至少有一条不通** —— 与用户「配好了却用不了」的体感吻合。
（本机 dev 库里遗留的测试服务商 base_url 均带 `/v1`，佐证实际使用习惯是带 `/v1`。）

### F2/F3. 当前会话不能改名、不能删除（高优先）

- 守卫：`backend/service/sessions.go:139-142`（Rename）、`:118-121`（Delete）：
  `s.Active()` 等于目标文件即回 `5002`。`Active` 绑定 `chat.CurrentFile`（`backend/service/app.go:104`）。
- 实测：

```
POST /sessions/rename {session_path: <当前打开会话>, session_title:"回归会话改名"}
→ 200 code=5002 这份会话正被引擎打开，切到其他会话后再改名
POST /sessions/delete {session_path: <当前会话>}
→ 200 code=5002 这份会话正被引擎打开，切到其他会话后再删除
（新建另一会话挪开绑定后，同一文件删除成功 code=0）
```

- 前端在会话列表行内直接调这两个接口（`frontend/src/components/sessions/SessionList.vue:160,189`），
  没有「先切走」的前置动作。**用户正在看的会话＝引擎绑定的会话**，所以对它的改名/删除永远失败。
- 注：守卫动机（防两路追加写坏 JSONL）写在注释里，Rename 走 `session.Rename` 追加入口本可复用引擎锁；
  是否放宽属下一阶段决策，此处只记录现象。

### F4. 斜杠命令 `/export`：清单宣告了、没人消费（高优先）

- 后端清单：`backend/service/chat_state.go:162-168` `nativeCommands()` 返回 `new / compact / export / plan` 四条。
- 消费分支：`backend/service/chat_state.go:188-213` `HandleCommand` 只有 `/new`、`/compact`、`/plan` 三个 case，
  **`/export` 落空** → `Send` 把它当普通消息起一轮运行。
- 实测：`POST /chat/send {message:"/export"}` → `disposition:"started"`（应为 `handled`），
  会话里追加了一条用户消息 `/export`，随后模型调用失败（假上游）。
- 前端侧掩盖了菜单入口：`frontend/src/composables/useComposerMenu.ts:76-78` 把与内置同名的引擎命令过滤掉
  （内置 `export` 是导出 Markdown，`frontend/src/composables/useBuiltinCommands.ts:36`），
  所以菜单里点不到坏的那条；但**手输 `/export` 回车就踩中**。
- 两侧命令清单对照：

| 命令 | 后端清单 | 后端消费 | 前端内置 | 实际效果 |
|---|---|---|---|---|
| /new | 有 | 有（chat_state.go:195） | 有（去重保留前端） | 可用 |
| /compact | 有 | 有（:203） | 有（去重保留前端） | 可用（空会话回 1000「还没有可压缩的对话」，可读） |
| /plan | 有 | 有（:209，on/off/取反） | 无 → 菜单填入文本发送 | 可用（实测 `/plan on`、`/plan off` 均 `handled`） |
| /export | 有 | **无** | 有（语义不同：Markdown） | **手输直发 → 发给模型** |
| /model /thinking /copy /exporthtml /reload /clone | 无 | 无 | 有（前端本地执行） | 可用（界面动作，不经引擎） |

### F5. `chat:patch` 帧的块类型字段失配（中优先，功能影响有限）

- 后端：`backend/service/chat_stream.go:301-302` payload 为 `{"mode","index","kind"}`（外层 emit 补 `run_id`）。
- 前端：`frontend/src/utils/sse.ts:144` `readPatch` 读的是 `data.block_kind` → 永远 undefined → 兜底成 `'text'`。
- 影响评估：`applyPatch`（`frontend/src/composables/useMessageFlow.ts:106-121`）delta 模式只按 index 追加文本、
  block 模式整块替换（块内自带正确的 `block_kind`，sse.ts:110 读的也是 `block_kind`，与
  `backend/domain/chat.go:20` 一致）。首块必为 block 模式（chat_stream.go:96-101），
  所以**流式正文/思考/工具卡目前能画对**；失配的是补丁级 `kind` 字段本身——任何依赖它的逻辑都会拿到错值。
- 本轮无真模型 key，未能实测完整流式渲染；此条为静态契约证据，标**存疑**。
  另：前端类型 `ChatPatch.kind`（api-chat.ts:204-208）与后端 payload 键名不一致这一事实是确定的。

### F6. 后端发出、前端未接管的 SSE 事件（中优先）

后端实际广播（实测捕获 + 代码定位）与前端分派表（`frontend/src/stores/events.ts:71-287`）对照：

| 事件 | 后端发射点 | 前端 | 后果 |
|---|---|---|---|
| `workspace:changed` | service/settings.go:243 | **无 case** | 他处切工作区时本页不刷新（本机 GUI 自己切时 app.ts:194-203 手动刷新，掩盖了缺口） |
| `chat:model` | service/chat_session.go:121 | **无 case** | 会话模型变更不驱动界面（切模型的界面自己更新本地态，掩盖） |
| `chat:plan` | service/plan_mode.go:36 | **无 case** | `/plan` 命令改状态后，计划模式徽标不即时更新（`/plan` 走 handled 后 chat.ts:312 会重拉一次 state，部分掩盖） |
| `mcp:changed` | service/mcp.go:43 | **无 case** | MCP 页不随后台变化刷新 |

以上四条实测均出现在 SSE 流里（seq 4、5-9、19-22、24），前端落到 default 分支只
`console.warn("[wb] 未接管的 SSE 事件")`（events.ts:283）。

反向（前端监听、后端永不发射）的死分支：`engine:status`、`engine:exit`、`engine:commands`、
`app:progress`、`chat:blocks`、`chat:notify`、`chat:status`、`chat:widget`、`chat:retry`、`chat:retry_end`、
`chat:editor_text`（全仓 grep 无发射点；`app:close_requested` 由 shell/window.go:75 发，正常）。
多为 pi 时代遗产：如「引擎进程退出」的兜底 UI 永远不会触发（引擎已进程内化，无进程可退）、
安装进度面板永远空转。不直接致坏，但说明分派表与现役引擎词汇已脱节。

### F7. 计划模式把 `index_search` 拦下（低优先，设计如此）

实测 `POST /chat/plan {plan_enabled:true}` →
`blocked_tools: ["write","edit","bash","index_search"]`。
`backend/agent/policy.go:48-57` 只读白名单没有 `index_search`，按「未登记按写处理」被拦。
`docs/TOOL-REFERENCE.md:44,49` 明文记载这是保守登记的**后果而非缺陷**。
记录在此是因为用户会感知「计划模式下检索用不了」。MCP 工具同理全拦。

### F8. `@` 最近会话条数：键生效，后端读取函数是死代码（低优先）

- 键 `ui.mention_sessions`：默认 5（service/settings.go:71），保存时归一化（:183-191）。
  实测写 7 → `GET /settings` 读回 7 → 已还原 5。
- 前端真实读取：`frontend/src/components/chat/MentionBrowser.vue:25-26` 读
  `app.settings['ui.mention_sessions']`，`:44` 就地步进回写同一键。**配置真实生效。**
- 后端 `Settings.MentionSessions()`（service/settings.go:107-117）**全仓无调用者**（除定义与测试外 grep 零命中）——死代码，非功能缺陷。

### F9. 工作区级配置目录叫 `.pi`（低优先，品牌残留）

- `backend/service/mcp.go:54`、`mcp_runtime.go:116` → `<工作区>\.pi\mcp.json`；
  `backend/service/skills.go:40` → `<工作区>\.pi\skills`。
- 实测 `GET /mcp` 返回 `project_path: D:\GoFiles\WorkBaby\.pi\mcp.json`。
- 读写两侧一致（功能自洽），但与产品名 WorkBaby 不符，且 dev 数据目录里可见 `pi`、`pi-app`、
  `.pi-lens` 等历史残留目录。AGENTS.md §2.5 禁注释留过程痕迹，代码里 `.pi` 属同类迁移残留。
  是否改名属产品决策，此处仅记录。

---

## 三、逐路由实测结果（全部 112 条路由）

信封契约验证：**恒 200 + `{code,message,data}`**，业务失败走 code；无令牌/错令牌回 **401 无信封**（实测 ✅）；
未知路由回 200 + code=1404（实测 ✅）。下表 code 均指信封 code。

### 引导 / 设置 / 偏好

| 路由 | 结果 | 证据摘要 |
|---|---|---|
| GET /bootstrap | ✅ | code=0，含 app_version/app_contract=3/engines/settings/workspaces/runtime/prefs |
| GET /settings | ✅ | code=0，密钥类键掩码 |
| POST /settings/save | ✅ | 写 `ui.font_scale=1.1` 读回 1.1；保留键 `workspace.current` 被拒 code=1000「工作区请走 /settings/workspace」（预期守卫） |
| POST /settings/workspace | ✅ | 切临时工作区成功并广播 workspace:changed；已切回 |
| GET /workspaces | ✅ | 当前+历史行，含 repo/会话概况 |
| POST /settings/workspace/forget | ✅ | 临时工作区已从历史移除 |
| GET /prefs | ✅ | pref_values 全量 |
| POST /prefs/autostart | ⚠️ 未实测 | 会写 HKCU Run 注册表，无人值守环境跳过；代码路径与 prefs 同源 |
| POST /prefs/restore | ✅ | 原值回写 code=0 |
| POST /prefs/close-action | ✅ | 原值回写 code=0 |
| POST /prefs/data-dir | ⚠️ 未实测 | 会弹资源管理器窗口，跳过 |
| GET /logs | ✅ | 当日日志尾部 |

### 引擎 / 应用 / 健康

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /engines | ✅ | native 引擎 installed=true，instance.running=true |
| POST /engines/detect | ✅ | git 2.56.1 / node 就绪，python 未装（如实标注） |
| POST /engines/restart | ✅ | ok:true（内置引擎无进程可重启，语义为中止运行） |
| POST /engines/repair | ✅ | 等价全面刷新；前端未封装此路由（走 detect，非缺陷） |
| GET /health/engines | ✅ | running=true, session_count=34 |
| GET /apps | ✅ | git/node 已装、python 缺失行 |

### 会话 / 对话

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /sessions | ✅ | 34 份历史会话；`?q=回归` 过滤命中临时会话 |
| GET /sessions/detail | ✅ | session+items；标题由首条用户消息派生 |
| POST /sessions/delete | ⚠️ **F3** | 当前会话 5002；切走后 code=0（克隆/分叉会话删除成功） |
| POST /sessions/rename | ❌ **F2** | 当前会话 5002「正被引擎打开」 |
| GET /chat/state | ✅ | history/turn_id/commands/caps/plan_state 齐全 |
| POST /chat/send | ✅/❌ | 正常消息 disposition=started；`/plan on|off` handled；`/export` **started（F4）**；`/compact` 空会话回 1000 可读 |
| POST /chat/stop | ✅ | code=0，无运行时幂等 |
| POST /chat/steer | ✅ | 静止态回 5003「当前没有正在进行的运行」（可读，预期） |
| POST /chat/followup | ✅ | 同上 5003 |
| POST /chat/clear-queue | ✅ | `{steering:[],follow_up:[]}` |
| POST /chat/remove-queued | ✅ | 空队 5003 可读 |
| POST /chat/compact | ✅ | 假上游下回 7000「压缩总结调不通上游」（错误路径可读，不挂死） |
| POST /chat/answer | ⚠️ 存疑 | 不存在的 ask_id + 非空 value 回 ok:true——设计为「超时提问的回答退回会话当普通回复」（chat_actions.go:207-213），不报错是故意的，但调用方无法区分「送达」与「已作废」 |
| POST /chat/new | ✅ | 返回 session_file，广播 session:info |
| POST /chat/clone | ✅ | 新文件生成（已删） |
| POST /chat/fork | ✅ | 从 turn_id 分叉成功；fork_text 为空（omitempty，起点即末条时合理） |
| POST /chat/export-html | ✅ | file_name + 827 字节自包含 HTML |
| POST /chat/model | ✅ | 切到 regress-prov/regress-model，广播 chat:model（前端未接，F6） |
| POST /chat/thinking | ✅ | high 生效 |
| POST /chat/autocompact | ✅ | false 生效 |
| POST /chat/plan | ✅/⚠️ | 开关生效；blocked_tools 含 index_search（F7）；广播 chat:plan 前端未接（F6） |
| POST /messages/hide | ✅ | hide 后 chat/state history 长度 1→0 |
| POST /messages/unhide | ✅ | 恢复 |

### 模型 / 服务商

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /models | ✅ | 平铺模型行 |
| GET /providers | ✅ | 密钥掩码 `sk-f••••3456`，全文无原 key 泄漏 |
| POST /providers/upsert | ✅ | 新建/更新；`api_key:""` 保留原值语义验证通过 |
| POST /providers/delete | ✅ | regress-prov 已删 |
| POST /providers/model/save | ✅ | 单条模型增改（前端未封装，编辑走 upsert 全量，非缺陷） |
| POST /providers/model/delete | ✅ | 单条删除 |
| POST /providers/models | ❌ **F1** | probe_ok=false，URL 打成 `/v1/v1/models`；连不通时错误可读、秒回不挂死 |
| POST /providers/test | ❌ **F1** | 同一 handler 别名，同样问题 |
| GET /models/defaults | ✅ | 读回一致 |
| POST /models/defaults | ✅ | 写读回一致（已还原） |

### 技能 / 模板 / MCP

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /skills | ✅ | 推荐 10 条、目录扫描正常 |
| POST /skills/install | ✅ | skill-creator 装入（已删还原 recommended_installed=0） |
| GET /skills/content | ✅ | file_text 全文 |
| POST /skills/save | ✅ | 编辑后 name/desc 从 YAML 头重新派生 |
| POST /skills/toggle | ✅ | 停用=目录搬入 skills_disabled、启用搬回；台账+广播一致 |
| POST /skills/create | ✅ | 返回 skill_path |
| POST /skills/import | ✅ | 从外部目录导入（skill_dir_name 取目录名而非 YAML name，行为记录） |
| POST /skills/delete | ✅ | 三份临时技能全删净 |
| POST /skills/reset | ✅ | 内置技能覆盖回随包原件 |
| GET /templates | ✅ | rows |
| POST /templates/save | ✅ | 回清洗后的 template_name |
| POST /templates/delete | ✅ | 删净 |
| GET /mcp | ✅ | servers/user_path/project_path（project_path 为 `.pi\mcp.json`，F9） |
| POST /mcp/save | ✅ | stdio 命令行引号分词正确（pkg/cmdline.go:23） |
| POST /mcp/probe | ✅ | 假进程秒退 → 0.25s 回「握手没应答：进程起来了，但在超时内没说出一句合法 JSON-RPC」（可读不挂死） |
| POST /mcp/toggle | ✅ | 开关往返 |
| POST /mcp/remove | ✅ | 清单清空 |

### 联网 / 索引（真实网络本轮可达）

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /web/status | ✅ | duckduckgo 就绪 |
| POST /web/settings | ✅ | kind 切换 |
| POST /web/search | ✅ | 真实 DuckDuckGo 搜索 0.92s 返回 3 条（gin framework） |
| POST /web/fetch | ✅ | https://example.com 正文抽取 156 字符；**环回地址被拒 code=1002「不允许访问内网地址」（SSRF 守卫 ✅）** |
| GET /index/status | ✅ | rebuild 后 doc_count=2 |
| POST /index/rebuild | ✅ | 临时工作区 2 文档 |
| POST /index/search | ✅ | 命中 hello2.txt，score=40「文件名前缀匹配」 |

### 文件 / git / 系统

| 路由 | 结果 | 证据 |
|---|---|---|
| GET /files | ✅ | 列目录（真实仓库根 + 临时工作区各验一次） |
| GET /files/search | ✅ | query=main 命中 main.go/main.ts |
| GET /files/read | ✅ | go.mod 全文；**越界读 C:\Windows\win.ini 拒 code=1002「路径不在工作区内」✅** |
| POST /files/save | ✅ | 新建/覆盖 |
| POST /files/mkdir | ✅ | subdir |
| POST /files/rename | ✅ | hello.txt→hello2.txt |
| POST /files/delete | ✅ | （临时文件清理时验证） |
| GET /git/status | ✅ | 真实仓库：main/origin/main ahead0 behind0；临时仓：init→merging→normal 状态机正确 |
| GET /git/diff | ✅ | 改动后 diff_lines 正确 |
| GET /git/history | ✅ | 真实仓库 5 条 + 临时仓提交 |
| GET /git/commit | ✅ | 单提交详情 |
| GET /git/file-history | ✅ | 单文件历史 |
| GET /git/file-diff | ✅ | 指定提交指定文件差异 |
| GET /git/branches | ✅ | 本地+远端分支 |
| POST /git/stage /unstage /discard /resolve | ✅ | 均在临时仓验证，回最新 status |
| POST /git/commit | ✅ | 普通提交与合并收尾提交 |
| POST /git/branch /switch /branch/delete | ✅ | regress-branch/side 建切删 |
| POST /git/merge | ✅ | 制造真实冲突 → repo_state=merging、has_conflict=true |
| POST /git/conflict/resolve | ✅ | resolve_side=ours 解决后 commit 收尾成功 |
| POST /git/state | ✅ | 无进行中操作时回 1101「git 命令失败」（可读；detail 建议下阶段补充上下文） |
| POST /git/init | ✅ | 临时仓 init -b main |
| POST /git/remote | ✅ | 指向本地裸仓库（不触网） |
| POST /git/push /pull /fetch | ✅ | 对本地裸远端推拉取全通（真实 GitHub 推拉未测：不动用户远端） |
| GET /git/stash/list | ✅ | 真实仓库遗留 stash 只读列出 |
| POST /git/stash/push /pop | ✅ | 临时仓存取 |
| POST /git/stash/drop | ✅ | confirm 必须等于 stash_ref（git_stash.go:64），错确认回 1000 可读，正确确认删除成功 |
| POST /system/reveal | ⚠️ 未实测 | 会弹资源管理器，跳过 |
| POST /system/open | ⚠️ 未实测 | 会用系统默认程序打开文件，跳过 |
| GET /config/files | ✅ | settings/mcp 清单 |
| GET /config/text | ✅ | `?name=settings` 原文；错误名回 1000「未知的配置文件名」（name 用 config_name 而非文件名） |

---

## 四、SSE 链路验证

连接 `GET /events?token=…`（`X-Desktop-Token` 头对 SSE 无效，走查询参数，与前端 http.ts:112 一致）：

| 断言 | 结果 | 证据 |
|---|---|---|
| Content-Type | ✅ | `text/event-stream` |
| 首帧 stream:open 带当前 seq | ✅ | `{seq:22,event:"stream:open"}` |
| seq 单调 | ✅ | 补帧序列 25→26→27→28 连续递增；实时段 1→28 无跳号 |
| 断线补帧 `?since=24` | ✅ | 精确从 seq=25 起补发 4 帧 |
| `?since=99999`（超前） | ✅ | 不报错，只给 open 帧 |
| 心跳 | ✅ | 静置 28s 收到 1 次 `: ping`（注释行不占 seq；周期 25s，sse/heartbeat.go:8） |
| 帧格式 | ✅ | `{seq,event,data,sent_at}`，data 内聊天事件带 run_id/engine_kind |
| 无 key/假上游错误路径 | ✅ | send 秒回 started；SSE 依次 `chat:start → chat:error(可读原文) → chat:settled → chat:done`，**不挂死**；前端 events.ts:260 对 chat:error 会解除 streaming 并 toast |

实测捕获的完整错误事件：

```
seq=26 chat:error {"message":"连不上接口: Post \"http://127.0.0.1:9/v1/chat/completions\":
dial tcp 127.0.0.1:9: connectex: No connection could be made because the target machine actively refused it."}
```

未验证（环境受限）：真实模型流式输出全程（无真实 API key）、token 速率采样、工具卡片实时渲染。
假上游的分片流式已被 `tests/` 覆盖（TestChatFlow），但**真实渲染链路**（含 F5 的字段失配）需要下一阶段带假上游的浏览器端验证。

---

## 五、前后端契约静态核对汇总

- **路径级**：后端 112 条路由 vs 前端 `api/http.ts` 封装，**前端调用的每一条路径后端都存在，无 404 型失配**。
  后端有而前端未封装的 4 条：`/engines/repair`、`/providers/model/save`、`/providers/test`（前端用 `/providers/models` 别名）、
  `/events`（走 EventSource）——均有替代路径，非缺陷。
- **字段级**：前端 types/api-*.ts 327 个字段名 vs 后端 domain json tag 388 个，
  差集仅 `plugin_name`（AppProgress 死事件的遗留字段）与 SSE 信封/补丁字段（见 F5），**业务 RESP 字段全部对得上**，
  且 188 个实测响应的字段与类型声明一致。
- **事件级**：见 F6 两张表（4 个后端事件无人接 + 11 个前端死分支）。
- **斜杠命令**：见 F4 对照表。
- **@ 引用**：见 F8，键真实生效。

---

## 六、环境受限未验证项

| 项 | 原因 |
|---|---|
| 真实模型流式对话（含工具轮次、思考块、提问卡实况） | 无真实 API key；错误路径已验，正常路径仅有 tests/ 假上游覆盖 |
| POST /prefs/autostart | 写 HKCU 注册表，无人值守跳过 |
| POST /prefs/data-dir、/system/reveal、/system/open | 弹桌面窗口/外部程序，跳过 |
| git 对真实 GitHub 远端推拉 | 不动用户远端；以本地裸仓库验证同一代码路径 |
| Brave/Exa/Tavily 搜索 | 无对应 key；DuckDuckGo 真实网络已通 |
| WebView2 内的前端实际渲染 | 本轮为纯 HTTP/SSE + 静态核对；浏览器端 E2E 属下阶段 |

---

## 七、清理回执

- 后端进程树（PID 26536 及全部 WebView2 子进程）已 taskkill /T /F，复查无残留。
- 临时会话 4 份（主/克隆/分叉/挪位）全部删除；临时工作区会话目录已 rmdir。
- 临时服务商 regress-prov、临时技能 3 份、模板 regress-tpl、MCP regress-mcp 全部经 API 删除。
- `workbaby.json` 已逐键还原为回归前快照（含删除本轮新增键）；`workspace.history` 中临时工作区已 forget。
- `%TEMP%\wb-regression\ws`、`remote.git`、`import-skill` 已删除；请求留档 `results.json` 与临时 exe 保留在
  `%TEMP%\wb-regression\` 供下一阶段复现（不属用户数据）。
- 仓库工作区 `git status` 干净（本报告文件除外），业务代码零改动，未引入依赖，无 cgo。

## 八、给下一阶段的修复优先级建议（仅建议，未动手）

1. **F1**：统一 base_url 约定（探测/聊天同拼法 + 占位符/文档三方对齐），这是「配了就用不了」的最大嫌疑。
2. **F2/F3**：放宽当前会话改名/删除（复用引擎写入口或先解绑），或前端补「先切走」流程。
3. **F4**：`HandleCommand` 补 `/export` 分支（或从清单移除）。
4. **F6**：分派表补 4 个 case，清理 11 个死分支；**F5**：`readPatch` 改读 `kind`（或后端改发 `block_kind`，两侧择一并同步 ContractVersion）。
5. F8 死代码、F9 `.pi` 目录名：低优先，随重构顺带。