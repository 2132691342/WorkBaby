package service

import (
	"sync"

	"WorkBaby/internal/domain"
)

// Emitter 是事件的唯一出口：注入会话归属、分配 seq，再交给 sink 推送。
type Emitter struct {
	mu   sync.Mutex
	seq  map[string]int64
	sink func(sessionID string, env domain.Envelope)
}

// NewEmitter 构造事件出口。
func NewEmitter() *Emitter { return &Emitter{seq: map[string]int64{}} }

// SetSink 装配推送目标（由 HTTP 层的 SSE Hub 提供）。
func (e *Emitter) SetSink(f func(sessionID string, env domain.Envelope)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sink = f
}

// Emit 发出一个事件并返回带 seq 的信封，便于日志与测试断言。
func (e *Emitter) Emit(sessionID, event string, data any) domain.Envelope {
	e.mu.Lock()
	e.seq[sessionID]++
	seq := e.seq[sessionID]
	sink := e.sink
	e.mu.Unlock()

	env := domain.Envelope{Seq: seq, Event: event, Data: data}
	if sink != nil {
		sink(sessionID, env)
	}
	return env
}

// Seq 取会话当前 seq，供断线重放定位。
func (e *Emitter) Seq(sessionID string) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seq[sessionID]
}
