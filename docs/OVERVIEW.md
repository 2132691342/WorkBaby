# 项目全景

这一篇回答四个问题：**WorkBaby 是什么、由哪些模块组成、数据怎么流、每个决定换来了什么**。
看完它再进各子系统规格（`specs/`）与契约文档（`docs/`），不需要先读代码。

## 一句话

WorkBaby 是 PI 的桌面工作台。**Agent 内核是外部 `pi --mode rpc` 进程，本项目不重写内核**，
只做六件事：发现 / 拉起 / 守护 / 协议编解码 / 事件翻译 / 会话文件索引，外加一层能操作它的界面。

```
PI 负责运行 Agent，WorkBaby 负责让你看见并控制它是怎么运行的。
```

## 能力清单

| 能力 | 落在哪 | 一句话 |
|---|---|---|
| 对话 | 对话页 | 流式正文、思考段、工具卡按**原始顺序**内联；插话 / 排队 / 停止 / 自动重试提示 |
| 提问 | 对话页 | 引擎扩展 `confirm` / `select` / `input` / `editor` 四基元出卡，回答写回内核 |
| 会话 | 对话页 | 读 PI 的会话目录：终端里聊过的这里也看得到，点开接着聊 |
| 模型 | 设置 · 模型服务 | 读写 PI 的 `models.json`：这里加的服务商，`pi` 命令立刻能用 |
| 工具 | 设置 · 工具 | 清单来自会话里的工具声明，开关写 `defaultTools` |
| MCP | 设置 · MCP | 配置原文编辑 + 在线状态与工具清单，支持 OAuth 登录 |
| 技能 | 设置 · 技能 | 列出 PI 真正会加载的技能（用户目录 / 工作区 / 插件包），启停靠移目录 |
| 插件 | 设置 · 插件 | 包走 `pi install/remove`，扩展走登记表；安装进度流式打到面板 |
| 工作区 | 工作区页 | 改动 / 提交 / 历史 / 分支：助手动了什么在这里对账并收成一次提交 |
| 用量 | 仪表盘 | token 与估算花费（按天 / 模型 / 会话），全部来自会话文件 |
| 环境 | 环境向导 | 复用系统已有的 Node / Python / PI；缺了才用内置运行时兜底 |

**界面只做「已有就在界面上暴露」**：不自研第二份 agent 能力，不另装审批闸门，
不改 PI 的默认行为。这条边界让界面能力永远跟得上内核演进，代价是界面受协议面约束。

## 模块地图

```
┌──────────────────────────────────────────────────────────────┐
│ 前端 Vue 3 + TS：视图 / Pinia store / SSE 订阅                  │
└───────────────────────────┬──────────────────────────────────┘
              HTTP（gin，回环随机端口）+ SSE
┌───────────────────────────▼──────────────────────────────────┐
│ server    路由 / SSE hub / 统一响应 / 方法白名单                 │
│ api       薄 handler + 系统能力绑定 + 组合根（唯一 import wails） │
│ service   编排：会话 / 对话 / 模型 / 技能 / 插件 / 工具 / MCP /   │
│           统计 / 设置 / 工作区 / 文件 / 事件翻译                  │
│ 能力域    pi（子进程 + 协议 + 会话索引）· skill · git · runtime   │
│ repo      GORM（只有设置 KV）                                   │
│ domain    域模型（DO / DTO / REQ / VO / RESP 同居一处）           │
│ pkg       叶子工具包（AppError / ID / 日志 / httprules / fsutil） │
└──────────────────────────────────────────────────────────────┘
```

| 模块 | 负责什么 | 规格 |
|---|---|---|
| `backend/pi` | PI 子进程监督、RPC 编解码、事件归一、会话文件索引 | [10](specs/10-pi-supervisor.md) |
| `backend/pi/env` | 运行时复用、PI 安装与探测、`models` / `settings` / `mcp` 互通 | [11](specs/11-pi-bootstrap.md) · [01](specs/01-runtime.md) |
| `backend/runtime` | 路径解析 + 内置 Node / Python 运行时（embed + 解压） | [01](specs/01-runtime.md) |
| `backend/service` | 全部业务编排与事件翻译 | 各规格 |
| `backend/skill` | 技能解析与注册表（目录即 PI 的技能目录） | [03](specs/03-skill.md) |
| `backend/git` | 工作区版本视图（读全开放 + 写白名单六动作） | [08](specs/08-workspace-git.md) |
| `backend/server` | 路由表、SSE hub、统一信封 | [API-CONTRACT](docs/API-CONTRACT.md) |
| `frontend/src` | 界面：四块页面 + 一套设计令牌 | [PAGE-STRUCTURE](docs/PAGE-STRUCTURE.md) · [DESIGN](../DESIGN.md) |
| 壳层（`app.go`） | Wails 生命周期、托盘、单实例、退出流程 | [04](specs/04-desktop-shell.md) |

依赖方向是单向的：`server → api → service → 能力域 → repo → domain → pkg`，
由 `tools/check-boundaries` 按编译器视角的真实 import 图把关。**能力域之间不互引**，
要合作就在 `service` 里编排——一互引，独立测试与独立演进同时作废。

