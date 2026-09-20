# 04 · Agent 内核

`internal/core` 是**纯内核**：不依赖上层、不感知 HTTP，输入只有「消息 + Provider + 工具」，
输出只有「事件流 + Outcome」。任何与安全、审批、持久化相关的决策都由外部注入（中间件或钩子）完成。

## 主循环

`Loop` 由 Builder 装配：`New(provider, registry, cfg)` → `WithSink / WithMeta / WithHooks /
WithCompressor / WithCheckpoints / WithAssistantMessage / WithSteps / WithRetry / WithGuard / Expose`。

`Config`：`Model / System / MaxTurns(默认20) / Parallel(默认4) / MaxInput(压缩触发线) / MaxRunTokens /
MaxRetries / Exec{Timeout:5min, MaxResultChars:32k} / Temperature / MaxTokens / Thinking`。

一轮循环（`runLoop`）：

```
for turn := startTurn; ; turn++ {
    ctx 取消        → finish(cancelled)
    turn > MaxTurns → finish(max_turns)
    累计 token 超预算 → finish(budget_exceeded)
    hooks.BeforeTurn(可改写消息)
    MaxInput>0 → Compress → 有裁剪则发 agent.compressed
    发 agent.turn.start
    res = 请求模型（流式 + 瞬时错误重试），失败 → agent.error + finish(error)
    累计正文/思考/用量；发 agent.turn.end
    无工具调用 → FollowUp 有内容则追加并继续，否则 finish(end_turn)
    执行工具：全只读且 Parallel>1 → 并发（信号量），否则串行；结果按调用顺序回填
    写检查点；hooks.ShouldStop → finish(stopped)
}
finish → 发 agent.run.done{Reason, Usage, Turns}
```

`Resume()` 从检查点加载消息与步骤记忆，从下一轮继续，不重发 `run.start`。

**终结原因**：`end_turn / max_turns / cancelled / budget_exceeded / error / stopped`。
外部据此决定前端收尾文案与是否给「继续」入口。

## 事件

12 类 + 重试通知，统一经 `Sink.Emit(Event)` 出口（内核唯一出口）：

`agent.run.start / turn.start / turn.delta / turn.thinking / turn.end / tool.call / tool.start /
tool.result / checkpoint / compressed / error / run.done`，重试走 `agent.retry`。

`FuncSink` 把事件交给上层映射（service 侧映射为 `chat:*`）；`NopSink` 用于无人值守执行（后台任务）。

映射结果经 `service.Emitter` 发布（`Emit` 显式归属 / `EmitCtx` 从 ctx 提取）：统一注入
`run_id` + `session_id`、分配 seq 并写入重放缓冲，再广播到总线——这是 SSE 断线重放的前置条件。

## 护栏中间件链

`Middleware = func(next Handler) Handler`，`Chain(ms...)` 中**索引 0 为最外层**，最先裁决、最先短路。
装配在 `service.guardsFor`，顺序即语义：

| # | 中间件 | 裁决 |
|---|---|---|
| 1 | `ExposeGuard(allowed)` | 未暴露 / 未注册 → 拒绝 |
| 2 | `SchemaGuard()` | 参数不合规 → 带错误回填给模型修正 |
| 3 | `PolicyGuard(mode, rules, approver, pathPolicy)` | 注入检测 → 路径信任 → 风险×模式×规则 → 人工审批 |
| 4 | `hookGuard`（service） | 用户钩子的放行/询问/拦截，并把附加上下文并入回执 |
| 5 | `RepeatGuard(limit, store)` | 同参复用直接返回缓存；连续重复达上限 → 熔断 |

**拒绝契约**：`ToolResult{Refused:true, Meta["refused_reason"]}`，**不带 Err**。
拒绝是给模型的回执而非故障——模型读到原因后可以改道，而不是整轮失败。

**权限模式 × 风险**：`Mode{default, auto_edit, yolo}` 的 `Decide(risk)` 决定 `allow / ask / deny`；
`destructive` 恒 `ask`。审批门是 `Approver{Approve(ctx, call, risk) bool}`，可选实现
`ApprovalReasoner{DenyReason() string}` 让拒绝回执可解释。

**RepeatGuard**：key = 工具名 + 规范化参数（同参异序同 key）；命中缓存直接复用（不计数、不耗审批配额）；
只有成功且未被拒绝的结果才入缓存。

## 上下文装配与压缩

`Section{Key, Title, Body, Order, Priority}` 由各能力提供，`BuildSystem(sections, maxRunes)`：
按 `Order` 升序拼接，超预算时反复丢弃 `priority()` 最大者（`Priority ≤ Essential` 的常驻段永不丢），
返回被丢段的 Key/Title（前端提示"哪些上下文被裁了"）。

`RebuildHistory` 在发请求前清洗历史：剔除空 assistant 占位、剔除无对应调用的孤儿 tool 结果、
空 tool 结果补 `(empty)`。上游对「有 tool_calls 无对应结果」直接返回 400，这步必须在发请求前完成。

**压缩（MicroCompressor）是确定性折叠，不调 LLM**：先折叠最旧的「assistant(带工具调用) + 其后连续
tool 结果」整段（正文替换为占位说明，标明勿重跑），仍超预算再逐轮裁掉最旧一轮的 user 锚点；
**最后一轮永不丢**，折叠不再降低占用即停。裁剪结果写会话元数据并通知前端。

## 工具执行

`Executor(ExecOptions)` 是最内层：超时、panic 隔离、结果按 rune 截断、写入耗时标记。
并发只在「全部只读 + 数量 > 1」时启用，且**结果严格按调用顺序回填**，
保证 `assistant(tool_calls)` 与 tool 结果一一配对（顺序错位会被上游拒绝）。

## 检查点与步骤记忆

每轮工具回填后写检查点（每 run 只保留最新一轮），含消息、用量、步骤记忆；写失败只告警不阻断。
`MapSteps` 提供进程内幂等：续跑时已完成且成功的工具调用直接复用结果，**不重放副作用**。

## 委派（子 Agent）

`delegate_task` 工具触发，四重隔离：

1. **上下文**：子 run 只装人设 + 任务描述，不带父历史
2. **预算**：独立轮次、工具超时、墙钟上限
3. **工具**：父 run 已暴露 ∩ 子 Agent 策略，只能收缩不能升权
4. **正文**：只回传摘要，正文与思考不进父回答；事件只转发工具层与生命周期并打 Agent 标签

并发相同 `(agent, task)` 共享一次执行。

## 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 显式 for 循环（无图引擎） | 全部状态在一处，可读可测；单 ReAct 场景零抽象开销 | 复杂拓扑（并行分支/条件汇聚）需自行扩展 |
| 安全全在中间件链 | 加规则不改循环；每个关注点可单测、可重排 | 顺序即语义，改动链顺序需谨慎 |
| 拒绝是回执不是错误 | 模型可改道，长任务不因一次拒绝中断 | 每个工具必须正确区分「拒绝」与「失败」 |
| 确定性压缩 | 零成本零延迟、可预测、无额外失败点 | 摘要质量不如 LLM 摘要 |
| 检查点只留最新一轮 | 存储恒定、写入廉价 | 无法回放到更早的轮次 |
| 事件是唯一输出通道 | 宿主只需一个 reducer；UI/存储/审计都是订阅者 | 事件契约变更影响面广（需 domain 侧结构体收敛形状） |
