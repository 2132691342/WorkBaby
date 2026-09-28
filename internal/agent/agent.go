// Package agent 是内核：单层流式循环 + 工具调度 + 上下文裁剪。
// 它不依赖 HTTP、数据库与桌面壳，LLM、工具、审批全部经 Config 注入。
package agent

import (
	"context"
	"sync"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
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

// 同一工具同一参数连续重复到该次数就判失败：模型卡在同一个调用上打转时，
// 撞轮数上限之前就能拦住，且报错信息比「达到最大轮数」可诊断得多。
const repeatCallLimit = 3

// Gate 是内核唯一的回调面：审批等需要挂起等用户决策的逻辑走这里。
// 返回 blocked=true 时该调用不执行，直接生成一条失败结果让模型换方案。
type Gate func(ctx context.Context, call *llm.ToolCall) (blocked bool, reason string)

// Budget 是上下文预算。三个整数由 service 层按模型与设置备好，
// 内核只负责按预算裁剪，不认识设置表也不认识模型名。
type Budget struct {
	Window  int // 模型上下文窗口
	Reserve int // 给模型输出留的余量
	Keep    int // 裁剪后保留的近期 token 预算
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
	MaxTurns  int
	Parallel  int
	Budget    Budget
	Gate      Gate
	Emit      func(Event)
	Steering  *Queue
}

// Loop 是一次运行的内核实例。
type Loop struct {
	cfg      Config
	mu       sync.Mutex
	msgs     []llm.Message
	writeMu  sync.Mutex
	repeated map[string]int
}

// New 构造内核；传入的历史会被复制，避免调用方后续修改造成竞态。
func New(cfg Config, history []llm.Message) *Loop {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 32
	}
	if cfg.Emit == nil {
		cfg.Emit = func(Event) {}
	}
	msgs := make([]llm.Message, len(history))
	copy(msgs, history)
	return &Loop{cfg: cfg, msgs: msgs, repeated: map[string]int{}}
}

// emit 是事件出口的唯一实现：同步串行调用，避免并发写出乱序事件。
func (l *Loop) emit(e Event) { l.cfg.Emit(e) }

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
		l.reportContext()
		l.drainSteering()

		l.emit(Event{Kind: EventTurnStart, Turn: turn})

		msg, stop, usage, err := l.streamTurn(ctx)
		if usage != nil {
			res.Usage.Input += usage.Input
			res.Usage.Output += usage.Output
			res.Usage.Total += usage.Total
			res.Usage.LatencyMs += usage.LatencyMs
		}
		if err != nil {
			l.appendMessage(msg)
			res.Turns = turn
			return l.finish(res, StopError, err)
		}
		l.appendMessage(msg)

		// 被截断的响应里参数可能不完整，一律判失败，绝不执行。
		calls := msg.ToolCalls
		if stop == llm.StopLength {
			calls = nil
		}
		if len(calls) > 0 {
			l.executeTools(ctx, calls)
		}

		res.Turns = turn
		l.emit(Event{Kind: EventTurnEnd, Turn: turn})

		if stop == llm.StopAborted || ctx.Err() != nil {
			return l.finish(res, StopAborted, nil)
		}
		if stop == llm.StopLength {
			return l.finish(res, StopLength, nil)
		}
		if stop == llm.StopError {
			return l.finish(res, StopError, nil)
		}
		if len(calls) == 0 {
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

// compact 按预算裁剪上下文；只有真的裁掉东西才发事件，避免前端反复闪提示。
func (l *Loop) compact() {
	snapshot := l.Messages()
	before := EstimateTokens(l.cfg.System, snapshot)
	trimmed, after := Compact(snapshot, l.cfg.Budget)
	if after >= before {
		return
	}
	l.mu.Lock()
	l.msgs = trimmed
	l.mu.Unlock()
	l.emit(Event{Kind: EventCompressed, TokensBefore: before, TokensAfter: after})
}

// reportContext 每轮广播一次上下文占用：用户靠它判断「还能聊多久、该不该开新对话」。
func (l *Loop) reportContext() {
	snapshot := l.Messages()
	l.emit(Event{
		Kind:         EventContext,
		TokensUsed:   EstimateTokens(l.cfg.System, snapshot),
		TokensWindow: l.cfg.Budget.Window,
		Messages:     len(snapshot),
	})
}

// drainSteering 把轮间插话并进上下文；一次只取一条，连续发三句不会一次糊给模型。
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
}

// streamTurn 请求一次模型，把流式增量实时发出，返回归一后的 assistant 消息。
func (l *Loop) streamTurn(ctx context.Context) (llm.Message, string, *llm.Usage, error) {
	req := llm.Request{
		Model:     l.cfg.Model,
		System:    l.cfg.System,
		Messages:  CleanForProtocol(l.Messages()),
		Tools:     llmToolDefs(l.cfg.Tools),
		MaxTokens: l.cfg.MaxTokens,
	}
	events, err := l.cfg.Streamer.Stream(ctx, req)
	if err != nil {
		return llm.Message{Role: llm.RoleAssistant}, StopError, nil, err
	}

	msg := llm.Message{Role: llm.RoleAssistant}
	stop := llm.StopStop
	var usage *llm.Usage
	for ev := range events {
		switch ev.Type {
		case llm.EventDelta:
			msg.Content += ev.Delta
			l.emit(Event{Kind: EventDelta, DeltaKind: DeltaText, Delta: ev.Delta})
		case llm.EventThinking:
			msg.Thinking += ev.Delta
			l.emit(Event{Kind: EventDelta, DeltaKind: DeltaThinking, Delta: ev.Delta})
		case llm.EventToolCall:
			if ev.ToolCall != nil {
				msg.ToolCalls = append(msg.ToolCalls, *ev.ToolCall)
			}
		case llm.EventDone:
			stop = ev.StopReason
			if ev.Usage != nil {
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
	return msg, stop, usage, nil
}

// appendMessage 把消息追加进累积数组。
func (l *Loop) appendMessage(m llm.Message) {
	l.mu.Lock()
	l.msgs = append(l.msgs, m)
	l.mu.Unlock()
}
