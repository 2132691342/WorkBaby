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
| `yolo` | allow | allow | allow | allow |

`destructive` 恒为 `ask`（不可逆，永不免审）。

审批门是 `Approver{Approve(ctx, call, risk) bool}`；可选实现 `ApprovalReasoner{DenyReason() string}`
让拒绝回执可解释（默认文案「用户拒绝执行」对后台任务这类无人工介入的门是误导）。

### 模式与全局设置的关系

会话级 `PermissionMode` 覆盖全局 `system_settings` 的 `agent.session_mode`。
装配时把两者合并为 `core.Mode`。

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
