# Agent 内核 · KERNEL

内核负责「让模型持续干活」这件事：管理会话树、驱动采样→工具→回灌循环、
维护转向队列、做自动重试与上下文压缩。

它不感知技能、功能、MCP 与界面的存在——只消费工具列表与系统提示词段，
只产出自己的事件枚举。

## 会话模型

- 会话 = 一个 JSONL 文件 + 内存树
- **上下文 = leaf→root 回放**：每个新条目以当前 leaf 为父；parent 链 + leaf 指针
  构成树形存储，当前没有切 leaf 的入口，实际运行恒为线性链
- 回放产物：系统提示词（service 侧组装的成品字符串）+ 消息序列

条目类型与存储格式见 `docs/SESSIONS.md`。

## 事件枚举

内核唯一出参语言，不含任何界面词汇。界面时序依赖事件顺序——改动顺序等于改界面行为。

| 事件 | 时机 |
|---|---|
| AgentStart | 每轮 prompt 开始 |
| TurnStart | 每次 provider 请求前 |
| TextDelta / ThinkingDelta | 流式增量 |
| ToolExecutionStart / ToolExecutionEnd | 工具批内（end 带 result / is_error / duration） |
| TurnEnd | 一个 turn 结束（含用量与停止原因） |
| CompactionStart / CompactionEnd | 上下文压缩前后 |
| AutoRetry | 失败重试（带退避时长） |
| AssistantFinal | 助手消息定稿（含完整块序与用量，供界面直接渲染） |
| MessageEnd | 消息定稿通知 |
| Failed | 错误终止 |
| AgentEnd | 循环终点 |
| Settled(aborted) | 完全结算（界面 done 挂这里） |

失败路径的事件顺序是 Failed → AgentEnd → Settled：错误先给出原因，
再走统一的循环终点与结算，界面只认 Settled 收尾。

## 主循环

```text
prompt(用户输入)
→ 落 user 条目 → emit AgentStart
→ loop:
    取走 steering 队列当前全部插话 → 逐条落 user 条目
    emit TurnStart → 组请求（系统提示词 + 回放上下文 + 工具 schema）
    provider.stream() → 逐事件累积内容块
    定稿 assistant 条目（含 usage / stop_reason）
    stopReason:
      toolUse → 逐个执行工具调用（参数已完整）→ 结果落盘
                → emit TurnEnd → continue
      stop   → 收尾正文 → emit TurnEnd → 跳出
→ 失败则 emit Failed → emit AgentEnd → Settled
```

**工具批按序执行**：一批里的工具逐个跑，结果按调用顺序回灌给模型。
顺序稳定比并行更重要——模型看到的顺序不随谁先跑完而变化，行为可复现。

**转向（steering）**：用户在助手跑的时候插话。主循环在每个 turn 边界把队列里
排队的**全部**插话一次取走，逐条落盘成 user 条目后合并进同一轮请求——
插话要么整批进本轮上下文，要么继续排队等下一个边界，不会被打散到多轮；
也不打断正在进行的工具调用。

## 上下文压缩

窗口快满时自动压缩，避免请求被服务端拒绝。

| 常量 | 值 | 含义 |
|---|---|---|
| RESERVE_TOKENS | 16384 | 留给「下一轮生成 + 工具结果」的空间 |
| KEEP_RECENT_TOKENS | 20000 | 选切点用的最近消息 token 预算 |

触发条件：`上下文占用 > context_window − RESERVE_TOKENS`，在每个 turn 边界
采样前检查。触发时发射 `CompactionStart`（reason=threshold）与 `CompactionEnd`。

**切点**：按 KEEP_RECENT_TOKENS 预算从最新往回累加估算 token，在消息边界选
切点，绝不把工具批拆开——切点候选跳过工具结果，落点自动上移到所属的
assistant 消息，「调用了什么」与「返回了什么」永远留在同侧。切点随
`first_kept_entry_id` 落盘进 `compaction` 条目，但**回放不读取它**。

