// Package agent 是内核：单层流式循环 + 工具调度 + 上下文裁剪。
// 它不依赖 HTTP、数据库与桌面壳，LLM、工具、审批全部经 Config 注入。
package agent

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/tool"
)

// 停止原因与 llm 层保持一致，内核只做透传与归一化。
const (
	StopCompleted = "completed"
	StopAborted   = "aborted"
	StopError     = "error"
	StopLength    = "length"
	StopMaxTurns  = "max_turns"
)

var ErrNoStreamer = pkg.New(5001, "没有可用的模型服务", "")

// repeatCallLimit 是同一工具同一参数的连续重复上限：超过就判失败，
// 让模型换方案，而不是等它撞到轮数上限。
const repeatCallLimit = 3

// Gate 是内核唯一的回调面：审批等需要挂起等用户决策的逻辑走这里。
// 返回 blocked=true 时该调用不执行，直接生成一条失败结果让模型换方案。
type Gate func(ctx context.Context, call *llm.ToolCall) (blocked bool, reason string)

// Budget 是上下文预算，由 service 层按模型与设置备好；内核只按预算裁剪。
type Budget struct {
	Window       int  // 模型上下文窗口
	WindowKnown  bool // false 表示窗口是估算值，前端读数加「约」前缀
	Reserve      int  // 给模型输出留的余量
	Keep         int  // 裁剪后保留的近期 token 预算
	SystemTokens int  // system 提示词的 token 估算：判断是否超预算必须算上它
	// ToolsTokens 是工具声明的 token 估算。工具 schema 与消息一样每轮都发出去，
	// 漏算它水位会偏低、压缩会来得太晚。由 New 按 Config.Tools 自动填，调用方不必管。
	ToolsTokens int
}

// Result 是一次 run 的产出。
type Result struct {
	Messages   []llm.Message
	StopReason string
	Usage      llm.Usage
	Turns      int
}

// Config 是内核的全部配置，构造期注入。
type Config struct {
	Streamer  llm.Streamer
	Tools     []tool.Tool
	Deps      tool.Deps
	Workspace string
	System    string
	Model     string
	MaxTokens int
	// Temperature / TopP 由调用方按模型配置备好；nil 表示不下发，交给上游默认。
	Temperature *float64
	TopP        *float64
	MaxTurns    int
	Parallel    int
	Budget      Budget
	Gate        Gate
	Emit        func(Event)
	Steering    *Queue
}

// Loop 是一次运行的内核实例。
type Loop struct {
	cfg      Config
	mu       sync.Mutex
	emitMu   sync.Mutex
	msgs     []llm.Message
	repeated map[string]int
}

// New 构造内核；传入的历史会被复制，避免调用方后续修改造成竞态。
func New(cfg Config, history []llm.Message) *Loop {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 64
	}
	if cfg.Emit == nil {
		cfg.Emit = func(Event) {}
	}
	// 工具声明由内核自己数：调用方少填一项预算，压缩判断就会偏乐观。
	cfg.Budget.ToolsTokens = estimateToolTokens(cfg.Tools)
	msgs := make([]llm.Message, len(history))
	copy(msgs, history)
	return &Loop{cfg: cfg, msgs: msgs, repeated: map[string]int{}}
}

// emit 是事件出口的唯一实现：把并发调用串行化。
// 并行工具各自在 goroutine 里 emit，不加锁则两个事件抢同一个条目 id，声明被主键冲突顶掉。
func (l *Loop) emit(e Event) {
	l.emitMu.Lock()
	defer l.emitMu.Unlock()
	l.cfg.Emit(e)
}

// Messages 返回当前累积的消息（含本轮产出），供调用方落库。
func (l *Loop) Messages() []llm.Message {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]llm.Message, len(l.msgs))
	copy(out, l.msgs)
	return out
}

