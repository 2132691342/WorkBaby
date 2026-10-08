# 01 · Agent 内核：流式循环

## 定位

内核是可单测的纯循环（`backend/agent`）：不知道 HTTP、数据库、桌面壳；
模型、工具、队列、闸门全部经 `Config` 注入。个人助手的复杂度下限就在这里。

## 设计

### 单层循环

```text
Run(ctx):
  emit agent_start
  for turn = 1..MaxTurns:
    if ctx 已取消: 以 aborted 收尾
    msgs = Compact(msgs, Budget)          # 超预算才动手，纯函数
    drain(steering) → append              # 轮间插话：并进本轮上下文
    emit context_usage                    # 插话之后量，才是真正发出去的那份
    emit turn_start
    msg, stop = streamTurn(ctx, msgs)     # 流式，边收边发；预算 = Config.MaxTokens
    if stop == error: append(msg); 以 error 收尾
    if stop == length: drop(msg.tool_calls)   # 半个 JSON 不执行也不入历史
    合并 usage                            # input 取上报值与上下文实测的较大者
    append(msg)
    if 有工具调用: executeTools(calls)     # 按声明顺序回填
    emit turn_end                         # 用量随本轮一起发出：落库 append-only
    if stop ∈ {aborted, length, error}: 收尾返回
    if 无工具调用: 以 completed 收尾       # 纯文本回复 = 回合自然结束
  以 max_turns 收尾
```

顺序要点：**收尾判据一律排在 `append` / 工具执行 / `turn_end` 之后**。
否则「这一轮产出了什么」没落库就退出了，界面刷新后那半截内容凭空消失。

**为什么是单层**：外层 follow-up 循环服务的是终端 REPL（用户一直敲键，
循环停了就自动续跑）。桌面端有明确的「插话 / 排队」按钮，语义由 UI 表达；
再套一层只会让「什么时候注入」变成隐式行为。跟进消息与插话进同一个队列。

**为什么没有 PrepareNextTurn**：压缩是纯函数（清洗 + 按预算找切点），
不需要在轮间回调里做副作用。`Budget` 三个整数由 service 层备好，
`Compact` 是每次发送前的必经之路，不是可选钩子。

**为什么内核不重试截断**：带思考的模型把推理 token 也算进输出预算，推理吃满时这一轮
会「只想不说」。内核不重试——它并不知道模型能吐多少：预算是 service 层按模型上限给的，
内核再抬一次就可能超过上游真实上限，直接换回一个 400，把「能看懂的截断」变成
「看不懂的报错」。到顶就是到顶：如实以 `length` 收尾，由界面的「继续」接着写。

### 契约

```go
type Gate func(ctx context.Context, call *llm.ToolCall) (blocked bool, reason string)

type Budget struct {
    Window       int   // 模型上下文窗口
    WindowKnown  bool  // false 表示窗口是估算值，界面读数加「约」前缀
    Reserve      int   // 给模型输出留的余量
    Keep         int   // 压缩后保留的近期 token 预算
    SystemTokens int   // system 提示词的 token 估算
}

type Config struct {
    Streamer    llm.Streamer
    Tools       []tool.Tool
    Deps        tool.Deps
    Workspace   string
    System      string
    Model       string
    MaxTokens   int      // 输出预算；service 层保证非零（见 10 的采样解析）
    Temperature *float64 // nil 表示不下发，交给上游默认
    TopP        *float64
    MaxTurns    int      // 缺省 32；到顶以 StopMaxTurns 收尾
    Parallel    int      // 并发工具上限，缺省 4
    Budget      Budget
    Gate        Gate
    Emit        func(Event)
    Steering    *Queue
}

type Result struct {
    Messages   []llm.Message
    StopReason string
    Usage      llm.Usage
    Turns      int
}

func New(cfg Config, history []llm.Message) *Loop
func (l *Loop) Run(ctx context.Context) (*Result, error)
func (l *Loop) Messages() []llm.Message   // 全量上下文（含 user/assistant/tool）
```

`Emit` 为 nil 时退化成空函数；`Steering` 为 nil 时不注入插话；
`Gate` 为 nil 时工具调用直接放行。

### 事件

| EventKind | 何时发 | 载荷 |
|---|---|---|
| `agent_start` | Run 入口 | — |
| `turn_start` | 每轮发模型前 | `Turn` |
| `message_delta` | 收到正文 / 思考增量 | `DeltaKind`(text\|thinking) `Delta` |
| `tool_execution_start` | 工具开始（预执行失败的调用同样发，保证事件成对） | `ToolCall` `ToolTitle` |
| `tool_execution_end` | 工具结束 | `ToolCall` `ToolOK` `ToolBlocked` `ToolTitle` `ToolOutput` `DurationMs` |
| `steering` | 插话注入上下文时，每条一个 | `UserContent` |
| `turn_end` | 工具结果全部回填后 | `Turn` `Usage` `ContextTokens` |
| `compressed` | 真的裁掉过东西 | `TokensBefore` `TokensAfter` |
| `context_usage` | 每轮发模型前（插话之后、裁剪之后） | `TokensUsed` `TokensWindow` `TokensKnown` `TokensReserve` `Messages` |
| `agent_end` | 所有退出路径 | `StopReason` `Usage` `Turns` `Err` |

