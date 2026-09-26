# 20 · 子智能体与任务委派

## 1. 定位

`delegate_task` 让父 Agent 把可独立完成的大块任务交给子 Agent 执行：上下文隔离、预算独立、只回传摘要，
父上下文不被子任务的中间过程撑爆。子 Agent 有两个来源：**内置定义**（`default` / `explore`）与
**用户自定义档案**（`agent_profiles` 表 + 数据目录 `agents/*.md` 文件）。

## 2. 委派工具

| 项 | 值 |
|---|---|
| 工具名 | `delegate_task`（`internal/tool/delegate`） |
| 参数 | `agent`（内置 `default` 通用全能 / `explore` 只读探索，或自定义档案名；未知回退 `default`）、`task`（必填，需自包含——子 Agent 看不到当前对话） |
| 风险 / 执行模式 | `RiskExec`（子 Agent 会真实调工具）；`ExecutionSequential`（并发相同 `(agent, task)` 才会共享执行，并发触发反而双跑） |
| 能力注入 | run 装配时 `tool.WithDelegator(ctx, …)` 注入委派器（携父 runID、父事件出口、模型与父 run 已暴露工具名）；未注入时返回可见错误，模型据此改道自己干 |
| 回执 | `子 Agent（X）完成，摘要如下：…`（摘要截断到 4000 rune） |

## 3. 四重隔离（`service/delegate_core.go`）

| 维度 | 实现 |
|---|---|
| 上下文 | 子 run 消息只有 `UserMessage(task)` + 档案人设作 system；不带父历史，不压缩 |
| 预算 | `delegateMaxTurns=12`、墙钟 3min、单工具 2min、摘要 4000 rune；子 run 用量独立落库（`Source=delegate`，`Turn=-1`） |
| 工具 | `filterToolNames` = 父 run 已暴露工具 ∩ 档案 Allow/Deny 策略，只收缩不能升权 |
| 正文 | 子 run 的正文/思考不转发；事件出口只放行工具层与生命周期事件并打 `ParentRunID` + `Agent` 标签 |

并发相同 `(agent, task)` 的委派共享一次执行（in-flight 去重）。

## 4. 自定义档案契约（`domain/agent_profile.go`）

表 `agent_profiles`（软删）：

| 字段 | 说明 |
|---|---|
| `Name` | kebab-case，唯一；与内置名 `default` / `explore` 互斥（8202） |
| `SystemPrompt` / `Description` | 人设与描述 |
| `ToolsAllow` / `ToolsDeny` | JSON glob 数组（存 text）；与父 run 已暴露工具求交 |
| `MemoryEnable` | 是否允许子 Agent 读写长期记忆 |
| `MaxTurns` | `<=0` 用委派默认（12）；上限 40（8203） |
| `Model` | 空 = 继承主 Agent |
| `Thinking` | `off/low/medium/high`；仅在指定 `Model` 时可单设 |
| `Enabled` | 禁用档案不参与 Sync |

**注册表同步**：`AgentProfileService.Sync` 把 enabled 行 + 数据目录 `agents/*.md` 文件物化成
`agent.Definition`（同名时以表为准），写入 `agent.SetCustomAgents`；Upsert / SetEnabled / Delete
每次写后即 Sync。`agent.Agent(name)` 未知名回退 `default`（委派与切换共用的唯一入口）。

## 5. 事件契约

子 run 的 `turn.delta / thinking / checkpoint / compressed / retry / queue.drained` 一律不外发；
子工具事件仍推 `chat:tool` / `chat:tool-result`（带 `agent` 标签），但不写父消息块与父工具历史。

| 事件 | 触发 | 载荷 |
|---|---|---|
| `chat:subagent-start` | 子 `run.start` | `{sub_run_id, agent}` |
| `chat:subagent-done` | 子 `run.done`（reason 经 `domain.MapHarnessReason` 映射） | `{sub_run_id, agent, reason}` |
| `chat:subagent-error` | 子 `error` | `{sub_run_id, agent, message}` |

收尾走独立 `subagent-*` 通道而非复用 `chat:done`：复用会让前端在子 run 结束时误判父 run 完成并提前关 SSE。

## 6. 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | `/agent-profiles` | 列表 / 新建 |
| POST | `/agent-profiles/:name/enabled` · `/agent-profiles/:name/delete` | 启停 / 删除 |

## 7. 约束与取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 只回传摘要 | 父上下文恒定，不随子任务膨胀 | 子过程细节丢失（靠子工具事件在前端补可见性） |
| 工具白名单取交集 | 子 Agent 无法升权 | 档案配置过窄时子任务可能做不完整 |
| 同参去重 | 批量相同委派不重复付费 | 参数略异即视为不同任务 |
| 内置名不可覆盖 | `default` / `explore` 行为是产品契约 | 同名自定义档案注册被拒（8202） |
