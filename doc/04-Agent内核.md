# 04 · Agent 内核

`internal/core` 是纯内核：不依赖上层，不感知 HTTP，输入输出只有「消息 + Provider + 工具」与「事件流 + Outcome」。

## 主循环

`Loop` 由 Builder 装配：`New(provider, registry, cfg)` → `WithSink / WithMeta / WithHooks / WithCompressor / WithCheckpoints / WithAssistantMessage / WithSteps / WithRetry / WithGuard / Expose`。

`Config`：`Model / System / MaxTurns(默认20) / Parallel(默认4) / MaxInput(压缩触发线) / MaxRunTokens / MaxRetries / Exec{Timeout:5min, MaxResultChars:32k} / Temperature / MaxTokens / Thinking`

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

`Resume()` 从检查点加载消息与步骤记忆，从下一轮继续，不发 `run.start`。

**Reason**：`end_turn / max_turns / cancelled / budget_exceeded / error / stopped`

## 事件

12 类 + 重试通知，统一经 `Sink.Emit(Event)` 出口（内核唯一出口）：

`agent.run.start / turn.start / turn.delta / turn.thinking / turn.end / tool.call / tool.start / tool.result / checkpoint / compressed / error / run.done`，重试走 `agent.retry`。

`FuncSink` 把事件交给上层映射（service 侧映射为 `chat:*`）；`NopSink` 用于无人值守执行（后台任务）。

## 护栏中间件链

`Middleware = func(next Handler) Handler`，`Chain(ms...)` 中**索引 0 为最外层**，最先裁决、最先短路。装配在 `service.guardsFor`，顺序即语义：

| # | 中间件 | 裁决 |
|---|---|---|
| 1 | `ExposeGuard(allowed)` | 未暴露 / 未注册 → 拒绝 |
| 2 | `SchemaGuard()` | 参数不合规 → 带错误回填给模型修正 |
| 3 | `PolicyGuard(mode, rules, approver, pathPolicy)` | 注入检测 → 路径信任 → 风险×模式×规则 → 人工审批 |
| 4 | `hookGuard`（service） | 用户钩子的放行/询问/拦截，并把附加上下文并入回执 |
| 5 | `RepeatGuard(limit, store)` | 同参复用直接返回缓存；连续重复达上限 → 熔断 |

**拒绝契约**：`ToolResult{Refused:true, Meta["refused_reason"]}`，**不带 Err**。拒绝是给模型的回执而非故障——模型读到原因后可以改道。

**权限模式 × 风险**：`Mode{default, auto_edit, yolo}` 的 `Decide(risk)` 决定 `allow / ask / deny`；`destructive` 恒 `ask`。`allowRules` 是 default 模式下显式放行的工具清单（放行不等于无护栏：命令级风险仍由 `RiskClassifier` 裁决）。

**审批门**：`Approver{Approve(ctx, call, risk) bool}`；可选实现 `ApprovalReasoner{DenyReason() string}` 让拒绝回执可解释。聊天 run 用阻塞等待用户决策的门，后台任务用只认免审授权的门（见 [10-后台任务](10-后台任务.md)）。

**路径信任**：只对 `exec.cwd` 生效，配合计划模式限制写入范围。

**RepeatGuard 细节**：key = 工具名 + 规范化参数（同参异序同 key）；命中缓存直接复用（不计数、不耗审批配额）；只有成功且未被拒绝的结果才入缓存；默认上限 3。

## 上下文装配与清洗

`Section{Key, Title, Body, Order, Priority}` 由各能力提供，`BuildSystem(sections, maxRunes)`：按 `Order` 升序拼接，超预算时反复丢弃 `priority()` 最大者（`Priority ≤ Essential` 的常驻段永不丢），返回被丢段的 Key/Title 列表（前端提示）。

`Order`：人格 10 / 环境 20 / 工作区 30 / 记忆 40 / 知识 50 / 技能 60 / 待办 70（service 侧另有沙箱、压缩提示等附加段）。

`RebuildHistory` 在发请求前清洗历史：剔除空 assistant 占位、剔除无对应调用的孤儿 tool 结果、空 tool 结果补 `(empty)`。上游对「有 tool_calls 无对应结果」直接返回 400，这步必须在发请求前完成。

## 压缩

`MicroCompressor` 是确定性折叠，不调 LLM：先折叠最旧的「assistant(带工具调用) + 其后连续 tool 结果」整段（正文替换为占位说明，标明勿重跑），仍超预算再逐轮裁掉最旧一轮的 user 锚点；**最后一轮永不丢**，折叠不再降低占用即停。裁剪结果写会话元数据并通知前端。

## 工具执行

`Executor(ExecOptions)` 是最内层：超时、panic 隔离、结果按 rune 截断、写入耗时标记。并发只在「全部只读 + 数量 > 1」时启用，且**结果严格按调用顺序回填**，保证 `assistant(tool_calls)` 与 tool 结果一一配对。

## 检查点与步骤记忆

每轮工具回填后写检查点（每 run 只保留最新一轮），含消息、用量、步骤记忆；写失败只告警不阻断。`MapSteps` 提供进程内幂等：续跑时已完成且成功的工具调用直接复用结果。

## 委派（子 Agent）

`delegate_task` 工具触发，四重隔离：

1. **上下文**：子 run 只装人设 + 任务描述，不带父历史
2. **预算**：独立轮次（12）、工具超时（2min）、墙钟（3min）
3. **工具**：父 run 已暴露 ∩ 子 Agent 策略，只能收缩不能升权
4. **正文**：只回传摘要（4000 rune），正文与思考不进父回答；事件只转发工具层与生命周期并打 Agent 标签

并发相同 `(agent, task)` 共享一次执行。用量按 `delegate` 来源单独落账。
