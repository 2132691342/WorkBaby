# 01 · ReAct 主循环（2 层循环）

`internal/agent` 是**纯内核**：不依赖上层、不感知 HTTP。输入只有「消息 + Provider + 工具」，
输出只有「事件流 + Outcome」。所有与安全、审批、持久化相关的决策都由外部注入（三回调 + 中间件链）完成。

## 1. 装配

`Loop` 由 Builder 装配（全部可选，零值 = 旧行为）：

```
New(provider, registry, cfg)
  → WithSink / WithMeta / WithHooks / WithCompressor / WithCheckpoints
  → WithAssistantMessage / WithSteps / WithRetry / Expose / WithGuard
  → WithPrepareNextTurn   // 压缩 + 模型/思考档 + 注入消息
  → WithGetAPIKey          // 可刷新 API key
  → WithSteeringQueue      // 轮间插话（QueueMode 队列）
  → WithFollowUpQueue      // 收尾续接（同上）
```

**护栏必装**：`WithGuard` 未装配时 `Run / Resume` 直接返回错误（5000），不回落到裸执行器——
漏装护栏等于放行全量工具（无暴露清单、无注入检测、无审批），必须当场可见。

### Config 与默认值

| 字段 | 默认 | 说明 |
|---|---|---|
| `Model` / `System` | — | 模型名与 system 正文 |
| `MaxTurns` | **100** | 轮次上限。20 轮对「读多步 → 写 → 验证」的真实任务远远不够 |
| `Parallel` | 4 | 未声明 `ExecutionMode` 的工具默认并行度；工具自己声明则忽略此值 |
| `MaxInput` | — | 上下文 token 预算（压缩触发线）；0 表示不压缩 |
| `MaxRunTokens` | **10M** | run 累计 token 预算（成本熔断） |
| `MaxRetries` | **5** | 建流瞬时错误重试次数（不含首调） |
| `Exec` | Timeout 5min / MaxResultChars 32k | 单次工具执行参数 |
| `Temperature` / `MaxTokens` / `Thinking` | — | 采样参数 |

墙钟上限由编排层控制（`ChatService.runLLM` 默认 10 分钟，可由 Agent 定义覆盖）。

## 2. 主循环（2 层）

```
runLoop(ctx, msgs, startTurn, out):
    for:                                              # 外层：收尾缝（inner 退出后到达）
        for:                                          # 内层：请求 → 工具 → 回填
            if lastTurn != nil && PrepareNextTurn != nil:
                update = PrepareNextTurn(lastTurnCtx)
                apply Model / Thinking / ExtraMessages / CompressInfo
            TransformContext（未设则 BeforeTurn）      # 每轮请求前裁剪
            Compress（WithCompressor）                 # 每轮预算折叠
            ConvertToLlm（未设则原样）                 # 协议边界
            emit turn.start
            res = requestTurn(ctx, turn, msgs)
            累计正文 / 思考 / 用量；emit turn.end
            无工具调用 → 跳出 inner
            toolMsgs, terminal = executeTools(...)
                批内任一 Sequential → 整批串行
                全部 Parallel（未声明视同）且 >1 → 信号量并发
                结果严格按调用顺序回填
            append toolMsgs
            pending = drain(steeringQueue)           # 工具执行后、检查点前插话
            append pending
            checkpoint(turn)
            if terminal && pending empty: 跳出 inner   # 有插话时不收尾，下一轮必须处理
            if ShouldStop: finish(stopped)
        # inner exit：本轮已收尾
        pending = drain(followUpQueue)
        if pending empty: finish(end_turn)
        append pending                                # 目标模式续跑 / 收尾续接统一走 follow-up
```

**关键形态**：

| 形态 | 说明 |
|---|---|
| 2 层循环 | 外层等收尾缝、内层做工具批与插话；目标模式续跑与检查点续跑统一走 follow-up |
| 三个回调 | 上下文裁剪 / 协议归一 / 切模型思考档 互不干扰 |
| 队列 + `QueueMode` | 轮间插话与收尾续接都从队列 drain，支持批量入队与可见性事件 |
| 工具自声明 `ExecutionMode` | 避免启发式误判；可声明 Sequential 强制整批串行 |

## 3. 三回调分离

| 回调 | 时机 | 职责 | 失败处理 |
|---|---|---|---|
| `Hooks.TransformContext(ctx, msgs) (msgs, err)` | 每轮请求前 | 删除空占位 / 孤儿 tool / 折叠历史 | err → finish(error) |
| `Hooks.ConvertToLlm(ctx, msgs) (msgs, err)` | 协议边界 | `AgentMessage → LlmMessage` 归一 | err → finish(error) |
| `Loop.PrepareNextTurn(ctx, lastTurnCtx) (NextTurnUpdate, err)` | 上一轮结束、下轮开始前 | 压缩 + 切换模型/思考档 + 注入消息 | err → finish(error) |

`NextTurnUpdate` 字段：

```go
type NextTurnUpdate struct {
    Model         string                  // 空 = 保持
    Thinking      *llm.ThinkingConfig     // nil = 保持
    ExtraMessages []*llm.Message          // 注入消息
    CompressInfo  *CompressInfo           // 压缩证据（与 chat:compressed 事件同源）
}
```

## 4. 事件

内核唯一出口是 `Sink.Emit(Event)`：