**回放语义是全量折叠**：回放遇到最后一条 compaction 条目时，它之前的全部
历史条目直接丢弃，摘要替代一切；摘要输入也是切点无关的全量回放消息，
更早的压缩摘要同样折叠进新摘要。优势是重启后上下文连贯、摘要不层层叠加；
代价是压缩点前的消息全部进摘要，最近消息的细节可能丢失（keepRecent 预算
目前只决定落盘记录的切点，不改变回放行为）。

**摘要生成**：把回放消息压成纯文本（思考块丢弃，工具结果只取前 500 字符），
交给模型产出结构化六段摘要（目标 / 约束与偏好 / 进度 / 关键决策 / 下一步 / 关键上下文）。
这一步是一次独立的模型调用。

**手动压缩**走 `POST /api/v1/chat/compact`：service 层的独立通道，直接调用
压缩例程并发 `chat:compressed` SSE，不经内核事件；空闲会话才允许。

**token 估算**：文本按 chars/4 折算，图片按 4800 chars 计——只喂压缩决策，
不进用量统计（usage 信任服务端）。

## 自动重试

采样出错且判定可重试时（网络抖动、上游 5xx、限流），执行：

1. 指数退避（2s 起，翻倍，上限 60s）
2. 退避时长与尝试次数发 `AutoRetry` 事件，界面显示「正在重试」
3. 退避期间收到取消信号则中止

不可重试错误（鉴权失败、参数非法）直接失败，不做无谓重试。
重试上限由 `AgentConfig::max_retries` 控制（对话运行默认 3 次）。

## 子代理

子代理在独立会话文件与独立上下文中运行，用途是把「一次要扫 47 个文件」的活
从主上下文里摘出去——主上下文只看到子代理的结论，看不到中间过程。

`task` 工具在父上下文里发起一轮子代理运行：子代理拿自己的会话文件、自己的系统提示词
（角色职责 + 父提示词的工作区与技能段），跑完后把结论文本作为工具结果回灌父上下文。

| 角色 | 职责 | 工具白名单 | 上限轮次 |
|---|---|---|---|
| scout | 只读探查、汇总 | read / grep / find / ls | 16 |
| worker | 动手执行并验证 | 继承父会话 | 20 |
| reviewer | 审查既有产出 | read / grep / find / ls / bash | 12 |

白名单与父会话工具取交集——子代理不能调用父会话没有的工具。
`task` 自身从子代理的工具表里剔除，避免子代理再派子代理。

子会话文件落在 `sessions\{工作区}\subagents\`，与父会话同格式（JSONL 树形条目），
可以直接回放查看子代理的中间过程。

**过程可观测**：内核向 `SubagentConfig.observe` 回调吐 `SubagentEvent`
（Started / TextDelta / ThinkingDelta / ToolStart / ToolEnd / Finished），
service 的 `AgentBoard` 把它翻成 `agent:start` / `agent:delta` / `agent:tool_start` /
`agent:tool_end` / `agent:done` 五类 SSE 事件，发到**父会话通道**。
观测通道与上下文完全分离：事件只走 SSE 与界面状态，一行都不进父会话的请求消息。
回调为 `None` 时（测试与无界面场景）不产生任何开销。

失败时用错误码 9704（子代理启动失败），结论为空时以「（子代理没有产出结论）」占位。
轮次用尽时回退到子代理最后说的正文，不给父会话一句空话。

## 约束与取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 树形条目而非线性数组 | 分支与重试剔除需要不可变历史 | 回放要走指针 |
| 工具批按序执行而非并发 | 模型看到的顺序稳定，行为可复现 | 慢工具会拖住整批 |
| 预算切点不靠模型选 | 切点可复现，不会把工具配对拆散 | 摘要质量依赖提示词打磨 |
| 无审批闸门 | 个人单机助手，少一步流程 | bash 直跑有风险 |
| usage 信任服务端 | 估算只喂压缩决策 | 个别 provider 无 usage 时用估算兜底 |
| 停止原因归一化后交内核 | wire 协议细节不外泄 | 新 wire api 要补一张映射表 |
