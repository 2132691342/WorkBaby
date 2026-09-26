# 04 · 检查点与恢复

## 1. 检查点

每轮工具回填后写检查点，**同一 run 只保留最新一轮**。

```go
type Checkpoint struct {
	RunID          string
	SessionID      string
	Turn           int
	AssistantMsgID string            // 续跑落回同一条 assistant 消息，不新开
	Messages       []*llm.Message    // 完整消息序列（含 system）
	Content        string            // 已累积正文
	Thinking       string            // 已累积推理
	Usage          llm.TokenUsage    // 跨段累计用量
	Steps          map[string]string // 幂等步骤记忆：tool:{name}|{args} → 结果
	CreatedAt      int64
}
```

`CheckpointStore` 三方法：`Append` / `LoadLast` / `Cleanup(sessionID, keep)`。

| 设计点 | 原因 |
|---|---|
| 只留最新一轮 | 每轮都留会让长 run 的存储放大成 N× 全文，而 `Resume` 只认最后一轮 |
| 完整消息序列（含 system） | 少了 system 段，续跑后模型失去人设与工具约定 |
| 写失败只告警不阻断 | 检查点是旁路能力，不能因为它写不进去就让 run 失败 |
| 清理按 run 粒度 | `Cleanup(sessionID, keep)` 保留最近 N 个 run，更早的整 run 删除 |

## 2. Resume（续跑）

```
Resume(ctx)
  → LoadLast(runID)              // 无检查点 → 5004
  → NewMapSteps(cp.Steps)        // 复用已完成调用的记忆
  → runLoop(cp.Messages, cp.Turn+1, out{Usage/Content/Thinking 从检查点继承}, emitStart=false)
```

三个关键语义：

| 语义 | 说明 |
|---|---|
| **不重发 run 起始事件** | 「本次是续跑」由调用方标注（编排层补发 `chat:stream.start{resumed:true}`） |
| **用量跨段累计** | 续跑不是新的计费周期，`Usage` 从检查点继承后继续累加 |
| **复用已完成调用** | `Steps` 恢复后，`RepeatGuard` 命中缓存直接返回结果，**不重放副作用**（复用键 = 检查点恢复的记录；本 run 内新记结果仅随检查点持久化，不在本 run 内复用，防止读到旧值） |

## 3. 步骤记忆（MapSteps）

进程内幂等：`tool:{name}|{args}` → 结果。

```
MapSteps.Load(key)   // 只命中检查点恢复的键
MapSteps.Store(key, content)  // 本 run 新记：随检查点持久化，不在本 run 内复用
Snapshot() map[string]string  // 全量快照，写入检查点
```

快照随检查点落库，续跑时从检查点恢复。这让「已经执行过的写操作」不会因为续跑而重复发生；
恢复键以外的调用本 run 内照常执行（模型「写后重读」不会拿到旧值）。

与 `RepeatGuard` 的分工：`MapSteps` 是存储，`RepeatGuard` 是使用它的中间件——
装配时经 `Loop.StepsStore()` 传入（`RepeatGuard(3, loop.StepsStore())`），
适配器每次调用解引用 `l.steps`，兼容 `Resume` 整体重建步骤记忆。

## 4. 执行平面

`ExecutionRegistry` 跟踪活动 run 的状态，供前端与运维观测：

| 字段 | 值 |
|---|---|
| `Scope` | `chat_turn` / `task` / `tool_only` / `delegate` |
| `State` | `planning` / `running` / `paused` / `waiting_input` / `completed` / `failed` / `cancelled` |

终态映射：`cancelled → StateCancelled`；`error → StateFailed`；其余 → `StateCompleted`。

## 5. 跨重启恢复

应用启动时的三类恢复动作（`api.Handler.Startup` 第 15 步）：

| 动作 | 作用 |
|---|---|
| `RearmPending` | 未决审批记录重新武装：窗口延长到 24h，重启后决策卡仍可见、可决策 |
| `LoadGrants` | 装载持久化免审授权（「本会话允许」跨重启生效） |
| `ReapInterrupted` | 崩溃时卡在 `streaming` 的 assistant 消息标记为 `failed` + `interrupted` |

### 审批跨重启闭环

```
进程内挂起（Approve 阻塞等待）
  → 未决记录落库
  → 【重启】新内存态 + 同一份库
  → RearmPending → Pending() 仍返回该卡 → Decide()
  → 触发 ResumeHook(runID) → ResumeRun 从检查点续跑
  → 续跑重放同一调用（带 resumed 标记）→ 已决记录快速放行并置 consumed
```

### 一次性消费与授权回滚

| 机制 | 行为 |
|---|---|
| 一次性消费 | 已决记录被消费后置 `consumed`，防止一次性审批被静默复用（同命令再次调用必须重新询问） |
| 授权回滚 | run 以 `error` / `cancelled` 收尾时，回滚该 run 扩出的「本会话允许」授权（未验证的工作不保留免审） |
| 不可逆永不免审 | `destructive` 风险即便选了「本会话允许」也每次必问 |

## 6. 残段清理

取消或崩溃会留下「assistant 发起了 tool_calls 但只有部分结果落库」的残段。
`toLLMMessages` 在重建上下文时处理：

| 情况 | 处理 |
|---|---|
| 有部分结果的 tool_calls | 保留有结果的那些，剥离悬挂的（`ToolCalls` 只留配对成功的） |
| 有正文的残段 | 保留半截正文（中断续聊时叙事不断裂） |
| 全悬挂且无正文 | 整体剥掉（等价空占位） |

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 检查点只留最新一轮 | 存储恒定、写入廉价、不随长 run 膨胀 | 无法回放到更早轮次 |
| 每轮工具回填后写 | 崩溃/中断的续跑位点精确到轮 | 每轮一次写（SQLite 单连接下为串行写） |
| 写失败只告警 | 检查点不影响主链路可用性 | 极端情况下丢失续跑能力（有日志） |
| 步骤记忆随检查点落库 | 续跑不重放副作用 | 存储随工具调用数增长（有界于单 run） |
| 审批持久化 + consumed 标记 | 跨重启不丢决策；一次性审批不被复用 | 多一张表与一套恢复逻辑 |
| 残段保留半截正文 | 中断续聊叙事不断裂 | 需精确的配对判定（写错会导致上游 400） |
