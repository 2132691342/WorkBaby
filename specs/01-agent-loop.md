# 01 · Agent 内核：流式循环

## 定位

内核是可单测的纯循环（`internal/agent`）：不知道 HTTP、数据库、桌面壳；
模型、工具、队列、闸门全部经 `Config` 注入。个人助手的复杂度下限就在这里。

## 设计

### 单层循环

```text
Run(ctx):
  for turn = 1..MaxTurns:
    msgs  = Compact(msgs, Budget)      # 超预算才动手
    emit context_usage                # 水位上涨的过程要看得见
    drain(steering) → append            # 轮间插话
    msg   = streamTurn(ctx, msgs)       # 流式，边收边发事件
    if stop ∈ {error, aborted}: 收尾返回
    append(msg)
    if stop == length: break            # 截断轮的 tool_calls 一律不执行
    if len(calls) == 0: break           # 纯文本回复 = 回合自然结束
    append(executeTools(calls))         # 按调用顺序回填
    emit turn_end
```

**为什么是单层**：外层 follow-up 循环服务的是终端 REPL（用户一直敲键，
循环停了就自动续跑）。桌面端有明确的「插话 / 排队」按钮，语义由 UI 表达；
再套一层只会让「什么时候注入」变成隐式行为。跟进消息与插话进同一个队列。

**为什么没有 PrepareNextTurn**：压缩是纯函数（清洗 + 按预算找切点），
不需要在轮间回调里做副作用。`Budget` 三个整数由 service 层备好，
`Compact` 是每次发送前的必经之路，不是可选钩子。

### 契约

```go
type Gate func(ctx context.Context, call *llm.ToolCall) (blocked bool, reason string)

type Budget struct {
    Window  int   // 模型上下文窗口
    Reserve int   // 给模型输出留的余量
    Keep    int   // 压缩后保留的近期 token 预算
}

type Config struct {
    Streamer  llm.Streamer
    Tools     []tool.Tool
    Deps      tool.Deps
    Workspace string
    System    string
    Model     string
    MaxTokens int
    MaxTurns  int
    Parallel  int
    Budget    Budget
    Gate      Gate
    Emit      func(Event)
    Steering  *Queue
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
| `tool_execution_start` | 工具开始 | `ToolCall` `ToolTitle` |
| `tool_execution_end` | 工具结束 | `ToolCall` `ToolOK` `ToolTitle` `ToolOutput` `DurationMs` |
| `turn_end` | 工具结果全部回填后 | `Turn` |
| `compressed` | 真的裁掉过东西 | `TokensBefore` `TokensAfter` |
| `context_usage` | 每轮发模型前（裁剪之后） | `TokensUsed` `TokensWindow` `Messages` |
| `agent_end` | 所有退出路径 | `StopReason` `Usage` `Turns` `Err` |

事件由 `Emit` 同步串行调用，因此**不需要下游加锁就能保证顺序**。

### 工具执行

`prepare`（串行）→ `execute`（并发或串行）→ `result`（按序回填）：

1. **prepare**：查工具 → `tool.ValidateArgs` → 过闸门。闸门拦下就当场生成一条
   `IsError` 结果，不进执行批次，但**仍占住自己的序号**。
2. **execute**：批中任一工具声明 `ExecutionSequential`，整批退化为串行；
   否则走信号量并发（上限 `Config.Parallel`，默认 4）。写类工具额外持写锁，
   避免两个写操作打同一个文件。
3. **回填**：按调用下标写回 `assistant(tool_calls)` 声明的顺序。

### 中断与异常

| 情况 | 行为 |
|---|---|
| ctx 取消 | `StopAborted`，已产出内容保留，不再执行工具 |
| 上游报错 | `StopError` + 返回 err；已产出内容照常保留 |
| 响应被截断 | `StopLength`，本轮 tool_calls 全部不执行 |
| 轮数超限 | `StopMaxTurns` |
| 同一工具同一参数重复第 3 次 | 该调用直接判失败，结果写回让模型换路 |

### 不变量

| 不变量 | 原因 |
|---|---|
| 工具结果严格按调用顺序回填 | assistant(tool_calls) 与 tool 配对错位 → 上游 400 |
| 截断轮不执行工具 | 半个 JSON 调用执行出去比不执行更危险 |
| 「报错但零产出」轮不算成功 | 否则空 assistant 落库，下一轮直接 400 |
| CleanForProtocol 输出可直接发上游 | 空消息 / 孤儿结果 / 未配对调用都在这一层兜底 |
| 事件发布永不阻塞内核 | Hub 慢客户端直接断连 + `chat:gap` 对账 |
| 工具 panic 不 recover | panic 说明有 bug；失败用 `pkg.New(4xxx, ...)` 表达 |

## 测试

`loop_test.go`：多轮工具回填顺序、并行批次按序回填、steering 注入、
ctx 取消收尾、截断轮不执行工具、重复调用被拦、超预算裁剪。