## 两个真相源

| 根 | 谁写 | 内容 |
|---|---|---|
| `~/.pi/agent/`（或 `PI_CODING_AGENT_DIR`） | **PI** | 会话 JSONL、`settings.json`、`models.json`、`mcp.json`、技能、扩展、包 |
| `%APPDATA%\WorkBaby\` | **WorkBaby** | SQLite（只有界面设置 KV）、内置运行时、日志、背景图、`persona.md` |

**不能有第三本账**。agent 目录只继承、不覆盖：换成私有目录的话，界面里技能、插件、模型、
会话全是空的，而命令行里一切正常。WorkBaby 也不另存会话与用量——它们是 PI 写的文件，
界面只读。「再存一份」的代价是两处需要失效，任何一处漏失效都会让界面与内核对不上。

字段级细节见 [DATA-MODEL](docs/DATA-MODEL.md)。

## 一次对话的完整链路

```
前端 POST /chat/send {workspace, content, session_id?}
  → ChatService.Send
      ├─ session_id 为空 → supervisor.Ensure(cwd) → new_session → get_state
      ├─ 已有会话 → 进程停在别处则 switch_session（要求该目录空闲）
      ├─ beginRun：登记 runState + busy 标记 + 等「prompt 已被接受」
      └─ 返回 {run_id, session_id}
PI 子进程 stdout（JSONL）
  → pi.Client 按 LF 分帧 → 记录分类 → translator.OnEvent
  → 翻译成 chat:* → Emitter（seq + 按会话串行）→ Hub → SSE
  → agent_settled → chat:done → 前端拉会话快照对账
```

四个关键判断：

| 判断 | 为什么 |
|---|---|
| `chat:done` 只挂 `agent_settled` | 重试 / 压缩 / 插话都挂在 `agent_end` 之后，提前完成会让界面自相矛盾 |
| 消息按内容块**顺序**渲染 | 按字段类型分组会把一轮里所有工具卡顶到正文前面，一次动作的先后读不出来 |
| SSE 只推临时态，权威态一律拉快照 | 增量合并是「两个会话叠在一起」这类 bug 的温床 |
| 一个工作区一个进程 | `cwd` 是进程级属性，共进程会让工具写到错的目录 |

## 运行态怎么到达界面

| 通道 | 用途 |
|---|---|
| HTTP（gin，回环随机端口） | 全部业务接口；只允许 GET / POST，路径变量必须在末尾 |
| SSE `/api/v1/events` | 唯一运行态通道：事件流 + `last_event_id` 重放 + 缓冲溢出时 `chat:gap` |
| Wails 绑定（7 个方法） | 只保留系统能力：端口查询 / 退出 / 对话框 / 自启 |

端口握手是硬约束：`app:ready` 事件与 `ServerPort()` 绑定方法两条路并行，谁先到用谁——
不靠重复广播去蒙一个时间窗口。前端因此也能在浏览器里跑通界面，调试不必开窗。

## 关键取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 内核归 PI | 工具 / 压缩 / 重试 / MCP / 扩展全由内核演进，界面零维护 | 界面能力受协议面约束，不能「先做出来再说」 |
| 会话文件即真相 | 与命令行天然一致，不会两本账 | 列表要扫盘（几百文件毫秒级，按 size+mtime 缓存） |
| 业务走 HTTP 而非 Wails 绑定 | 前端在浏览器里就能跑通，调试不必开窗 | 启动顺序变成硬约束（端口握手必须早于 domReady） |
| 一个工作区一个进程 | 工具落到对的目录 | 同目录多条对话不能并行（内核约束，如实报 5005） |
| SQLite 只存设置 KV | 没有需要迁移的业务表，卸载即净 | 界面偏好之外的状态都在 PI 的文件里 |
| 写 git 走白名单六动作 | 「准备一次提交」在应用内闭环 | `rebase` / `cherry-pick` / `stash` 仍要回用户自己的 Git 工具 |
| 内置运行时只做兜底 | 用户机器上什么都没装也能起来 | 安装包大 48MB，首启多等一次解压 |
| 归档嵌进 exe | 分发只有一个文件 | exe 约 90MB，构建期强依赖 LFS 拉取 |

## 从这里往下读

| 想知道 | 读 |
|---|---|
| 分层、启动时序、一次对话 | [ARCHITECTURE](ARCHITECTURE.md) |
| 哪些数据属于谁、字段口径 | [DATA-MODEL](DATA-MODEL.md) |
| HTTP 与 SSE 契约 | [API-CONTRACT](API-CONTRACT.md) |
| 页面与组件地图 | [PAGE-STRUCTURE](PAGE-STRUCTURE.md) |
| 视觉语言与交互原则 | [DESIGN](../DESIGN.md) |
| 错误码与前端分流 | [ERROR-CODES](ERROR-CODES.md) |
| 构建、CI 与分发 | [DEPLOYMENT](DEPLOYMENT.md) |
| 环境要求、验证入口、排障 | [DEVELOPMENT](DEVELOPMENT.md) |
| 某个子系统的完整设计 | [specs/](specs/) |
