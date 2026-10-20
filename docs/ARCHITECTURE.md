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

| 层 | 职责 | 禁止 |
|---|---|---|
| `server` | 路由、SSE hub、统一响应、方法白名单 | 引 service / repo / 能力域 |
| `api` | 薄 handler（解析参数 → 调 service → 统一信封）+ 系统能力绑定 + 组合根 | handler 绕过 service 直写业务 |
| `service` | 业务编排、事务边界、事件翻译 | 引 gin / wails / api / server；写 SQL |
| 能力域 `pi` / `skill` / `git` | 子进程与协议 / 技能目录 / 受控 git 写入 | 回引 api / service / server |
| `repo` | GORM 持久化 | 引上层 |
| `domain` | 域模型（DO / DTO / REQ / VO / RESP 同居一处） | 引任何上层 |
| `pkg` | 叶子工具包 | 引任何其他 backend 业务包 |

依赖方向由 `tools/check-boundaries` 按编译器视角的真实 import 图检查（`go list -json`），
不做文本 grep——注释与字符串里的 import 字样会让 grep 得出错误结论。

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

## 前端状态归属

| store | 管什么 |
|---|---|
| `session` | 会话列表、当前会话、消息快照（来自 `/sessions`） |
| `chat` | 当前 run 的流式正文、工具卡、提问卡、上下文水位、错误与重试 |
| `settings` | bootstrap 快照、服务商列表、技能列表、PI 配置快照、设置 KV |
| `toast` | 轻提示 |

规则：**SSE 只推进行中的临时态**，任何一次 `chat:done` / 断线重连都拉一次权威快照，
前端不做增量合并——合并逻辑是「两个会话叠在一起」这类 bug 的温床。

## 页面边界

界面只保留四块页面，各自回答一个真问题：**聊得怎么样（对话）**、**它动了什么（工作区）**、
**花了多少（仪表盘）**、**怎么配（设置 + 帮助）**。

不为「看起来全」加总览页。这条边界的判据是「PI 是否已有」：已有就在界面上暴露它，
不自研第二份。

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
