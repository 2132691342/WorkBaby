# 01 · ReAct 主循环

`internal/core` 是**纯内核**：不依赖上层、不感知 HTTP。输入只有「消息 + Provider + 工具」，
输出只有「事件流 + Outcome」。所有与安全、审批、持久化相关的决策都由外部注入（中间件或钩子）完成。

## 1. 装配

`Loop` 由 Builder 装配：

```
New(provider, registry, cfg)
  → WithSink / WithMeta / WithHooks / WithCompressor / WithCheckpoints
  → WithAssistantMessage / WithSteps / WithRetry / Expose / WithGuard
```

**护栏必装**：`WithGuard` 未装配时 `Run / Resume` 直接返回错误（5000），不回落到裸执行器——
漏装护栏等于放行全量工具（无暴露清单、无注入检测、无审批），必须当场可见。

### Config 与默认值

| 字段 | 默认 | 说明 |
|---|---|---|
| `Model` / `System` | — | 模型名与 system 正文 |
| `MaxTurns` | **100** | 轮次上限。20 轮对「读多步 → 写 → 验证」的真实任务远远不够 |
| `Parallel` | 4 | 只读工具并行度 |
| `MaxInput` | — | 上下文 token 预算（压缩触发线）；0 表示不压缩 |
| `MaxRunTokens` | **10M** | run 累计 token 预算（成本熔断） |
| `MaxRetries` | **5** | 建流瞬时错误重试次数（不含首调） |
| `Exec` | Timeout 5min / MaxResultChars 32k | 单次工具执行参数 |
| `Temperature` / `MaxTokens` / `Thinking` | — | 采样参数 |

墙钟上限由编排层控制（`ChatService.runLLM` 默认 10 分钟，可由 Agent 定义覆盖）。

## 2. 主循环

```
for turn := startTurn; ; turn++ {
    ctx 取消           → finish(cancelled)
    turn > MaxTurns    → finish(max_turns)
    累计 token 超预算   → finish(budget_exceeded)
    hooks.BeforeTurn(可改写消息)
    MaxInput>0 → Compress → 有裁剪则发 agent.compressed
    发 agent.turn.start
    res = 请求模型（流式 + 瞬时错误重试）；失败 → agent.error + finish(error)
    累计正文 / 思考 / 用量；发 agent.turn.end
    无工具调用 → FollowUp 有内容则追加并继续，否则 finish(end_turn)
    抓取流失败且本轮零产出 → agent.error + finish(error)（有产出则按正常轮收尾）
    执行工具：全只读且 Parallel>1 → 并发（信号量），否则串行；结果按调用顺序回填
    本轮结果全部自述终局（tool.MetaTerminate）且无插话 → finish(end_turn)，不再请求模型
    写检查点；hooks.ShouldStop → finish(stopped)
}
finish → 发 agent.run.done{Reason, Usage, Turns}
```

`turns` 为本轮累计用量与分段耗时（`LLMMs` / `ToolsMs` / `CompressMs`）同步累加，供仪表盘归因。

### 终止原因

`end_turn / max_turns / cancelled / budget_exceeded / error / stopped`

外部据此决定前端收尾文案与是否给「继续」入口。

### 三个保持不变的约束

| 约束 | 原因 |
|---|---|
| 结果**严格按调用顺序回填** | `assistant(tool_calls)` 与 tool 结果必须一一配对，顺序错位会被上游拒绝 |
| 「报错但零产出」的轮不算成功 | 否则空 assistant 会被回填进历史，下一轮上游直接 400，而 run 却表现为正常收尾 |
| 延迟覆盖消费全程 | 在建流返回时就取值会漏掉整段流式生成时间，耗时归因全部失真 |

## 3. 事件

内核唯一出口是 `Sink.Emit(Event)`：

`agent.run.start / turn.start / turn.delta / turn.thinking / turn.end / tool.call / tool.start /
tool.result / checkpoint / compressed / error / run.done`，重试走 `agent.retry`。

`FuncSink` 把事件交给上层映射（service 侧映射为 `chat:*`）；`NopSink` 用于无人值守执行（后台任务）。

映射结果经 `service.Emitter` 发布（`Emit` 显式归属 / `EmitCtx` 从 ctx 提取）：统一注入
`run_id` + `session_id`、分配 seq 并写入重放缓冲，再广播到总线——这是 SSE 断线重放的前置条件。

## 4. Hooks（可选扩展点）

全部为 nil 时行为不变——扩展不侵入主循环。

| Hook | 时机与用途 |
|---|---|
| `BeforeTurn` | 每轮请求前改写消息序列（上下文裁剪、临时注入） |
| `AfterToolCall` | 工具执行后回调（落块、记忆抽取、审计） |
| `ShouldStop` | 轮结束后询问是否提前终止 |
| `Steering` | **轮间插话**：跑过工具之后、下一轮之前注入（长任务中途纠偏） |
| `FollowUp` | **收尾续接**：本轮无工具调用、run 即将收尾时注入（自动接下一波） |

`Steering` 与 `FollowUp` 共同实现「用户随时可插话」：前者在工具执行后立即生效，后者在模型准备收尾时拦截。

## 5. 工具执行

`Executor(ExecOptions)` 是最内层：超时、panic 隔离、结果按 rune 截断、写入耗时标记。
并发只在「全部只读 + 数量 > 1」时启用。

### 错误回执摘要

失败时不再原样回填 `err.Error()`——长栈会撑爆上下文。`summarizeToolError` 截断到 4 行 / 600 rune，
以 `工具执行失败: <name> -> <摘要>` 为固定首行；改道提示（见 `02-guard-chain.md`）拼到摘要末尾，
保证模型看见完整信号。

## 6. 子 Agent 委派

`delegate_task` 工具触发，四重隔离：

| 隔离维度 | 实现 |
|---|---|
| 上下文 | 子 run 只装人设 + 任务描述，不带父历史 |
| 预算 | 独立轮次、工具超时、墙钟上限 |
| 工具 | 父 run 已暴露 ∩ 子 Agent 策略，只能收缩不能升权 |
| 正文 | 只回传摘要，正文与思考不进父回答；事件只转发工具层与生命周期并打 Agent 标签 |

并发相同 `(agent, task)` 共享一次执行。

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 显式 for 循环（无图引擎） | 全部状态在一处，可读可测；单 ReAct 场景零抽象开销 | 复杂拓扑（并行分支/条件汇聚）需自行扩展 |
| 安全全在中间件链 | 加规则不改循环；每个关注点可单测、可重排 | 顺序即语义，改动链顺序需谨慎 |
| 拒绝是回执不是错误 | 模型可改道，长任务不因一次拒绝中断 | 每个工具必须正确区分「拒绝」与「失败」 |
| 事件是唯一输出通道 | 宿主只需一个 reducer；UI/存储/审计都是订阅者 | 事件契约变更影响面广 |
| 结果严格按序回填 | 上游协议配对完整 | 并行执行也需串行回填（牺牲一点尾延迟） |
