# 架构 · ARCHITECTURE

## 定位

WorkBaby 是 Windows 桌面单机 AI 助手：Tauri 壳 + 内嵌 HTTP 服务 + 自研 Rust agent 内核。
一切业务走 `127.0.0.1:{随机端口}` 的 HTTP 与 SSE；Tauri 只承担系统能力（窗口/托盘/对话框/自启/单实例）。

## 总体分层

```text
┌──────────────────────────────────────────────────────────────┐
│ frontend (Vue3 + WebView2)                                    │
│   axios ──► HTTP /api/v1/*        EventSource ──► /events SSE │
│   invoke ──► Tauri commands（仅系统能力：对话框/窗控/自启/退出）│
├──────────────────────────────────────────────────────────────┤
│ src-tauri（壳）          wb-api（HTTP + SSE hub + 组合根）      │
├──────────────────────────────────────────────────────────────┤
│ wb-service（编排：会话/对话/设置/技能/功能开关/统计/翻译）        │
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

## 深入阅读

| 主题 | 文档 |
|---|---|
| Agent 内核：会话树、循环、压缩、重试 | [KERNEL.md](KERNEL.md) |
| 工具体系：Tool 契约、内置工具、执行纪律 | [TOOLS.md](TOOLS.md) |
| 模型接入：三族 wire API、流式解析、错误分类 | [PROVIDER.md](PROVIDER.md) |
| 会话存储：JSONL 条目类型、回放算法 | [SESSIONS.md](SESSIONS.md) |
| 技能体系：SKILL.md 格式、三区发现、注入 | [SKILLS.md](SKILLS.md) |
| 12 项内置功能 | [FEATURES.md](FEATURES.md) |
| 工作区与 Git 写白名单 | [WORKSPACE.md](WORKSPACE.md) |
| MCP 客户端与连接池 | [MCP.md](MCP.md) |
| 前端架构、令牌、构建分块 | [FRONTEND.md](FRONTEND.md) |
| 桌面壳、进程模型、数据目录 | [SHELL.md](SHELL.md) |

## 一次对话的完整路径

1. 前端 `POST /chat/send`（session_id + 文本）→ `wb-api` handler → `wb-service::chat`
2. service 校验忙闲与队列，生成 `RUN_` id，把用户消息写进内核会话，唤醒内核
3. `wb-kernel` 组装上下文（leaf→root 回放 + service 侧组装的系统提示词）→ 调 `wb-provider` 流式采样
4. 内核事件经 service 翻译成 `chat:*` SSE 事件（Emitter 分配 seq、按会话串行）推 Hub
5. 工具调用在内核执行（`wb-tools` 注册表 + `wb-features`/`wb-mcp` 提供的扩展工具），结果截断后回灌
6. `Settled` 触发 `chat:done`；前端断线重连凭 `Last-Event-ID` 对账

## 双通道职责边界

| 通道 | 承载 | 禁止承载 |
|---|---|---|
| HTTP（GET/POST） | 全部业务请求：会话/对话控制/配置/技能/功能开关/统计/Git | 流式推送 |
| SSE | `chat:*` 实时事件 + 全局进度事件 | 请求-响应 |
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

