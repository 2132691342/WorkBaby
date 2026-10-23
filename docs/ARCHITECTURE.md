# 架构

## 一句话

**内核归 PI，界面归 WorkBaby**：Agent 循环在外部 `pi --mode rpc` 进程里，
本项目做发现 / 拉起 / 守护 / 协议编解码 / 事件翻译 / 会话文件索引，以及一层能操作它的界面。

## 分层

```
前端（Vue 3 + TS）：视图 / Pinia store / SSE 订阅
  ↓ HTTP（gin，回环随机端口）+ SSE
server    路由 / SSE hub / 统一响应 / 方法白名单
  ↓
api       薄 handler + 系统能力绑定 + 组合根（唯一 import wails 的包）
  ↓
service   编排：会话 / 对话 / 模型 / 技能 / 插件 / 工具 / MCP / 统计 / 引擎 / 工作区 …
  ↓
能力域    pi（子进程 + 协议 + 会话索引）· skill · git · runtime
  ↓
repo      GORM（只有设置 KV）
domain    域模型（DO / DTO / REQ / VO / RESP 同居一处）
pkg       叶子工具包（AppError / ID / 日志 / httprules / fsutil）
```

每层能依赖谁、禁止依赖谁，唯一权威表在 [`AGENTS.md` §2.2](../AGENTS.md)；
这张图只表达顺序与数据流。依赖方向由 `tools/check-boundaries` 按编译器视角的真实
import 图检查（`go list -json`），不做文本 grep——注释与字符串里的 import 字样会让 grep 得出错误结论。

**api = 组合根**：`Handler.Startup` 是唯一装配入口，持有 Repo / Cfg / 子进程句柄。
业务 handler 方法必须经 service 走流程，唯一例外是查询型系统能力（运行时状态检测）。

## 两个真相源

| 根 | 内容 | 谁写 |
|---|---|---|
| `~/.pi/agent/`（或 `PI_CODING_AGENT_DIR`） | 会话 JSONL、settings / models / mcp、技能、扩展、包 | PI（WorkBaby 只通过 RPC 或 PI 认可的字段写） |
| `%APPDATA%\WorkBaby\` | SQLite（只有设置 KV）、内置运行时、日志、背景图、persona.md | WorkBaby |

**不能有第三本账**：agent 目录只继承、不覆盖。换成私有目录的话，界面里技能、插件、模型、
会话全是空的，而命令行里一切正常——两本账必然分叉。这条纪律写死在 `pi/env`：
进程环境只继承 `PI_CODING_AGENT_DIR`，自己不注入也不覆盖它。

WorkBaby 不另存一份会话与用量：它们是 PI 写的文件，界面只读。
「再存一份」的代价是两处需要失效，而任何一处漏失效都会让界面与内核对不上。

## 启动时序

```
wails.Run
 └─ app.startup → api.Handler.Startup
     ├─ runtime.Resolve()        数据目录 + agent 目录（只解析，不创建 agent 子目录）
     ├─ InitLog / config.Load / db.Open / repo.New
     ├─ reportRuntime()          内置运行时探测（失败只告警）
     ├─ pienv.New(paths)         PI 的发现（PATH → npm 前缀）
     ├─ skill.New() + Emitter
     ├─ service.New(env)         装配全部服务 + PI 监督器
     └─ container.Bootstrap()    设置默认值 / 生成 persona / 扫技能
 └─ domReady → 起 HTTP → 广播端口一次（app:ready）→ 前端握手（事件 + 绑定方法拉取）→ SSE 订阅