`agent.run.start / turn.start / turn.delta / turn.thinking / turn.end / tool.call / tool.start /
tool.result / checkpoint / compressed / error / run.done / queue.drained`，重试走 `agent.retry`。

| 事件 | 时机 |
|---|---|
| `queue.drained` | `steeringQueue` 或 `followUpQueue` 一次 drain 取出 ≥1 条消息时下发，让前端看到「用户消息已入队并被消费」 |

`FuncSink` 把事件交给上层映射（service 侧映射为 `chat:*`）；`NopSink` 用于无人值守执行（后台任务）。
映射结果经 `service.Emitter` 发布（`Emit` 显式归属 / `EmitCtx` 从 ctx 提取）：统一注入
`run_id` + `session_id`、分配 seq 并写入重放缓冲，再广播到总线——这是 SSE 断线重放的前置条件。

## 5. Hooks（可选扩展点）

全部为 nil 时行为不变——扩展不侵入主循环。

| Hook | 时机与用途 |
|---|---|
| `BeforeTurn` | 每轮请求前改写消息序列（保留兼容；新代码用 `TransformContext`） |
| `AfterToolCall` | 工具执行后回调（落块、记忆抽取、审计） |
| `ShouldStop` | 轮结束后询问是否提前终止 |
| `Steering` | 轮间插话（保留兼容；新代码用 `WithSteeringQueue`） |
| `FollowUp` | 收尾续接（保留兼容；新代码用 `WithFollowUpQueue`） |
| `TransformContext` | 每轮请求前裁剪上下文 |
| `ConvertToLlm` | 协议边界归一 |
| `PrepareNextTurn` | 压缩 + 模型/思考档 + 注入消息（挂在 `Loop` 上而非 `Hooks`） |
| `GetAPIKey` | 可刷新 API key |

## 6. 队列与 QueueMode

```go
type QueueMode string
const (
    QueueOneAtATime QueueMode = "one-at-a-time"  // 默认
    QueueAll        QueueMode = "all"
)

type MessageQueue interface {
    Drain() []*llm.Message
    Enqueue(*llm.Message)
    HasItems() bool
    Mode() QueueMode
}

type FollowUpQueue = MessageQueue // 语义别名：两个队列位是同一契约
```

- `one-at-a-time`：每次 `Drain()` 取 1 条；多条用户消息按时间序逐轮消费
- `all`：每次 `Drain()` 全量取出（多用户协同编辑 / 同步并发场景）
- 空队列返回 nil：与原 `Hooks.Steering(ctx)` 直返一致
- 两个位点各调一次：steering 在工具执行后、检查点前（见 §2）；follow-up 在 inner 退出后的收尾缝

## 7. 工具执行

`Executor(ExecOptions)` 是最内层：超时、panic 隔离、结果按 rune 截断、写入耗时标记。
并发判定完全由工具声明驱动：批内任一工具声明 `Sequential` → 整批串行；
否则 `Loop.cfg.Parallel > 1`（总闸与信号量宽度）且批内 >1 且全部为 Parallel（未声明视同 Parallel）→ 信号量并发。

### 错误回执摘要

失败时不再原样回填 `err.Error()`——长栈会撑爆上下文。`summarizeToolError` 截断到 4 行 / 600 rune，
以 `工具执行失败: <name> -> <摘要>` 为固定首行；改道提示（见 `02-guard-chain.md`）拼到摘要末尾，
保证模型看见完整信号。

## 8. 子 Agent 委派

`delegate_task` 工具触发，四重隔离：

| 隔离维度 | 实现 |
|---|---|
| 上下文 | 子 run 只装人设 + 任务描述，不带父历史 |
| 预算 | 独立轮次、工具超时、墙钟上限 |
| 工具 | 父 run 已暴露 ∩ 子 Agent 策略，只能收缩不能升权 |
| 正文 | 只回传摘要，正文与思考不进父回答；事件只转发工具层与生命周期并打 Agent 标签 |

并发相同 `(agent, task)` 共享一次执行。

## 9. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 显式 for 循环（无图引擎）| 全部状态在一处，可读可测；单 ReAct 场景零抽象开销 | 复杂拓扑（并行分支/条件汇聚）需自行扩展 |
| 2 层循环（vs 单层） | 收尾缝、目标模式续跑、检查点续跑统一走外层 | 多一层嵌套，单测需覆盖 inner/outer 边界 |
| 三回调分离 | 关注点独立可测；PrepareNextTurn 是单一变更点 | 三个 nil 判空分支 |
| 队列 vs 直返 | 支持批量入队；UI 可显式感知「已排队」 | 队列空时无消息，兼容旧 hook 需包装 |
| 工具 ExecutionMode | 工具自己声明；避免启发式误判 | 可选接口，未实现默认 Parallel——有副作用的工具必须显式声明 Sequential |
| 安全全在中间件链 | 加规则不改循环；每个关注点可单测、可重排 | 顺序即语义，改动链顺序需谨慎 |
| 拒绝是回执不是错误 | 模型可改道，长任务不因一次拒绝中断 | 每个工具必须正确区分「拒绝」与「失败」|
| 事件是唯一输出通道 | 宿主只需一个 reducer；UI/存储/审计都是订阅者 | 事件契约变更影响面广 |
| 结果严格按序回填 | 上游协议配对完整 | 并行执行也需串行回填（牺牲一点尾延迟）|
