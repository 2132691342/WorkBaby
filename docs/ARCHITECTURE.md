# 架构总览

## 一句话

WorkBaby = 一个 Go 单进程桌面应用：Wails 出窗口，gin 出本地 HTTP，
agent 内核跑「说话→干活→回话」的循环，SQLite 存一切状态。

## 分层

```
┌──────────────────────────────────────────────────┐
│ WebView（Vue3 SPA）                              │
│  ChatView / SettingsView ── axios ──► gin API    │
│            ▲                            │        │
│            └── SSE /api/v1/events ◄─────┘        │
├──────────────────────────────────────────────────┤
│ Wails 绑定（仅系统能力：对话框/剪贴板/窗口）        │
├──────────────────────────────────────────────────┤
│ internal/api     业务 handler（薄）               │
│ internal/service 编排：会话/对话/审批/配置          │
│ internal/agent   内核：流式循环 + 工具调度          │
│ internal/llm     协议适配：openai/anthropic/ollama │
│ internal/tool    工具：files/exec/python/web      │
│ internal/repo    GORM + SQLite（WAL + FTS5）      │
│ internal/pkg     叶子工具（错误/ID/日志/加密）      │
└──────────────────────────────────────────────────┘
```

依赖方向铁律见 `AGENTS.md §2.2`：`server → api → service → 能力域 → repo → domain`，
`pkg` 是任何层都可用的叶子，自身不依赖任何业务包。

## 一次对话的生命周期

1. 前端 `POST /chat/send` → `service.Chat.Send` 校验 + 落 user 条目 + 建 run
2. 起 goroutine 组装 `agent.Config`（系统提示 → 工具 → 预算 → 闸门）并 `Run`
3. 内核单层循环：轮间压缩 → 注入插话 → 流式调模型 → 执行工具 → 按序回填
4. 事件经 `Emitter → Hub → SSE` 推给前端；需要审批时闸门挂起等决策
5. run 结束：落 assistant 条目与 token 用量，推 `chat:done`，前端拉权威快照

## 四个关键取舍

| 决策 | 理由 |
|---|---|
| 业务全走 HTTP + SSE，Wails 只留系统能力 | 单一通信面；前端可在浏览器里调试 |
| 单层内核循环 + 单一插话队列 | 终端 REPL 那种「停了自动续跑」的外层循环在桌面端没有价值，UI 已经表达了排队语义 |
| 压缩不调 LLM | 确定性、零延迟、可单测；数据库永远存完整历史，裁剪只发生在发给模型那一刻 |
| SQLite 单文件 | 本机单用户，零运维；FTS5 trigram 解决中文检索 |
| 内置运行时只留 Python | 办公场景（表格/文档/图片）覆盖面最大，体积可控 |
| 关闭到托盘 | 常驻待命，符合「助手」心智 |

## 可靠性设计

| 风险 | 对策 |
|---|---|
| assistant(tool_calls) 与 tool 结果错位 | 工具结果按调用下标回填；发送前 `CleanForProtocol` 兜底 |
| 模型卡在同一个工具调用上打转 | 同名同参重复第 3 次直接判失败，结果写回让模型换路 |
| SSE 客户端消费过慢 | 缓冲写满即断连，靠 `Last-Event-ID` 重放补齐；重放窗口滚过则发 `chat:gap` 让前端拉快照 |
| 审批无人值守 | 5 分钟超时按拒绝，绝不自动放行 |
| 压缩把上下文裁坏 | 切点只在 user 边界；切不出完整 turn 就放弃裁剪 |

## 分层规模

服务层是最大的包（编排逻辑集中），内核是最小的包（纯逻辑、无 IO 依赖）。
前端无 UI 组件库，控件全部自绘，色值与字体只在 `themes.css` 定义。
