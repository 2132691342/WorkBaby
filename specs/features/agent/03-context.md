# 03 · 上下文装配与压缩

上下文管理分三段：**装配**（把各来源拼成 system）、**清洗**（发请求前修正消息序列）、**压缩**（超预算时折叠历史）。

## 1. 装配：Section 模型

```go
type Section struct {
	Key      string // 语义标识（裁剪回传用）；空则用 Title
	Title    string
	Body     string
	Order    int // 拼接顺序：小者先
	Priority int // 裁剪优先级：大者先丢；0 = 取 Order
}
```

各能力（Preload 通道）提供自己的 Section，`BuildSystem(sections, maxRunes)` 统一装配。

### 拼接顺序

| Order | 段 | 说明 |
|---|---|---|
| 10 | `Persona` | 人格与方法论（环境能力产出） |
| 20 | `Env` | 环境（时间、平台、工具约定） |
| 30 | `Workspace` | 工作区路径与绑定信息 |
| 40 | `Memory` | 本会话长期记忆召回 |
| 50 | `Knowledge` | 知识库召回 |
| 60 | `Skill` | 命中技能的方法论注入（未命中时为 `SkillIndex` 索引段） |

各能力显式声明 `agent.Order*` 常量；注册序（`Registry.Register(c, order)`）仅作未显式声明段的兜底。

**人格与环境最先，重型召回段最后**——越靠前的段越稳定，越靠后的段越可能被裁剪。

### 裁剪策略

超预算时**按 Priority 降序反复丢弃**，直到装得下：

| Priority | 含义 |
|---|---|
| ≤ 10（`PriorityEssential`） | **常驻**：预算再紧也不丢 |
| 30 `PriorityHigh` | 高（如工作区信息） |
| 50 `PriorityMedium` | 中（默认档） |
| 70 / 80 `PriorityLow` / `Lowest` | 低（召回类段） |

未显式声明 Priority 时取 `Order`（拼接序即重要性序）。

**被丢段必须回传**（`BuildSystem` 返回 `dropped []string`，标识优先取 `Key`）——
否则「上下文里少了什么」不可解释。前端据此显示 `chat:context-trimmed`。

## 2. 清洗：RebuildHistory

发请求前对消息序列做三项修正：

| 问题 | 处理 | 为什么 |
|---|---|---|
| 空 assistant 占位 | 剔除 | 空 content + 无 tool_calls 的 assistant 是脏数据 |
| 孤儿 tool 结果（无对应 tool_call） | 剔除 | 上游对「有 tool_result 无对应 tool_calls」直接返回 400 |
| 空 tool 结果 | 补 `(empty)` 占位 | 剔除会让 `tool_calls` 失去配对，只能补占位 |

这三项必须在**发请求前**完成：上游 400 的代价是整轮失败，而清洗是纯本地操作。

## 3. 压缩：MicroCompressor

**确定性折叠，不调 LLM**：零成本、零延迟、可预测，压缩本身不引入额外失败点。

### 折叠顺序

1. **折叠整段**：从最旧的「assistant(带工具调用) + 其后连续 tool 结果」开始，
   把这一整段替换为占位说明（标明「此处已折叠，勿重跑」）——必须整段折叠，
   否则拆散 assistant+tool 对会被上游拒绝
2. **裁 user 锚点**：仍超预算时，逐轮裁掉最旧一轮的 user 锚点
3. **停止条件**：最后一轮**永不丢**；折叠不再降低占用即停

### 结果通知

裁剪结果写入会话元数据（`compressBoundaryMeta`：`removed_msgs / cutoff_at / summary / filter_key`），
并发事件：

| 事件 | 载荷 |
|---|---|
| `chat:compressed` | `removed_messages` / `summary` / `truncated` / `filter_key`（压缩器标识，恒为 `"micro"`）/ `cutoff_at` |

压缩是上下文被改写的少数时刻——不发事件用户只会觉得「前面的聊天不见了」。

## 4. 上下文占用透视

`GET /chat/sessions/:id/usage/context` 返回分段占用，与真实请求口径同源：
走同一套 `Section` 装配与同一套裁剪逻辑，保证「看到的占用」等于「实发的占用」。

前端 `ContextRing.vue` 展示环形占比。

## 5. 预算来源

压缩触发线 `MaxInput` 不是静态值：按「模型上下文窗口 × 压缩比例」计算。