```

**环境闸门在 spawn 之前**：前端读 `bootstrap.pi_env`，`ready=false` 就整屏进向导，
不会出现「第一条消息必然失败，十几秒后才浮现」的体验。

端口握手是硬约束：`app:ready` 先于前端 axios 初始化就会漏掉事件。
所以端口有两条路可拿：事件（已注册监听器的场景）与 `ServerPort()` 绑定方法（随时可拉，幂等），
前端启动时两条并行，谁先到算谁的——不靠重复广播去蒙一个时间窗口。

## 运行态通道

| 通道 | 用途 |
|---|---|
| HTTP（gin，只绑 127.0.0.1 + 随机端口） | 全部业务接口；只允许 GET / POST，路径变量必须在末尾 |
| SSE `/api/v1/events` | 唯一运行态通道：事件流 + `last_event_id` 重放 + 缓冲溢出时 `chat:gap` |
| Wails 绑定（7 个方法） | 只保留系统能力：端口查询 / 退出 / 对话框 / 自启 |

## 一次对话

```
前端 POST /chat/send {workspace, content, session_id?}
  → ChatService.Send
      ├─ session_id 为空 → supervisor.Ensure(cwd) → new_session → get_state
      ├─ 已有会话 → 若进程停在别处则 switch_session（要求该目录空闲）
      ├─ Prompt()（运行中带 streamingBehavior=steer）
      └─ 返回 {run_id, session_id}
PI 子进程 stdout（JSONL）
  → pi.Client 按 LF 分帧 → 记录分类 → translator.OnEvent
  → 翻译成 chat:* → Emitter（seq + 按会话串行）→ Hub → SSE
  → agent_settled → chat:done → 前端拉会话快照对账