// Run 执行单层循环：每轮压缩 → 注入插话 → 流式调模型 → 执行工具 → 按序回填。
func (l *Loop) Run(ctx context.Context) (*Result, error) {
	if l.cfg.Streamer == nil {
		return nil, ErrNoStreamer
	}
	res := &Result{StopReason: StopCompleted}
	l.emit(Event{Kind: EventAgentStart})

	for turn := 1; turn <= l.cfg.MaxTurns; turn++ {
		if ctx.Err() != nil {
			return l.finish(res, StopAborted, nil)
		}
		l.compact()
		l.drainSteering()
		// 插话是并进下一轮上下文的，所以占用要在插话之后量，才是真正发出去的那份。
		ctxTokens := l.reportContext()

		l.emit(Event{Kind: EventTurnStart, Turn: turn})

		msg, stop, usage, err := l.streamTurnWithRetry(ctx)
		if err != nil {
			l.appendMessage(msg)
			res.Turns = turn
			return l.finish(res, StopError, err)
		}

		if stop == llm.StopLength {
			// 截断轮的参数只有半个 JSON，既不执行也不留进历史：
			// 留着会被下一轮当成真调用发回上游，等于凭空多出一个没做过的动作。
			msg.ToolCalls = nil
		}

		if usage != nil {
			// 上游报出的输入量比本地估算更可信：取两者较大者，消息指标、会话累计、
			// 仪表盘三处口径一致，也不会出现「入 9.9K 上下文 25K」这种自相矛盾的读数。
			// 本地估算偏保守时以它兜底，估算偏高时以上游为准。
			if usage.Input > ctxTokens {
				ctxTokens = usage.Input
			}
			usage.Input = ctxTokens
			if usage.Total < usage.Input+usage.Output {
				usage.Total = usage.Input + usage.Output
			}
			res.Usage.Input += usage.Input
			res.Usage.Output += usage.Output
			res.Usage.Total += usage.Total
			res.Usage.Cached += usage.Cached
			res.Usage.LatencyMs += usage.LatencyMs
		}
		l.appendMessage(msg)

		if len(msg.ToolCalls) > 0 {
			l.executeTools(ctx, msg.ToolCalls)
		}

		res.Turns = turn
		// 用量必须随本轮 turn_end 一起给出：落库是 append-only，事后补写做不到。
		l.emit(Event{Kind: EventTurnEnd, Turn: turn, Usage: usage, ContextTokens: ctxTokens})

		if stop == llm.StopAborted || ctx.Err() != nil {
			return l.finish(res, StopAborted, nil)
		}
		if stop == llm.StopLength {
			return l.finish(res, StopLength, nil)
		}
		if stop == llm.StopError {
			return l.finish(res, StopError, nil)
		}
		if len(msg.ToolCalls) == 0 {
			return l.finish(res, StopCompleted, nil)
		}
	}

	return l.finish(res, StopMaxTurns, nil)
}

// finish 是所有退出路径的唯一出口：已产出的消息照常保留，用户能看到半成品。
func (l *Loop) finish(res *Result, reason string, err error) (*Result, error) {
	res.StopReason = reason
	res.Messages = l.Messages()
	l.emit(Event{Kind: EventAgentEnd, StopReason: reason, Usage: &res.Usage, Turns: res.Turns, Err: err})
	return res, err
}

// compact 按预算裁剪上下文。是否裁过以消息条数为准，事件里的 token 一律是完整口径。
func (l *Loop) compact() {
	snapshot := l.Messages()
	trimmed, after := Compact(snapshot, l.cfg.Budget)
	if len(trimmed) == len(snapshot) {
		return
	}
	l.mu.Lock()
	l.msgs = trimmed
	l.mu.Unlock()
	l.emit(Event{Kind: EventCompressed,
		TokensBefore: fullContext(l.cfg.Budget, snapshot),
		TokensAfter:  after})
}

// reportContext 每轮广播一次上下文占用：用户靠它判断「还能聊多久、该不该开新对话」。
// 返回本次实测值，调用方把它挂到 turn_end 上随消息一起落库。
func (l *Loop) reportContext() int {
	snapshot := l.Messages()
	used := fullContext(l.cfg.Budget, snapshot)
	l.emit(Event{
		Kind:          EventContext,
		TokensUsed:    used,
		TokensWindow:  l.cfg.Budget.Window,
		TokensKnown:   l.cfg.Budget.WindowKnown,
		TokensReserve: l.cfg.Budget.Reserve,
		Messages:      len(snapshot),
	})
	return used
}

// drainSteering 把轮间插话并进上下文，一次只取一条。
// 每条插话发一个 steering 事件：上层必须在注入时刻落库，链序才与真实对话一致。
func (l *Loop) drainSteering() {
	if l.cfg.Steering == nil {
		return
	}
	pending := l.cfg.Steering.Drain()
	if len(pending) == 0 {
		return
	}
	l.mu.Lock()
	l.msgs = append(l.msgs, pending...)
	l.mu.Unlock()
	for _, m := range pending {
		l.emit(Event{Kind: EventSteering, UserContent: m.Content})
	}
}

// 降级重试的边界：预算收到 minRetryMaxTokens 以下就短到没用，不再试；
// maxRetryTurns 是次数上限——被拒的轮次一个 token 都没产出，但恒拒绝的端点不能无限试。
const (
	minRetryMaxTokens = 4096
	maxRetryTurns     = 2
)

