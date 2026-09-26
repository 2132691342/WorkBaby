# 02 · 护栏中间件链

安全与审批**全部**通过中间件链实现，主循环里没有任何 `if` 分支。

## 1. 契约

```go
type Handler    func(ctx context.Context, call Call) tool.ToolResult
type Middleware func(next Handler) Handler
func Chain(ms ...Middleware) Middleware   // 索引 0 为最外层：最先裁决、最先短路
```

**拒绝契约**：`ToolResult{Refused:true, Meta["refused_reason"]}`，**不带 Err**。
拒绝是给模型的回执而非故障——模型读到原因后可以改道，而不是整轮失败。

## 2. 链顺序（顺序即语义）

装配在 `service.guardsFor`：

| # | 中间件 | 裁决 |
|---|---|---|
| 1 | `ExposeGuard(allowed)` | 未暴露 / 未注册 → 拒绝 |
| 2 | `SchemaGuard()` | 参数不合规 → 带错误回填给模型修正 |
| 3 | `PolicyGuard(mode, rules, approver, pathPolicy)` | 注入检测 → 路径信任 → 风险×模式×规则 → 人工审批 |
| 4 | `hookGuard`（service） | 用户钩子的放行 / 询问 / 拦截，并把附加上下文并入回执 |
| 5 | `AdaptiveLoopGuard(threshold)` | 同工具连续失败达阈值 → 追加改道提示 |
| 6 | `RepeatGuard(limit, store)` | 同参复用直接返回缓存；连续重复达上限 → 熔断 |

排在后面的原因：**前面的拒绝（未暴露 / 越权）不算「重复调用」或「失败」**，
否则护栏拒绝会污染熔断计数与失败计数。

## 3. PolicyGuard：一站式安全门

四步依次裁决，任一步不通过即短路：

| 步 | 检查 | 拒绝原因码 |
|---|---|---|
| 1 | 参数含伪工具调用标记（`<tool_call>` / `<tool_result>` / `im_start` 等） | `prompt_injection` |
| 2 | 路径信任 + 计划模式预检（`PathPolicy`） | `path_trust` |
| 3 | 风险 × 模式 × 规则 | `policy` |
| 4 | 需审批时问人工审批门 | `approval` |

已由上层标记 `Trusted` 的调用直接放行（消除双层重复询问）。

### 权限模式 × 风险

| 模式 | readonly | write_local | exec / network | destructive |
|---|---|---|---|---|
| `default` | allow | ask | ask | ask |
| `auto_edit` | allow | allow | ask | ask |
| `yolo` | allow | allow | allow | **ask** |

`destructive` 恒为 `ask`（不可逆，永不免审）。

审批门是 `Approver{Approve(ctx, call, risk) bool}`；可选实现 `ApprovalReasoner{DenyReason() string}`
让拒绝回执可解释（默认文案「用户拒绝执行」对后台任务这类无人工介入的门是误导）。

### 模式与全局设置的关系

会话级 `PermissionMode` 覆盖全局 `system_settings` 的 `agent.session_mode`。
装配时把两者合并为 `agent.Mode`。

## 4. AdaptiveLoopGuard：失败改道

与 `RepeatGuard` 维度互补：

| 中间件 | 维度 | 防什么 |
|---|---|---|
| `RepeatGuard` | 工具名 + 规范化参数（同参异序同 key） | 重复副作用 |
| `AdaptiveLoopGuard` | **工具名** | 死磕同一种工具 |

失败判定只看 `res.Err != nil`；**拒绝不计入失败计数**（拒绝是策略回执不是故障）。

三档行为：

| 连续失败次数 | 行为 |
|---|---|
| 1 ~ threshold-1 | 仅把次数写进 `Meta["same_failure_count"]`，正文不变 |
| ≥ threshold（默认 3） | 正文末尾追加 `[guard] 工具 X 已连续失败 N 次。请换一种方式：…`，并在 `Meta["adaptive_hint"]="1"` 打标 |
| 任意一次成功 | 计数清零 |

改道文案按**风险档**给出候选路径：

| 风险档 | 建议 |
|---|---|
| readonly | 改用 `file_read` / `file_grep` / `file_glob` 直接探查；`doc_reader` 抽正文；`delegate_task` 派 explore 子 Agent；资料缺失时向用户说明 |
| exec | 检查命令参数（路径 / 引号 / 工作目录）；`file_list` 先确认目标存在；仍失败则报告症状（错误码 + 关键 stderr 一行） |
| network | 检查 URL / 参数格式；改用 `webfetch` 直拉正文；多次失败后向用户报告 |
| 其他 | 核对入参；用相关只读工具先验证前置条件 |

**与错误摘要的协作**：`runOne` 在失败时用 `summarizeToolError` 重建正文，
必须先把 `AdaptiveLoopGuard` 附加的 `[guard]` 段抽离再拼回——否则摘要会盖掉改道提示（fail-open 的关键）。

## 5. RepeatGuard：同参熔断与幂等复用

key = 工具名 + 规范化参数（JSON 重新序列化，同参异序判同一 key）。

执行顺序：**复用 → 计数 → 检查 → 执行 → 记录**。

