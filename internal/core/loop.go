package core

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 终止原因：统一出口，前端据此决定是否给「继续」入口。
const (
	ReasonEndTurn   = "end_turn"
	ReasonMaxTurns  = "max_turns"
	ReasonCancelled = "cancelled"
	ReasonBudget    = "budget_exceeded"
	ReasonError     = "error"
	ReasonStopped   = "stopped" // 钩子要求停止
)

// Config run 级配置；零值字段在 New 里补默认。
type Config struct {
	Model        string
	System       string
	MaxTurns     int // 轮次上限；0 取默认 20
	Parallel     int // 只读工具并行度；0 取默认 4
	MaxInput     int // 上下文 token 预算（压缩触发线）；0 表示不压缩
	MaxRunTokens int // run 累计 token 预算（成本熔断）；<=0 不限
	MaxRetries   int // 建流瞬时错误最大重试次数；0 取默认 3
	Exec         ExecOptions
	Temperature  *float64
	MaxTokens    *int
	Thinking     *llm.ThinkingConfig
}

func (c Config) withDefaults() Config {
	if c.MaxTurns <= 0 {
		c.MaxTurns = 20
	}
	if c.Parallel <= 0 {
		c.Parallel = 4
	}
	return c
}

// RequestParams 单次 run 的请求级采样参数，优先级高于全局默认。
type RequestParams struct {
	Temperature *float64
	Thinking    *llm.ThinkingConfig
}

// Meta run 身份信息，注入每个事件。
type Meta struct {
	RunID       string
	SessionID   string
	ParentRunID string
	Agent       string
}

// Hooks 可选扩展点。全部为 nil 时行为不变——扩展不得侵入主循环。
type Hooks struct {
	// BeforeTurn 每轮请求前改写消息序列（上下文裁剪、临时注入）。
	BeforeTurn func(ctx context.Context, turn int, msgs []*llm.Message) []*llm.Message
	// AfterToolCall 工具执行后回调（落块、记忆抽取、审计）。
	AfterToolCall func(ctx context.Context, call Call, res tool.ToolResult)
	// ShouldStop 轮结束后询问是否提前终止。
	ShouldStop func(ctx context.Context, turn int) bool
	// Steering 轮间插话：跑过工具之后、下一轮之前注入（长任务中途纠偏）。
	Steering func(ctx context.Context) []*llm.Message
	// FollowUp 收尾续接：本轮无工具调用、run 即将收尾时注入（自动接下一波）。
	FollowUp func(ctx context.Context) []*llm.Message
}

// Compressor 上下文压缩器：预算内返回原序列，超预算返回折叠后的序列。
type Compressor interface {
	Compress(msgs []*llm.Message, budget int) ([]*llm.Message, CompressInfo)
}

// Checkpoint 与 CheckpointStore 定义见 checkpoint.go。

// Outcome run 结果与新增消息；调用方负责落库。
type Outcome struct {
	Reason     string
	StopReason string
	Content    string         // 累积正文（多轮拼接）
	Thinking   string         // 累积推理
	Messages   []*llm.Message // 本 run 新增（assistant + tool），不含输入历史
	Usage      llm.TokenUsage // run 累计用量
	Turns      int
	PerTurn    []TurnUsage // 每轮用量明细：token_usages 逐调用落库的数据源
	Err        error       // 失败原因（Reason == ReasonError 时非 nil）
	Timings    Timings     // 分段耗时归因：回答「这次 run 慢在哪」
}

// Timings 分段耗时（毫秒）。三段互斥且近似穷举：等模型、跑工具、压缩上下文。
// 剩余差额是本机调度与落库开销——低到不值得单独归因。
type Timings struct {
	LLMMs      int64
	ToolsMs    int64
	CompressMs int64
}

// TurnUsage 单轮用量。
type TurnUsage struct {
	Turn      int
	Usage     llm.TokenUsage
	LatencyMs int64 // 本轮「建流→流结束」墙钟耗时
}

// Loop ReAct 执行内核：一个 for，三件事（请求 → 执行 → 回填）。
type Loop struct {
	provider llm.Provider
	registry *tool.Registry
	exec     Handler
	sink     Sink
	cfg      Config
	meta     Meta
	hooks    Hooks
	exposed  map[string]bool // nil = 全部已注册工具
	compress Compressor
	cp       CheckpointStore

	assistantMsgID string // 续跑落回同一条 assistant 消息，不新开
	steps          *MapSteps

	// retryPolicy 与 onRetry 共同决定建流重试：两者都不为 nil 时生效；
	// nil 时沿用旧行为（建流瞬时错误直接失败），兼容既有测试与未注入重试的场景。
	retryPolicy llm.RetryPolicy
	onRetry     func(attempt int, delay time.Duration)
}

