# 架构 · ARCHITECTURE

## 定位

WorkBaby 是 Windows 桌面单机 AI 助手：Tauri 壳 + 内嵌 HTTP 服务 + 自研 Rust agent 内核。
一切业务走 `127.0.0.1:{随机端口}` 的 HTTP 与 SSE；Tauri 只承担系统能力
（窗口 / 托盘 / 对话框 / 自启 / 单实例 / 剪贴板 / 外链）。

## 总体分层

```text
┌──────────────────────────────────────────────────────────────┐
│ frontend (Vue3 + WebView2)                                    │
│   axios ──► HTTP /api/v1/*        EventSource ──► /events SSE │
│   invoke ──► Tauri commands（仅系统能力：对话框/窗控/自启/退出）│
├──────────────────────────────────────────────────────────────┤
│ src-tauri（壳）          wb-api（HTTP + SSE hub + 组合根）      │
├──────────────────────────────────────────────────────────────┤
│ wb-service（编排：对话 / 会话 / 设置 / 技能 / 功能开关 / 统计）  │
├──────────────┬───────────────┬──────────────┬────────────────┤
│ wb-kernel    │ wb-skills     │ wb-git       │ wb-mcp          │
│ (agent 循环) │ (技能注册表)   │ (版本视图)    │ (MCP 客户端)    │
│ wb-tools     │               │              │ wb-features     │
│ (工具集)     │               │              │ (12 项内置功能)  │
├──────────────┴───────────────┴──────────────┴────────────────┤
│ wb-provider（模型 wire API）   wb-repo（SQLite）                │
├──────────────────────────────────────────────────────────────┤
│ wb-domain（纯类型）  →  wb-pkg（叶子工具）                       │
└──────────────────────────────────────────────────────────────┘
```

依赖方向单向（见 `AGENTS.md` §2.2）：`wb-service` 编排全部能力域；
`wb-api` 作为组合根，开库（`Repo::open`）与会话文件操作（`SessionFile` / `Entry`，
改名 / 切模型 / 切思考档 / 导出 / 分支）在 api 层直接做，其余能力域一律经 service。

## 代码地图

找不到东西时先看这一节：每个问题域对应一个 crate 或一组文件。

| 我想改的东西 | 去哪看 |
|---|---|
| 会话怎么存、怎么回放、怎么分支 | `wb-kernel/src/{session,session_msg,session_repair}.rs` |
| 主循环、工具批、重试、压缩、子代理 | `wb-kernel/src/{agent,blocks,toolbatch,compaction,subagent}.rs` |
| 系统提示词怎么拼 | `wb-service/src/prompt.rs` |
| 内核事件怎么变成 SSE | `wb-service/src/chat_events.rs` + `wb-domain/src/events.rs` |
| 对话 run 管理、排队、目标续跑 | `wb-service/src/chat.rs` |
| 功能怎么装入注册表 | `wb-service/src/features.rs` |
| 功能面板的读与写 | `wb-service/src/{feature_state,feature_write,feature_config}.rs` |
| 工作区热切换 | `wb-service/src/workspace.rs` + `wb-api/src/system_routes.rs` |
| 内置运行时解压 | `wb-service/src/runtime.rs` + `wb-domain/src/runtime.rs` |
| 单个内置功能 | `wb-features/src/<功能名>.rs`（或同名子目录） |
| 工具契约与输出截断 | `wb-tools/src/{registry,exec,truncate,difftext}.rs` |
| 流式协议解析 | `wb-provider/src/openai.rs` |
| 路由表 | `wb-api/src/routes.rs`（主表）与同目录 `*_routes.rs` |
| SSE 重放缓冲 | `wb-api/src/hub.rs` |
| git 读写 | `wb-git/src/{exec,parse,write}.rs` |
| 技能扫描与注入 | `wb-skills/src/{frontmatter,registry}.rs` |
| 前端 SSE 事件桥 | `frontend/src/composables/{useSse,chatBridge}.ts` |
| 前端对话状态机 | `frontend/src/stores/{chat,session}.ts` |
| 色值与尺寸令牌 | `frontend/src/{themes,scale}.css` |

## 深入阅读

