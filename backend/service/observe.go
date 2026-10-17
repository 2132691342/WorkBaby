// 会话旁听：把一轮 run 的原生事件采成执行时间线，并记账错误原因。
package service

import (
	"sync"
	"time"

	"WorkBaby/backend/domain"
)

// observation 是一个被旁听的会话：采时间线与错误原因。
// 旁听是只读的——它不改数据、不放行审批，只负责「这一轮到底干了什么」。
type observation struct {
	mu  sync.Mutex
	err string
	// pending 记 tool_start：tool_end 不带工具名，名字只在开始那一刻知道。
	pending map[string]domain.RunStep
	steps   []domain.RunStep
	chars   int
}

func newObservation() *observation { return &observation{} }

func (o *observation) fail(msg string) {
	o.mu.Lock()
	o.err = msg
	o.mu.Unlock()
}

func (o *observation) error() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

// toolStart 记下这次调用的名字与开始时刻。
func (o *observation) toolStart(id, tool, label string) {
	o.mu.Lock()
	if o.pending == nil {
		o.pending = map[string]domain.RunStep{}
	}
	o.pending[id] = domain.RunStep{Kind: domain.StepTool, Name: tool, Label: label, At: time.Now().UnixMilli()}
	o.mu.Unlock()
}

// toolEnd 落一步时间线：耗时用事件自带的 duration，开始时刻从 pending 取。
func (o *observation) toolEnd(id string, ok bool, output string, ms int64) {
	o.mu.Lock()
	st, hit := o.pending[id]
	if hit {
		delete(o.pending, id)
	} else {
		// 没有配对的开始（理论上不会发生）：用现在减耗时当开始，别整步丢掉。
		st = domain.RunStep{Kind: domain.StepTool, At: time.Now().UnixMilli() - ms}
	}
	st.OK, st.Ms, st.Detail = ok, ms, domain.TrimTimelineDetail(output)
	o.steps = append(o.steps, st)
	o.mu.Unlock()
}

// addChars 累加助手正文字符数（按 rune：中文一个字算一个，不按字节）。
func (o *observation) addChars(delta string) {
	n := len([]rune(delta))
	if n == 0 {
		return
	}
	o.mu.Lock()
	o.chars += n
	o.mu.Unlock()
}

// timeline 把采到的时间线编码成一个 JSON 列的值；空时间线给空串（不占空间）。
func (o *observation) timeline() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.chars == 0 && len(o.steps) == 0 {
		return ""
	}
	return domain.EncodeRunTimeline(domain.RunTimeline{Chars: o.chars, Steps: o.steps})
}

// Observer 持有全部旁听记录：它是 ChatTap 的宿主，只做时间线与错误记账。
type Observer struct {
	mu    sync.Mutex
	watch map[string]*observation // 会话 id -> 旁听记录
}

// NewObserver 构造旁听器。
func NewObserver() *Observer { return &Observer{watch: map[string]*observation{}} }

// watchSession 为会话挂一个旁听记录；已存在则复用（同一会话不会有两轮 run 同时跑）。
func (s *Observer) watchSession(sessionID string) *observation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o, ok := s.watch[sessionID]; ok {
		return o
	}
	o := newObservation()
	s.watch[sessionID] = o
	return o
}

// unwatch 摘掉旁听记录并返回它；没挂过返回 nil。
func (s *Observer) unwatch(sessionID string) *observation {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.watch[sessionID]
	delete(s.watch, sessionID)
	return o
}

// Forward 是 ChatService 的事件旁听钩子：只做时间线与错误记账。
func (s *Observer) Forward(sessionID, event string, data any) {
	s.mu.Lock()
	o := s.watch[sessionID]
	s.mu.Unlock()
	if o == nil {
		return
	}
	switch event {
	case domain.EventChatToolStart:
		if d, ok := data.(domain.ToolStartData); ok {
			o.toolStart(d.ToolCallID, d.Tool, d.Label)
		}
	case domain.EventChatToolEnd:
		if d, ok := data.(domain.ToolEndData); ok {
			o.toolEnd(d.ToolCallID, d.OK, d.Output, d.DurationMs)
		}
	case domain.EventChatDelta:
		if d, ok := data.(domain.DeltaData); ok && d.Kind == "text" {
			o.addChars(d.Delta)
		}
	case domain.EventChatError:
		if d, ok := data.(domain.ErrorData); ok {
			o.fail(d.Message)
		}
	}
}