// New 构造 Loop；执行器默认为裸 Executor，需经 WithGuard 装上护栏链。
func New(provider llm.Provider, reg *tool.Registry, cfg Config) *Loop {
	if reg == nil {
		reg = tool.NewRegistry()
	}
	return &Loop{provider: provider, registry: reg, cfg: cfg.withDefaults(), sink: NopSink{}}
}

// WithSink 设置事件出口。
func (l *Loop) WithSink(s Sink) *Loop {
	if s != nil {
		l.sink = s
	}
	return l
}

// WithMeta 设置 run 身份。
func (l *Loop) WithMeta(m Meta) *Loop { l.meta = m; return l }

// WithHooks 设置扩展点。
func (l *Loop) WithHooks(h Hooks) *Loop { l.hooks = h; return l }

// WithCompressor 设置压缩器。
func (l *Loop) WithCompressor(c Compressor) *Loop { l.compress = c; return l }

// WithCheckpoints 设置检查点存储。
func (l *Loop) WithCheckpoints(s CheckpointStore) *Loop { l.cp = s; return l }

// WithAssistantMessage 指定 assistant 消息 id：检查点据此落位，续跑不新开消息。
func (l *Loop) WithAssistantMessage(id string) *Loop { l.assistantMsgID = id; return l }

// WithSteps 注入步骤记忆（续跑复用已完成调用，不重放副作用）。
func (l *Loop) WithSteps(s *MapSteps) *Loop { l.steps = s; return l }

// WithRetry 注入建流重试策略与回调；policy 为零值或 onRetry 为 nil 时不重试。
// 每次进入下一次重试前会调 onRetry(attempt, delay)，编排层可借此发 chat:retry。
func (l *Loop) WithRetry(policy llm.RetryPolicy, onRetry func(attempt int, delay time.Duration)) *Loop {
	if onRetry == nil {
		return l
	}
	l.retryPolicy = policy
	l.onRetry = onRetry
	return l
}

// WithGuard 组装护栏中间件链并生成最终执行器。
func (l *Loop) WithGuard(ms ...Middleware) *Loop {
	l.exec = Chain(ms...)(Executor(l.cfg.Exec))
	return l
}