事件经 `Loop.emit` 的互斥锁串行化后同步回调：并行工具各自在 goroutine 里
发事件，锁保证「同一时刻只有一个事件在处理」，上层落库位点无需再加锁。

`steering` 事件是插话落库的时点契约：上层必须在注入时刻写 user 条目——
早于它会插进 assistant(tool_calls) 与工具结果之间撕裂协议配对，晚于它
落库链序就与真实对话不一致。错误轮没有 `turn_end`，上层负责在错误出口
把已流出的正文兜底落库（对应「已产出内容照常保留」）。

### 工具执行

`prepare`（串行）→ `execute`（并发或串行）→ `result`（按序回填）：

1. **prepare**：查工具 → `tool.ValidateArgs` → 过闸门。闸门拦下就当场生成一条
   `IsError` 结果，不进执行批次，但**仍占住自己的序号**。
2. **execute**：批中任一工具声明 `ExecutionSequential`，整批退化为串行；
   否则走信号量并发（上限 `Config.Parallel`，默认 4）。写类工具靠这条分批规则
   保证不会并发（sequential 工具永远不在并发批里），因此不需要额外的锁。
3. **回填**：按调用下标写回 `assistant(tool_calls)` 声明的顺序。

### 中断与异常

| 情况 | 行为 |
|---|---|
| ctx 取消 | `StopAborted`，已产出内容保留（判据排在工具执行之后，本轮工具因 ctx 已取消而失败） |
| 上游报错 | `StopError` + 返回 err；已产出内容照常保留 |
| 响应被截断 | `StopLength`，已写出的正文保留，本轮 tool_calls 全部不执行；界面提示可续写 |
| 轮数超限 | `StopMaxTurns` |
| 同一工具同一参数重复第 3 次 | 该调用直接判失败，结果写回让模型换路 |

### 不变量

| 不变量 | 原因 |
|---|---|
| 工具结果严格按调用顺序回填 | assistant(tool_calls) 与 tool 配对错位 → 上游 400 |
| 截断轮不执行工具、不入历史 | 半个 JSON 调用执行出去比不执行更危险；留进历史会被当成真调用回传上游 |
| 输出预算必须显式下发 | 不下发时上游各按自己的默认（常见 4096/8192），带思考的模型会「只想不说」 |
| 内核不擅自抬高输出预算 | 内核不知道模型能吐多少，抬过头就是把「能看懂的截断」换成「看不懂的 400」 |
| 「报错但零产出」轮不算成功 | 否则空 assistant 落库，下一轮直接 400 |
| CleanForProtocol 输出可直接发上游 | 空消息 / 孤儿结果 / 未配对调用都在这一层兜底 |
| 事件出口串行化 | 并行工具各自在 goroutine 里发事件，不锁则上层落库位点被并发读写 |
| 工具 panic 不 recover | panic 说明有 bug；失败用 `pkg.New(4xxx, ...)` 表达 |

事件驱动落库时，上层还要守住两条与协议配对相关的硬约束——声明必须在结果之前
合并落库、ULID 熵源必须并发安全，详见 [`07-session.md`](07-session.md) 的「落库契约」。

## 取舍

- **单层循环、无 PrepareNextTurn 挂载点**：理由见上文「设计」两节——桌面端语义由
  UI 表达，压缩是纯函数而非钩子。
- **Gate 是唯一回调面**：审批等需要挂起等用户的逻辑收成一个函数，而不是钩子集合。
  内核因此不认识审批表、不认识设置，依赖全部经 Config 注入，可以纯内存单测。
- **截断不自作主张补偿**：内核不知道模型的输出上限，唯一可靠的预算是 service
  按能力目录给的 `MaxTokens`。到顶就以 `length` 收尾，把「还能不能再写」这个
  判断交回给用户（界面上的「继续」），而不是替用户赌一次更大的预算。
- **代价**：单层循环意味着「运行中改配置」要到下一轮才生效；内核对压缩内容
  无感知，压缩质量完全取决于 `Compact` 的切点策略（见 02）。

## 测试

| 文件 | 测试 | 锁住的行为 |
|---|---|---|
| `loop_test.go` | `TestLoopProtocolOrder` | 多轮回填顺序、并行批次保序 |
| | `TestLoopStopsOnCancelLengthAndError` | 取消 / 截断（截断轮不执行工具、零产出也不重试）/ 上游报错 |
| | `TestLoopBlocksRepeatedIdenticalCall` | 重复调用拦截到上限 + 被拦调用事件成对 |
| | `TestEmitSerializesConcurrentTools` | 事件出口串行化 |
| `compact_test.go` | `TestCompactProtocol` | 清洗硬约束、压缩切点合法性 |

输出预算是否真的下发到上游由 `service/agent_test.go` 的 `TestChatSendsOutputBudget`
从装配层锁住（`llmtest.Scripted.Requests` 记下每次请求的参数）。

多轮驱动用 `backend/llm/llmtest` 的脚本替身，不联网。
