// 测试专用：脚本化的 Streamer 替身，供 agent / service / api 各层复用。
package llmtest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"WorkBaby/backend/llm"
)

// Scripted 按脚本逐轮返回响应，不联网即可驱动内核与上层编排。
type Scripted struct {
	// Turns 是依次返回的轮次；用尽后返回 StopError。
	Turns []llm.Message
	// Stops 覆盖每轮的 stop_reason，缺省按有无工具调用推断。
	Stops []string
	// Usage 是每轮上报的 token 用量，缺省 Total=2。
	Usage *llm.Usage
	// Err 非空时该轮以 EventError 结束，用于验证错误收尾。
	Err error
	// ErrAfterDelta 非空时先流出该增量再报错，模拟「流到一半断掉」。
	ErrAfterDelta string
	// Requests 按顺序记下每次上游请求：断言「下发的参数」只能从这里看。
	Requests []llm.Request

	idx   int
	calls int32
	mu    sync.Mutex
}

// New 造一个脚本替身。
func New(turns ...llm.Message) *Scripted { return &Scripted{Turns: turns} }

// Calls 返回已发起的流式请求次数。
func (s *Scripted) Calls() int { return int(atomic.LoadInt32(&s.calls)) }

// Stream 实现 llm.Streamer。
func (s *Scripted) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	i := s.idx
	s.idx++
	atomic.AddInt32(&s.calls, 1)
	s.mu.Lock()
	s.Requests = append(s.Requests, req)
	s.mu.Unlock()

	ch := make(chan llm.Event, 8)
	go func() {
		defer close(ch)
		if i >= len(s.Turns) {
			ch <- llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: errors.New("脚本已用完")}
			return
		}
		m := s.Turns[i]
		if s.ErrAfterDelta != "" {
			ch <- llm.Event{Type: llm.EventDelta, Delta: s.ErrAfterDelta}
			ch <- llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: s.Err}
			return
		}
		if s.Err != nil {
			ch <- llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: s.Err}
			return
		}
		if m.Thinking != "" {
			ch <- llm.Event{Type: llm.EventThinking, Delta: m.Thinking}
		}
		if m.Content != "" {
			ch <- llm.Event{Type: llm.EventDelta, Delta: m.Content}
		}
		for _, tc := range m.ToolCalls {
			cp := tc
			ch <- llm.Event{Type: llm.EventToolCall, ToolCall: &cp}
		}
		stop := llm.StopStop
		if i < len(s.Stops) && s.Stops[i] != "" {
			stop = s.Stops[i]
		} else if len(m.ToolCalls) > 0 {
			stop = llm.StopToolUse
		}
		u := s.Usage
		if u == nil {
			u = &llm.Usage{Input: 1, Output: 1, Total: 2}
		}
		ch <- llm.Event{Type: llm.EventDone, StopReason: stop, Usage: u}
	}()
	return ch, nil
}