// Expose 限制本轮暴露给模型的工具；不调用则暴露全部已注册工具。
func (l *Loop) Expose(names []string) *Loop {
	if len(names) == 0 {
		l.exposed = nil
		return l
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	l.exposed = m
	return l
}

// Run 从头执行主循环；ctx 取消归一为 cancelled。
func (l *Loop) Run(ctx context.Context, history []*llm.Message) (*Outcome, error) {
	msgs := make([]*llm.Message, 0, len(history)+1)
	if l.cfg.System != "" {
		msgs = append(msgs, llm.SystemMessage(l.cfg.System))
	}
	msgs = append(msgs, history...)
	return l.runLoop(ctx, msgs, 1, &Outcome{Reason: ReasonEndTurn}, true)
}

// Resume 从检查点续跑：只认最后一轮，已完成工具调用经幂等中间件复用，不重放副作用。
// 不重发 run 起始事件——「本次是续跑」由调用方标注。
func (l *Loop) Resume(ctx context.Context) (*Outcome, error) {
	if l.cp == nil {
		return nil, pkg.New(5004, "检查点未启用", "")
	}
	cp, err := l.cp.LoadLast(l.meta.RunID)
	if err != nil {
		return nil, pkg.Wrap(5004, "加载检查点失败", err)
	}
	if cp == nil || len(cp.Messages) == 0 {
		return nil, pkg.New(5004, "无可用检查点", l.meta.RunID)
	}
	// 已完成调用跨进程复用：检查点里存了哪些副作用已经发生过。
	l.steps = NewMapSteps(cp.Steps)
	// 用量跨段累计：续跑不是新的计费周期。
	return l.runLoop(ctx, cp.Messages, cp.Turn+1, &Outcome{
		Reason: ReasonEndTurn, Usage: cp.Usage, Content: cp.Content, Thinking: cp.Thinking,
	}, false)
}

// runLoop 主循环：请求 → 执行 → 回填，三件事一个 for。Run 与 Resume 共用。
func (l *Loop) runLoop(ctx context.Context, msgs []*llm.Message, startTurn int, out *Outcome, emitStart bool) (*Outcome, error) {
	if l.provider == nil {
		return nil, pkg.New(5001, "provider 未就绪", "")
	}
	if l.exec == nil {
		l.exec = Executor(l.cfg.Exec)
	}
	if l.steps == nil {
		l.steps = NewMapSteps(nil)
	}
	// run 身份注入 ctx：工具与审计经 core.RunIDFromCtx / SessionIDFromCtx 取用。
	ctx = WithRunContext(ctx, l.meta.RunID, l.meta.SessionID)
	if emitStart {
		l.emit(EventRunStart, 0, RunStartPayload{Model: l.cfg.Model, Provider: l.provider.Name()})
	}

	for turn := startTurn; ; turn++ {
		if err := ctx.Err(); err != nil {
			return l.finish(out, ReasonCancelled, ""), nil
		}
		if turn > l.cfg.MaxTurns {
			return l.finish(out, ReasonMaxTurns, ""), nil
		}
		// 成本熔断：与轮次上限分工——一个管「花多少钱」，一个管「跑多大」。
		if l.cfg.MaxRunTokens > 0 && out.Usage.TotalTokens >= l.cfg.MaxRunTokens {
			return l.finish(out, ReasonBudget, ""), nil
		}
		if l.hooks.BeforeTurn != nil {
			msgs = l.hooks.BeforeTurn(ctx, turn, msgs)
		}
		if l.compress != nil && l.cfg.MaxInput > 0 {
			c0 := time.Now()
			var info CompressInfo
			msgs, info = l.compress.Compress(msgs, l.cfg.MaxInput)
			out.Timings.CompressMs += time.Since(c0).Milliseconds()
			// 压缩是上下文被改写的少数时刻，不发事件用户只会觉得「前面的聊天不见了」。
			if info.Removed > 0 {
				l.emit(EventCompressed, turn, CompressedPayload{
					Removed: info.Removed, Summary: info.Summary, Truncated: info.Truncated,
					CutoffAt: time.Now().UnixMilli(),
				})
			}
		}

		l.emit(EventTurnStart, turn, nil)
		res, err := l.requestTurn(ctx, turn, msgs)
		if err != nil {
			out.Err = err
			return l.finish(out, ReasonError, ""), err
		}
		out.Turns = turn
		out.Timings.LLMMs += res.LatencyMs
		out.Content += res.Content
		out.Thinking += res.Thinking
		out.Usage = addUsage(out.Usage, res.Usage)
		out.PerTurn = append(out.PerTurn, TurnUsage{Turn: turn, Usage: res.Usage, LatencyMs: res.LatencyMs})
		out.StopReason = res.StopReason

		assistant := &llm.Message{
			Role:      llm.RoleAssistant,
			Content:   res.Content,
			Thinking:  res.Thinking,
			ToolCalls: res.ToolCalls(),
		}
		msgs = append(msgs, assistant)
		out.Messages = append(out.Messages, assistant)
		l.emit(EventTurnEnd, turn, TurnEndPayload{
			Usage:      usagePayload(res.Usage),
			ToolCalls:  len(res.ToolCallsRaw),
			StopReason: res.StopReason,
			LatencyMs:  res.LatencyMs,
		})

		if len(res.ToolCallsRaw) == 0 {
			// 收尾缝：待发送消息或 Stop 钩子要求的续跑在此接入，run 才会继续。
			if fu := l.followUp(ctx); len(fu) > 0 {
				msgs = append(msgs, fu...)
				continue
			}
			return l.finish(out, ReasonEndTurn, res.StopReason), nil
		}

		t0 := time.Now()
		toolMsgs := l.runTools(ctx, turn, res.ToolCallsRaw)
		out.Timings.ToolsMs += time.Since(t0).Milliseconds()
		msgs = append(msgs, toolMsgs...)
		out.Messages = append(out.Messages, toolMsgs...)
		// 轮间插话：跑过工具后允许用户纠偏，下一轮生效。
		if steer := l.steering(ctx); len(steer) > 0 {
			msgs = append(msgs, steer...)
		}

		l.checkpoint(turn, msgs, out)
		if l.hooks.ShouldStop != nil && l.hooks.ShouldStop(ctx, turn) {
			return l.finish(out, ReasonStopped, res.StopReason), nil
		}
	}
}

// finish 统一出口：先发终态事件再返回。
func (l *Loop) finish(out *Outcome, reason, stopReason string) *Outcome {
	if stopReason != "" {
		out.StopReason = stopReason
	}
	out.Reason = reason
	l.emit(EventRunDone, out.Turns, RunDonePayload{
		Reason: reason, StopReason: out.StopReason, Usage: usagePayload(out.Usage), Turns: out.Turns,
	})
	return out
}

// steering 轮间缝：跑过工具后取插话。
func (l *Loop) steering(ctx context.Context) []*llm.Message {
	if l.hooks.Steering == nil {
		return nil
	}
	return l.hooks.Steering(ctx)
}

// followUp 收尾缝：无工具调用、run 即将收尾时取续接消息。
func (l *Loop) followUp(ctx context.Context) []*llm.Message {
	if l.hooks.FollowUp == nil {
		return nil
	}
	return l.hooks.FollowUp(ctx)
}

// checkpoint 写入续跑位点；失败仅记事件，不阻断 run。
func (l *Loop) checkpoint(turn int, msgs []*llm.Message, out *Outcome) {
	if l.cp == nil {
		return
	}
	cp := &Checkpoint{
		RunID:          l.meta.RunID,
		SessionID:      l.meta.SessionID,
		Turn:           turn,
		AssistantMsgID: l.assistantMsgID,
		Messages:       msgs,
		Content:        out.Content,
		Thinking:       out.Thinking,
		Usage:          out.Usage,
		Steps:          l.steps.Snapshot(),
		CreatedAt:      time.Now().UnixMilli(),
	}
	if err := l.cp.Append(cp); err != nil {
		l.emit(EventError, turn, ErrorPayload{Code: 5003, Message: "检查点写入失败", Kind: "persist"})
		return
	}
	l.emit(EventCheckpoint, turn, CheckpointPayload{Turn: turn})
}

// runTools 执行本轮工具调用；全只读且并行度 > 1 时并发，否则严格串行。
// 结果严格按调用顺序回填，保证 assistant(tool_calls) 与 tool 结果配对完整。
func (l *Loop) runTools(ctx context.Context, turn int, calls []llm.NormalizedToolCall) []*llm.Message {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Name)
	}
	parallel := l.cfg.Parallel > 1 && len(calls) > 1 && l.registry.AllReadOnly(names)

	results := make([]*llm.Message, len(calls))
	if !parallel {
		for i, c := range calls {
			results[i] = l.runOne(ctx, turn, c)
		}
		return results
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, l.cfg.Parallel)
	for i, c := range calls {
		wg.Add(1)
		go func(i int, c llm.NormalizedToolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = l.runOne(ctx, turn, c)
		}(i, c)
	}
	wg.Wait()
	return results
}