```
window = modelContextWindow(provider, model)
budget = window × compressRatio（全局设置）
```

小窗口模型不会撑爆，大窗口模型不会过早压缩。

## 6. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 确定性折叠（不调 LLM） | 零成本零延迟、可预测、无额外失败点 | 摘要质量低于 LLM 摘要 |
| 五档优先级 + 常驻段 | 裁剪可解释、可控 | 需要每个能力正确声明 Priority |
| 被丢段回传前端 | 「少了什么」可解释 | 多一个事件与前端展示 |
| 整段折叠（不拆对） | 满足上游协议 | 折叠粒度粗，可能一次丢掉较多内容 |
| 最后一轮永不丢 | 当前任务上下文始终完整 | 极端情况下仍可能超预算（此时由上游报错兜底） |

## 7. 三回调边界（PI Phase 3）

`specs/features/agent/01-react-loop.md §3` 列了三回调，本节细化 `TransformContext` 与 `ConvertToLlm` 的边界，
避免在 service 层错位装配。

### 7.1 TransformContext vs ConvertToLlm

| 维度 | TransformContext | ConvertToLlm |
|---|---|---|
| 作用对象 | `[]*llm.Message`（最终要发的序列） | `[]*llm.Message`（协议边界上的同一序列） |
| 调用时机 | 每轮请求**之前**（Compressor 之前） | 每轮请求**之前**（Compressor 之后、发送前一刻） |
| 失败语义 | err → `finish(error)` | err → `finish(error)` |
| 编排层现状 | **未注入**：清洗在 `Run` 前一次性 `RebuildHistory` 完成，压缩经 `WithCompressor` 在同一位点每轮生效 | **未注入**：内核按 role 过滤的缺省足够 |
| 调用次数 | 每轮 1 次（已设时） | 每轮 1 次（已设时） |
| 谁负责 | 编排层需要 per-turn 裁剪（如按会话状态折叠）时经 `Hooks.TransformContext` 注入 | 某协议需要特殊字段（如 Anthropic `cache_control`）时经 `Hooks.ConvertToLlm` 注入 |

**为什么分开**：旧实现把「裁剪上下文」与「协议归一」挤在 `BeforeTurn` 一个回调里。
当某个 Provider 出现新字段（比如 Anthropic `cache_control`）时，协议归一回调可单独
打补丁而不影响压缩逻辑；反之亦然。

### 7.2 PrepareNextTurn 的特殊位

`PrepareNextTurn` 挂在 `Loop` 上（不在 `Hooks{}` 里），因为它返回的是
**结构化更新**（`NextTurnUpdate`）而不是消息序列：

```go
type NextTurnUpdate struct {
    Model         string                  // 空 = 保持
    Thinking      *llm.ThinkingConfig     // nil = 保持
    ExtraMessages []*llm.Message          // 注入消息
    CompressInfo  *CompressInfo           // 压缩证据
}
```

`CompressInfo` 走 PrepareNextTurn 而不是 TransformContext，是因为压缩会改变 `Loop.cfg.Model`
或 `Loop.cfg.Thinking`（如「压缩后切到更便宜的模型」），属于「下一轮准备」而非「本轮裁剪」。

### 7.3 装配（现状）

```go
loop := agent.New(provider, reg, cfg).
    WithSink(sink).
    WithMeta(...).
    WithAssistantMessage(...).
    WithCompressor(agent.MicroCompressor{}).   // 每轮请求前的预算折叠（内核 TransformContext 位点之后调用）
    WithHooks(Hooks{Steering: ..., FollowUp: ...}).
    Expose(toolNames).
    WithGuard(ExposeGuard, SchemaGuard, PolicyGuard, hookGuard, AdaptiveLoopGuard, RepeatGuard).
    WithRetry(policy, onAttempt)

// Run 前一次性清洗（空占位 / 孤儿 tool 会让上游 400）：
res, runErr = loop.Run(ctx, agent.RebuildHistory(llmMsgs))
```

**注意点**：
- `TransformContext` / `ConvertToLlm` 内核已支持、编排层暂未注入：清洗是 run 前一次性 `RebuildHistory`，
  per-turn 压缩由 `WithCompressor` 承担（见 §3）。出现 per-turn 裁剪或协议补丁需求时再注入，形态不变。
- 旧 `BeforeTurn` 钩子仍生效，但**与 TransformContext 二选一**：若两者都设置，`TransformContext` 优先。