// streamTurnWithRetry 请求模型，两类「本地算不准、上游能纠正」的拒绝按需降级重试：
// 上下文超限 → 强制压缩（只做一次）；输出预算被拒 → 按上游说的上限收紧（最多两次）。
// 输出预算只降不升：内核不知道模型能吐多少，抬过头就是把能看懂的截断换成看不懂的 400。
func (l *Loop) streamTurnWithRetry(ctx context.Context) (llm.Message, string, *llm.Usage, error) {
	maxTokens := l.cfg.MaxTokens
	compacted := false
	for attempt := 0; ; attempt++ {
		msg, stop, usage, err := l.streamTurn(ctx, maxTokens)
		if err == nil || ctx.Err() != nil || attempt >= maxRetryTurns {
			return msg, stop, usage, err
		}
		// 已经吐出过内容的轮次与重试无关：再发一次会让用户看到一段重复的正文。
		if strings.TrimSpace(msg.Content) != "" || strings.TrimSpace(msg.Thinking) != "" || len(msg.ToolCalls) > 0 {
			return msg, stop, usage, err
		}
		switch {
		case llm.IsContextOverflow(err) && !compacted:
			compacted = true
			l.forceCompact()
		case llm.IsOutputLimit(err):
			next := llm.OutputLimitFrom(err, maxTokens)
			if next >= maxTokens || next < minRetryMaxTokens {
				return msg, stop, usage, err
			}
			pkg.Warnf("agent: 上游拒绝输出预算 %d，本轮降到 %d 重试: %v", maxTokens, next, err)
			maxTokens = next
		default:
			return msg, stop, usage, err
		}
	}
}

// forceCompact 上游报超限后的强制裁剪：保留量砍半、允许硬切，
// 裁剪结果照常广播——用户要能看到「它整理了上下文」而不是凭空重试。
func (l *Loop) forceCompact() {
	snapshot := l.Messages()
	trimmed, after := CompactForce(snapshot, l.cfg.Budget)
	if len(trimmed) == len(snapshot) {
		return
	}
	l.mu.Lock()
	l.msgs = trimmed
	l.mu.Unlock()
	l.emit(Event{Kind: EventCompressed,
		TokensBefore: fullContext(l.cfg.Budget, snapshot),
		TokensAfter:  after})
	pkg.Warnf("agent: 上游报上下文超限，已强制压缩 %d 条 → %d 条（%d → %d tokens）",
		len(snapshot), len(trimmed), fullContext(l.cfg.Budget, snapshot), after)
}

// streamTurn 请求一次模型，把流式增量实时发出，返回归一后的 assistant 消息。
// maxTokens 由调用方给出（service 层按模型能力定的输出预算，内核不自行改动）。
func (l *Loop) streamTurn(ctx context.Context, maxTokens int) (llm.Message, string, *llm.Usage, error) {
	req := llm.Request{
		Model:       l.cfg.Model,
		System:      l.cfg.System,
		Messages:    CleanForProtocol(l.Messages()),
		Tools:       llmToolDefs(l.cfg.Tools),
		MaxTokens:   maxTokens,
		Temperature: l.cfg.Temperature,
		TopP:        l.cfg.TopP,
	}
	events, err := l.cfg.Streamer.Stream(ctx, req)
	if err != nil {
		return llm.Message{Role: llm.RoleAssistant}, StopError, nil, err
	}

	msg := llm.Message{Role: llm.RoleAssistant}
	stop := llm.StopStop
	var usage *llm.Usage
	// 每个 token 拼一次字符串是把整段正文复制一遍：长回答末尾就是 O(n²)。
	// Builder 一次分配、最后收口，循环里只做追加。
	var body, thinking strings.Builder
	for ev := range events {
		switch ev.Type {
		case llm.EventDelta:
			body.WriteString(ev.Delta)
			l.emit(Event{Kind: EventDelta, DeltaKind: DeltaText, Delta: ev.Delta})
		case llm.EventThinking:
			thinking.WriteString(ev.Delta)
			l.emit(Event{Kind: EventDelta, DeltaKind: DeltaThinking, Delta: ev.Delta})
		case llm.EventToolCall:
			if ev.ToolCall != nil {
				msg.ToolCalls = append(msg.ToolCalls, *ev.ToolCall)
			}
		case llm.EventDone:
			stop = ev.StopReason
			if ev.Usage != nil {
				// 口径归一收在内核这一处：新增适配器不必各自记得做，命中率也不会越过 100%。
				ev.Usage.Normalize()
				usage = ev.Usage
			}
		case llm.EventError:
			if ev.Err != nil {
				return msg, StopError, usage, ev.Err
			}
		}
	}
	if ctx.Err() != nil {
		stop = llm.StopAborted
	}
	msg.Content = body.String()
	msg.Thinking = thinking.String()
	return msg, stop, usage, nil
}

// appendMessage 把消息追加进累积数组。
func (l *Loop) appendMessage(m llm.Message) {
	l.mu.Lock()
	l.msgs = append(l.msgs, m)
	l.mu.Unlock()
}