```

### 事件出口

`service.Emitter` 是唯一出口：注入会话归属、分配 seq、按会话分道串行化。
「分配 seq」与「投递」是一次原子步，否则前端按 seq 去重会把先到的当旧事件丢掉。

SSE Hub 每个会话维护重放缓冲；慢客户端先挤 delta 保关键事件，
回灌再丢关键事件就断开并提示对账；缓冲裁掉中段时主动发 `chat:gap`。

### 引擎提问

PI 的扩展可以 `ctx.ui.confirm/select/input/editor` 问用户。这些请求走
`extension_ui_request` → `chat:ask` → 界面卡片 → `POST /chat/answer` → 回写响应。

协议只有这四个提问基元，界面**不自己发明第五种**：多选与「输入自己的想法」是
扩展层的回退组合（`select` 末尾追加哨兵行再发 `input`，或把编号清单折进 `input`
的标题、格式说明放进 `placeholder`）。所以 `placeholder` / `prefill` / `timeout`
必须原样透传，界面也不把清单重画成复选框——PI 认不出我们编出来的答案形态。
详见 `specs/10-pi-supervisor.md`「提问在界面上的形态」。

### 收尾：从临时态换回权威态

`agent_settled` 是唯一的「真的结束」信号，它挂 `chat:done`。但 done 到「快照拉回来」
之间有一个空窗：done 已经把流式状态清了，而权威消息还在 HTTP 往返里。直接清空会
表现为「答案凭空消失一下又回来」，所以 store 里有一个 `finalizing` 中间态：
本轮已写好的内容继续挂在流上，`reload()` 完成（或失败已告知）后才调 `clearLive()` 换掉。

同理，**切会话一定清空流式态**：SSE 按会话订阅，残留的上一段正文会流进新会话的画面。

### 消息顺序：内容块是渲染依据

一条 assistant 消息的 `content` 是有序块数组（`thinking` / `text` / `toolCall`），
投影时按数组顺序产出 `MessageVO.blocks`，界面就按这个顺序就地渲染：

| 块 | 界面 |
|---|---|
| `thinking` | 就地折叠的思考段（不抽到回合顶部） |
| `text` | markdown 正文 |
| `toolCall` | 工具卡；它的结果在紧随其后的 `tool` 消息上，按 `tool_call_id` 对上号 |

这是这条链路上最容易做错的一处：按**字段类型**分组渲染（所有工具卡一批、所有正文一批）
看起来也能把东西展示全，但它把一次动作的先后丢了——用户读不出「助手先改了什么、
后说了什么」。所以顺序必须是数据的一部分，而不是界面自己排的。

流式中的回合走同一套形态：store 里的 `parts` 是有序段（thinking / text / tool），
`tool_start` / 文本增量都往尾部追加，换段就开新段。历史回合与实时回合因此长得一样。

工具卡上的「看改动」就地开抽屉读该文件的版本差异（走 `/git/diff`）：
看改动是读一眼的事，跳去工作区再把消息列表的滚动位置找回来不是。

### 引擎的即时反馈不进消息流

扩展可以 `setStatus` / `setWidget` / `notify` 报自己干了什么。三条路分得很清：

| 方法 | 去向 |
|---|---|
| `notify` | 轻提示（自动消失，不占版面） |
| `setStatus` / `setWidget` | 输入框上沿一排 chip（`EngineStatusBar`） |
| `setTitle` | 窗口标题 |

它们是「引擎正在干什么」的运行读数，不是对话内容：放进消息流就会在一轮结束后
留下一条永远在那儿的正文，还会把扩展自己的状态覆盖当成历史读数读。

## 前端状态归属

| store | 管什么 |
|---|---|
| `session` | 会话列表、当前会话、消息快照（来自 `/sessions`） |
| `chat` | 当前 run 的有序段（思考 / 正文 / 工具）、提问卡、上下文水位、错误与重试、收尾态 |
| `settings` | bootstrap 快照、服务商列表、技能列表、PI 配置快照、设置 KV |
| `toast` | 轻提示 |

规则：**SSE 只推进行中的临时态**，任何一次 `chat:done` / 断线重连都拉一次权威快照，
前端不做增量合并——合并逻辑是「两个会话叠在一起」这类 bug 的温床。

**事件消费挂在应用壳上，不挂在对话页**：`composables/chatBridge.ts` 在 `App.vue`
的 setup 里装一次，SSE 事件直接落进 `chat` store。run 在用户切去设置页 / 工作区时
还在跑，消费方随视图卸载就意味着 done 与正文全部丢掉，切回来只能看到一份「运行中」
的僵尸快照。同理，重新进入对话页只补快照（且只在没在跑时），不重置 store。

**跑到一半不切会话、不开新对话**：SSE 一条连接只跟一条会话，切走就再也接不上；
界面在运行中拒绝切会话 / 新建对话并明说原因（5005 与 `chat.busy*` 提示），
而不是默默把那一轮抛下。

## 页面边界

六个路由，各自回答一个真问题：**聊得怎么样（对话）**、**它动了什么（工作区）**、
**花了多少（仪表盘）**、**怎么配（设置 + 帮助）**，外加首次启动的**环境向导**。

不为「看起来全」加总览页。这条边界的判据是「PI 是否已有」：已有就在界面上暴露它，
不自研第二份。路由与每页内容的细节见 [PAGE-STRUCTURE](PAGE-STRUCTURE.md)。

## 端口与安全

| 面 | 措施 |
|---|---|
| 网络 | 只绑 `127.0.0.1`，随机端口；端口经 `app:ready` 事件或 `ServerPort()` 绑定方法告知前端 |
| CORS | 只放行本机来源（`wails://`、`localhost`、`127.0.0.1`、`wails.localhost`） |
| 方法 | 白名单只有 GET / POST；路径变量必须在末尾 |
| 路径 | 文件列举 / 文档名 / 附件都过 `pkg.SafeJoin`，越界在门口拒绝 |
| 密钥 | 明文只在「显式点显示」时回传一次，界面默认掩码 |
| 子进程 | 入口不走 shell（npm shim 还原成 node + 脚本），全部子进程设 `CREATE_NO_WINDOW` |

## 关键取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 内核归 PI | 工具 / 压缩 / 重试 / MCP / 扩展全由内核演进，界面零维护 | 界面能力受协议面约束，不能「先做出来再说」 |
| 会话文件即真相 | 与命令行天然一致，不会两本账 | 列表要扫盘（几百文件毫秒级，按 size+mtime 缓存） |
| 业务走 HTTP 而非 Wails 绑定 | 前端在浏览器里就能跑通，调试不必开窗 | 启动顺序变成硬约束（端口握手必须早于 domReady） |
| 一个工作区一个进程 | cwd 是进程级属性，工具不会写错目录 | 同目录多条对话不能并行（内核约束，如实报 5005） |
| SQLite 只存设置 KV | 没有需要迁移的业务表，卸载即净 | 界面偏好之外的状态都在 PI 的文件里 |
| 消息按内容块顺序渲染 | 一次动作的先后读得出来，实时回合与历史长得一样 | 顺序得由后端进契约（`blocks`），界面不能自己排 |
| 只增强不干预内核 | 默认行为与 `pi` 一致，装了扩展也不被界面抢跑 | 想要审批闸门得自己装扩展，界面不伪造一个 |