| 主题 | 文档 |
|---|---|
| 每个 crate 的职责与边界 | [MODULES.md](MODULES.md) |
| Agent 内核：会话树、主循环、压缩、重试、子代理 | [KERNEL.md](KERNEL.md) |
| 会话存储：JSONL 条目类型、回放算法 | [SESSIONS.md](SESSIONS.md) |
| 工具体系：Tool 契约、内置工具、执行纪律 | [TOOLS.md](TOOLS.md) |
| 模型接入：wire API、流式解析、错误分类 | [PROVIDER.md](PROVIDER.md) |
| 技能体系：SKILL.md 格式、三区发现、注入 | [SKILLS.md](SKILLS.md) |
| 12 项内置功能 | [FEATURES.md](FEATURES.md) |
| 工作区与 Git 写白名单 | [WORKSPACE.md](WORKSPACE.md) |
| MCP 客户端与连接池 | [MCP.md](MCP.md) |
| 前端架构、令牌、构建分块 | [FRONTEND.md](FRONTEND.md) |
| 桌面壳、进程模型、数据目录 | [SHELL.md](SHELL.md) |
| 数据模型 | [DATA-MODEL.md](DATA-MODEL.md) |
| HTTP 与 SSE 契约 | [API.md](API.md) |

## 一次对话的完整路径

1. 前端 `POST /chat/send`（`chat_session_id` + `chat_text`）→ `wb-api` handler → `wb-service::chat`
2. service 校验忙闲与队列，生成 `RUN_` id，把用户消息写进内核会话，唤醒内核
3. `wb-kernel` 组装上下文（leaf→root 回放 + service 侧组装的系统提示词）→
   调 `wb-provider` 流式采样
4. 内核事件经 service 翻译成 `chat:*` SSE 事件（Emitter 分配 seq、按会话串行）推 Hub
5. 工具调用在内核执行（`wb-tools` 注册表 + `wb-features` / `wb-mcp` 提供的扩展工具），
   结果截断后回灌
6. `Settled` 触发 `chat:done`；前端断线重连凭 `last_event_id` 对账

## 双通道职责边界

| 通道 | 承载 | 禁止承载 |
|---|---|---|
| HTTP（GET/POST） | 全部业务请求：会话 / 对话控制 / 配置 / 技能 / 功能开关 / 统计 / Git | 流式推送 |
| SSE | `chat:*` 实时事件 + `app:*` 全局广播 | 请求-响应 |
| Tauri invoke | 系统能力：server_port、对话框 ×3、自启、退出、窗控 | 业务数据 |

## 关键取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| HTTP+SSE 而非 Tauri 全绑定 | 前端 api 层统一，可浏览器调试 | 多一层端口握手 |
| 会话存 JSONL 而非 SQLite | 追加即落盘、崩溃安全、树形分支天然表达 | 列表/统计需索引表配合 |
| 内核进程内而非子进程 | 无 RPC 编解码、无进程守护、启动即用 | 内核 panic 影响整个进程（用错误边界兜底） |
| 能力原生化而非运行时插件 | 加载失败、版本不兼容在编译期消失 | 失去第三方生态；剩余扩展由技能与 MCP 承接 |
| 功能开关即装配边界 | 关掉的功能模型根本看不到 | 切换要重建注册表 |
| 只发 Windows | 用户群明确；Tauri 依赖 WebView2 系统自带 | `cfg(windows)` 隔离点需保持纪律 |
| 不做工具审批闸门 | 单机个人助手，流程少一步 | 风险由 Git 写白名单与输出截断兜底 |

## 不变量

这些约束散落在各 crate 实现里，改动时必看：

| 不变量 | 含义 |
|---|---|
| 内核不感知界面 | `KernelEvent` 是内核唯一出参语言，不含任何界面词汇 |
| 内核不依赖上层 | `wb-kernel` 不知道 service / api / 界面的存在 |
| 工具契约唯一 | 所有工具（内置 / 功能 / MCP）实现同一个 `Tool` trait |
| 事件顺序是契约 | 界面时序依赖事件顺序，改动顺序等于改界面行为 |
| 会话文件是真相 | 界面不另存状态，重启后从 JSONL 回放 |
| 事件写对会话 | 功能工具经 `ToolCtx` 拿会话归属，提问 / 状态行只发本会话 |
| 功能开关即边界 | 关掉的功能不进工具表、不发事件 |
| 启动不等重物 | 内置运行时解压与 MCP 连接都在后台跑，端口与界面先就绪 |
| 工作区可换 | 切换会重建全部按目录派生的状态并广播 `app:workspace` |
| 看得见就点得动 | 活状态面板（待办 / 目标 / 计划 / 记忆）必须可写 |
| 写入只有一套 | 界面写入与模型工具共用同一份落盘与同一套校验 |
| 活状态分层归属 | 待办按工作区共享；目标按会话隔离（目标只对登记它的对话有意义） |
| 截断必留句柄 | 超长输出先落盘再截断，被砍掉的部分必须能再取回来 |
