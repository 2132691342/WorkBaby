package service

import (
	"sync"

	"WorkBaby/backend/domain"
)

// Emitter 是事件的唯一出口：注入会话归属、分配 seq，再交给 sink 推送。
// 同一会话内「分配 seq」与「投递」必须是一次原子步：只在锁内分号的话，
// seq 大的那条可能先到，前端按 seq 去重会把先到的当成旧事件丢掉。
type Emitter struct {
	mu    sync.Mutex
	seq   map[string]int64
	lanes map[string]*sync.Mutex
	sink  func(sessionID string, env domain.Envelope)
	onCut func(sessionID string)
}

// laneOf 取会话的串行化锁；按会话分道而不是全局一把，
// 免得一个慢订阅者把别的会话的事件出口一起堵住。
func (e *Emitter) laneOf(sessionID string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	lk := e.lanes[sessionID]
	if lk == nil {
		lk = &sync.Mutex{}
		e.lanes[sessionID] = lk
	}
	return lk
}

// SetCutHook 注册「会话回收」回调：transport 侧的缓冲同样按会话记账，
// 但 service 不能反向依赖 server，只能由 HTTP 层装配进来。
func (e *Emitter) SetCutHook(f func(sessionID string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onCut = f
}

// Forget 丢掉会话的 seq 计数并通知 transport 层清自己的缓冲；
// 会话删除时必须调用，否则两张 per-session 表只增不减。
func (e *Emitter) Forget(sessionID string) {
	e.mu.Lock()
	delete(e.seq, sessionID)
	delete(e.lanes, sessionID)
	cut := e.onCut
	e.mu.Unlock()
	if cut != nil {
		cut(sessionID)
	}
}

// NewEmitter 构造事件出口。
func NewEmitter() *Emitter {
	return &Emitter{seq: map[string]int64{}, lanes: map[string]*sync.Mutex{}}
}

// SetSink 装配推送目标（由 HTTP 层的 SSE Hub 提供）。
func (e *Emitter) SetSink(f func(sessionID string, env domain.Envelope)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sink = f
}

// Emit 发出一个事件并返回带 seq 的信封，便于日志与测试断言。
func (e *Emitter) Emit(sessionID, event string, data any) domain.Envelope {
	lk := e.laneOf(sessionID)
	lk.Lock()
	defer lk.Unlock()

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
