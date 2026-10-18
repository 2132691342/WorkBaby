# 架构

## 一句话

**内核归 PI，界面归 WorkBaby**：Agent 循环在外部 `pi --mode rpc` 进程里，
本项目做发现 / 拉起 / 守护 / 协议编解码 / 事件翻译 / 会话文件索引，以及一层能操作它的界面。

## 分层

```
┌──────────────────────────────────────────────┐
│ 前端（Vue 3 + TS）：视图 / Pinia / SSE 订阅      │
└───────────────┬──────────────────────────────┘
        HTTP（gin，回环随机端口）+ SSE
┌───────────────▼──────────────────────────────┐
│ server（路由 / SSE hub / 统一响应 / 方法白名单） │
│ api（薄 handler + 系统能力绑定 + 组合根）         │
│ service（编排：会话 / 对话 / 模型 / 技能 / 插件…） │
│ 能力域：pi（子进程 + 协议 + 会话索引）· skill · git │
│ repo（GORM，只有设置 KV）· domain（域模型）        │
│ pkg（叶子工具包：AppError / ID / 日志 / httprules） │
└──────────────────────────────────────────────┘
```

依赖方向由 `tools/check-boundaries` 按真实 import 图检查：
上层可依赖下层，能力域不得回指 api / service / server，`pkg` 不依赖任何业务包。

## 两个真相源

| 根 | 内容 | 谁写 |
|---|---|---|
| `~/.pi/agent/`（或 `PI_CODING_AGENT_DIR`） | 会话 JSONL、settings / models / mcp、技能、扩展、包 | PI（WorkBaby 只通过 RPC 或 PI 认可的字段写） |
| `%APPDATA%\WorkBaby\` | SQLite（只有设置 KV）、内置运行时、日志、背景图、persona.md | WorkBaby |

**不能有第三本账**：agent 目录只继承、不覆盖。换成私有目录的话，界面里技能、插件、模型、
会话全是空的，而命令行里一切正常——两本账必然分叉。现在的纪律写死在
`pi/env`：进程环境只继承 `PI_CODING_AGENT_DIR`，自己不注入也不覆盖它。

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
 └─ domReady → 前端握手拿端口 → bootstrap / SSE 订阅
```

**环境闸门在 spawn 之前**：前端读 `bootstrap.pi_env`，`ready=false` 就整屏进向导，
不会出现「第一条消息必然失败，十几秒后才浮现」的体验。

## 一次对话

```
前端 POST /chat/send {workspace, content, session_id?}
  → ChatService.Send
      ├─ session_id 为空 → supervisor.Ensure(cwd) → new_session → get_state
      ├─ 已有会话 → 若进程停在别处则 switch_session（要求该目录空闲）
      ├─ Prompt()（运行中带 streamingBehavior=steer）
      └─ 返回 {run_id, session_id}
PI 子进程 stdout（JSONL）
  → pi.Client 分帧 → 记录分类 → translator.OnEvent
  → 翻译成 chat:* → Emitter（seq + 按会话串行）→ Hub → SSE
  → agent_settled → chat:done → 前端拉会话快照对账
```

### 事件出口

`service.Emitter` 是唯一出口：注入会话归属、分配 seq、按会话分道串行化。
SSE Hub 每个会话维护重放缓冲；慢客户端先挤 delta 保关键事件，
缓冲裁掉中段时主动发 `chat:gap`，前端据此拉快照对账。

### 引擎提问

PI 的扩展可以 `ctx.ui.confirm/select/input/editor` 问用户。这些请求走
`extension_ui_request` → `chat:ask` → 界面卡片 → `POST /chat/answer` → 回写响应。
WorkBaby 自己不判断「该不该问」：闸门由扩展决定，界面只负责显示与回传。

## 前端状态归属

| store | 管什么 |
|---|---|
| `session` | 会话列表、当前会话、消息快照（来自 `/sessions`） |
| `chat` | 当前 run 的流式正文、工具卡、提问卡、上下文水位、错误与重试 |
| `settings` | bootstrap 快照、服务商列表、技能列表、PI 配置快照、设置 KV |
| `toast` | 轻提示 |

规则：**SSE 只推进行中的临时态**，任何一次 `chat:done` / 断线重连都拉一次权威快照，
前端不做增量合并——合并逻辑是「两个会话叠在一起」这类 bug 的温床。

## 模块边界

界面只保留四块页面，各自回答一个真问题：**聊得怎么样（对话）**、**它动了什么（工作区）**、
**花了多少（仪表盘）**、**怎么配（设置 + 帮助）**。

不在 WorkBaby 里实现的能力：运行记录与用量（PI 的会话文件里有权威的一份）、
审批策略（审批闸门属于扩展生态，内核默认不打断）、知识库与架构图（与「操作 PI」无关）。
这条边界的判据是「PI 是否已有」：已有就在界面上暴露它，不自研第二份。

## 端口与安全

- 只绑 `127.0.0.1`，随机端口；端口经 `app:ready` 事件告知前端。
- CORS 只放行本机来源（`wails://`、`localhost`、`127.0.0.1`、`wails.localhost`）。
- 方法白名单只有 GET / POST；路径变量必须在末尾。
- 密钥明文只在「显式点显示」时回传一次，界面默认掩码。