// runOne 执行一次调用：经护栏链 → 发事件 → 转成 tool 消息。
func (l *Loop) runOne(ctx context.Context, turn int, tc llm.NormalizedToolCall) *llm.Message {
	t, _ := l.registry.Get(tc.Name)
	call := Call{ID: tc.ID, Name: tc.Name, Args: tc.Arguments, Tool: t, RunID: l.meta.RunID, Turn: turn}

	l.emit(EventToolCall, turn, ToolCallPayload{
		ID: tc.ID, Name: tc.Name, Arguments: string(tc.Arguments), Activity: tool.ActivityOf(t, tc.Arguments),
	})
	l.emit(EventToolStart, turn, ToolCallPayload{
		ID: tc.ID, Name: tc.Name, Activity: tool.ActivityOf(t, tc.Arguments),
	})

	start := time.Now()
	res := l.exec(ctx, call)
	if l.hooks.AfterToolCall != nil {
		l.hooks.AfterToolCall(ctx, call, res)
	}

	errText := ""
	if res.Err != nil {
		errText = res.Err.Error()
	}
	l.emit(EventToolResult, turn, ToolResultPayload{
		ToolCallID: tc.ID, Name: tc.Name, Content: res.Content, Err: errText,
		DurationMs: time.Since(start).Milliseconds(), Meta: res.Meta, Data: res.Data,
		Refused: res.Refused, RefusedReason: res.Meta["refused_reason"],
		UIHint: tool.MetaOf(t).UIHint,
	})

	content := res.Content
	if res.Err != nil {
		content = "工具执行失败: " + res.Err.Error()
	}
	return llm.ToolMessage(tc.ID, tc.Name, content)
}