| 步 | 行为 |
|---|---|
| 1 复用 | `StepStore.Load(key)` 命中直接返回缓存（不计数、不耗审批配额）——续跑/重发场景不重放副作用 |
| 2 计数 | streak（连续同 key 次数）与 total（累计次数）双计数 |
| 3 检查 | `streak >= limit` 或 `total > limit` → 熔断拒绝（`loop_guard`） |
| 4 记录 | 只有**成功且未被拒绝**的结果才入缓存（失败与拒绝的副作用不可重用） |

## 6. 用户钩子（hookGuard）

在策略门之后：内置护栏先裁决，钩子只处理「本机护栏已放行、但用户另有规矩」的场景。

| 钩子决策 | 行为 |
|---|---|
| `deny` | 转成拒绝回执（`policy`），理由取钩子给的 message |
| `ask` | 升级为人工确认；未批准则拒绝（`approval`） |
| 放行 | 执行后把 `PostToolUse` 的附加上下文并入回执 |

钩子自身故障不阻断（`UserHookService` 内部兜底放行）。

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 拒绝 = 结构化回执 | 模型可改道，长任务不中断 | 每个工具必须正确区分「拒绝」与「失败」 |
| 链式组合而非循环内分支 | 加规则不改主循环；每个关注点可单测 | 顺序即语义，重排需谨慎 |
| 两个互补防护（同参 / 同工具） | 分别覆盖「重复副作用」与「死磕工具」 | 模型可能因熔断而提前收尾（需改道提示配合） |
| 权限四档 + destructive 恒需审批 | 不可逆操作永不免审 | 首次使用有学习成本 |
| 钩子在策略门之后 | 内置安全不被用户配置削弱 | 用户无法用钩子放宽内置护栏 |

## 8. 为何保留 middleware 链 vs PI 的单一回调

PI `pi-agent-core` 的安全门是「`BeforeToolCall` / `AfterToolCall`」两个回调；
本工程保留 **6 层 middleware 链** 是有意为之的偏离（见 `AGENTS.md §9.7.2`）。

### 8.1 单一回调的合并压力

把 6 维度（Expose / Schema / Policy / Approval / Adaptive / Repeat）塞进 1 个回调：

```ts
// PI 风格（伪）
beforeToolCall(ctx) {
    if (!allowed(ctx.toolCall)) return block(...)
    if (!validArgs(ctx.args)) return block(...)
    if (!pathTrusted(ctx)) return block(...)
    if (!mode.allows(ctx.risk)) {
        const ok = await approver.approve(...)
        if (!ok) return block(...)
    }
    if (hook.deny) return block(...)
    if (await hook.ask && !userApprove) return block(...)
}
```

带来的问题：

| 问题 | 后果 |
|---|---|
| 顺序即语义但分散在 if 链 | 调换「先问审批 vs 先查路径」会让模型看到不同的拒绝原因 |
| 短路的早返点散落 | 新增规则时容易漏改一处 |
| 每个维度无法单独 mock 单测 | 必须把 6 维度连起来构造 fixture |
| 「拒绝原因码」分散维护 | 新增维度时容易与已有 6 个 reason 撞名 |

### 8.2 middleware 链的优势

```go
// 本工程
Chain(ExposeGuard, SchemaGuard, PolicyGuard, hookGuard, AdaptiveLoopGuard, RepeatGuard)(base)
```

- 每个中间件只负责一个维度，`func TestXxxGuard` 单测零依赖
- 短路靠 `return refuse(...)`，新增维度 = 插入一个新 `Middleware`，零侵入
- 顺序在 `service.guardsFor` 一处集中，改顺序=改一个 slice
- 拒绝原因码（`policy` / `approval` / `path_trust` / `prompt_injection` / `loop_guard`）与中间件一一对应

### 8.3 与 PI 的兼容性

- 链路最外层 `ExposeGuard` 的语义等价于 PI 的工具暴露检查
- `PolicyGuard` 内的「风险×模式×规则」+ `Approver` 等价于 PI 的 `beforeToolCall`
- `hookGuard` 用 `AfterToolCall` 链路回调等价于 PI 的 `afterToolCall` 注入附加上下文

如果未来需要可扩展性（用户注册自定义护栏），本形态用「`Hook{AfterToolCall}` + middleware 注册表」
扩展即可，不必退回到单一回调。

### 8.4 迁移到单一回调的代价

| 项 | 现状（middleware）| 假设迁移（单一回调） |
|---|---|---|
| 6 个 `TestXxxGuard` | 6 文件 | 拆 6 内部函数 + 1 顶层 orchestrator 测试 |
| 新增「OCR 注入检测」中间件 | 1 文件 + 1 注册 | 改 orchestrator + 加 reason 码分支 |
| 审批门注入 | `Approver` interface | 仍 interface，但 orchestrator 顶层要管 |
| 钩子 veto 与审批门互斥 | 显式分层 | 需 if-else 分支 |

**判定**：保留 middleware 链是当前复杂度的最优解。如未来「安全维度 ≥10」或「用户可注册护栏」需求出现，再迁移到带优先级的中间件图。