// streamWithRetry 包装 provider.Stream：瞬时错误按 policy 退避后重试，
// 每轮重试前调 onRetry 回调发 agent.retry 事件给编排层桥接 chat:retry。
// 未配置重试策略时回退到原行为（一次尝试、瞬时错误即失败）。
func (l *Loop) streamWithRetry(ctx context.Context, turn int, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	maxAttempts := l.retryPolicy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = l.cfg.MaxRetries + 1
	}
	if maxAttempts <= 0 {
		maxAttempts = 4 // 含首调的默认值：失败 → 重试 3 次
	}
	for attempt := 0; ; attempt++ {
		ch, err := l.provider.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}
		if l.onRetry == nil || !llm.IsTransient(err) || attempt+1 >= maxAttempts {
			return nil, err
		}
		delay := l.retryPolicy.Backoff(attempt, llm.RetryAfter(err))
		l.onRetry(attempt+1, delay)
		if waitErr := llm.Wait(ctx, delay); waitErr != nil {
			return nil, waitErr
		}
	}
}

// requestTurn 单轮模型请求；流式消费 chunk 并实时发增量事件。
func (l *Loop) requestTurn(ctx context.Context, turn int, msgs []*llm.Message) (*turnResult, error) {
	req := &llm.ChatRequest{
		Model:       l.cfg.Model,
		Messages:    msgs,
		Tools:       l.toolDefs(),
		Temperature: l.cfg.Temperature,
		MaxTokens:   l.cfg.MaxTokens,
		Thinking:    l.cfg.Thinking,
		SessionID:   l.meta.SessionID,
	}
	started := time.Now()
	ch, err := l.streamWithRetry(ctx, turn, req)
	if err != nil {
		l.emit(EventError, turn, ErrorPayload{Code: 3005, Message: err.Error(), Kind: "connection"})
		return nil, pkg.Wrap(3005, "建流失败", err)
	}

	res := &turnResult{LatencyMs: time.Since(started).Milliseconds()}
	for chunk := range ch {
		if chunk.Err != nil {
			l.emit(EventError, turn, ErrorPayload{Code: 3006, Message: chunk.Err.Error(), Kind: "upstream"})
			res.HadError = true
			continue
		}
		if s := chunk.Delta.Content; s != "" {
			res.Content += s
			l.emit(EventTurnDelta, turn, DeltaPayload{Kind: "content", Text: s})
		}
		if s := chunk.Delta.Thinking; s != "" {
			res.Thinking += s
			l.emit(EventTurnThinking, turn, DeltaPayload{Kind: "thinking", Text: s})
		}
		if chunk.ToolCall != nil {
			res.ToolCallsRaw = append(res.ToolCallsRaw, *chunk.ToolCall)
		}
		if chunk.FinalUsage != nil {
			res.Usage = *chunk.FinalUsage
		}
		if chunk.FinishReason != nil {
			res.StopReason = *chunk.FinishReason
		}
	}
	return res, nil
}

// toolDefs 生成暴露给模型的工具定义。
func (l *Loop) toolDefs() []llm.ToolDefinition {
	all := l.registry.List()
	defs := make([]llm.ToolDefinition, 0, len(all))
	for _, t := range all {
		if l.exposed != nil && !l.exposed[t.Name()] {
			continue
		}
		s := t.Schema()
		var params map[string]any
		if len(s.Parameters) > 0 {
			_ = json.Unmarshal(s.Parameters, &params)
		}
		defs = append(defs, llm.ToolDefinition{Name: s.Name, Description: s.Description, Parameters: params})
	}
	return defs
}

// emit 注入 run 身份后推送事件。
func (l *Loop) emit(kind EventKind, turn int, payload any) {
	l.sink.Emit(Event{
		Kind: kind, RunID: l.meta.RunID, ParentRunID: l.meta.ParentRunID,
		SessionID: l.meta.SessionID, Turn: turn, Agent: l.meta.Agent, Payload: payload,
	})
}

// turnResult 单轮模型输出。
type turnResult struct {
	Content      string
	Thinking     string
	ToolCallsRaw []llm.NormalizedToolCall
	Usage        llm.TokenUsage
	StopReason   string
	LatencyMs    int64
	HadError     bool
}

// ToolCalls 转成 assistant 消息的 ToolCall 结构。
func (r *turnResult) ToolCalls() []llm.ToolCall {
	if len(r.ToolCallsRaw) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(r.ToolCallsRaw))
	for _, c := range r.ToolCallsRaw {
		out = append(out, llm.ToolCall{
			ID: c.ID, Type: "function",
			Function: llm.FunctionCall{Name: c.Name, Arguments: string(c.Arguments)},
		})
	}
	return out
}

// usagePayload 转事件载荷。
func usagePayload(u llm.TokenUsage) UsagePayload {
	return UsagePayload{
		Input: u.InputTokens, Output: u.OutputTokens,
		CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens, Total: u.TotalTokens,
	}
}
