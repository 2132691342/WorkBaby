// Package sse 是事件广播中心：seq 单调发号、断线按 since 补帧、心跳保活。
// 界面的一切实时状态都从这里出去；服务层只调 Broadcast，不关心谁在听。
package sse

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"WorkBaby/backend/domain"
)

// Frame 是一帧事件。
type Frame struct {
	// Seq 单调递增的帧号；断线补帧按它对齐
	Seq int64 `json:"seq"`
	// Event 事件名（chat:patch 之类）
	Event string `json:"event"`
	// Data 载荷
	Data map[string]any `json:"data"`
	// SentAt 发号时刻（毫秒）
	SentAt int64 `json:"sent_at"`
}

// Sink 是一个订阅者的出口；实现返回 error 表示连接已断，会被立刻摘掉。
type Sink func(frame Frame) error

// Hub 是广播中心。
type Hub struct {
	mu      sync.Mutex
	seq     int64
	buffer  []Frame
	maxKeep int
	sinks   map[int]Sink
	nextID  int
}

// New 造一个广播中心；keep 是断线补帧的保留条数。
func New(keep int) *Hub {
	if keep <= 0 {
		keep = 1000
	}
	return &Hub{buffer: make([]Frame, 0, keep), maxKeep: keep, sinks: map[int]Sink{}}
}

// Subscribe 登记一个订阅者，并把 since 之后缺的帧补发一遍。
// 补不上的缺口由前端拿 chat/state 全量对账——这里不假装补齐。
func (h *Hub) Subscribe(since int64, sink Sink) (cancel func()) {
	h.mu.Lock()
	missed := make([]Frame, 0, len(h.buffer))
	for _, frame := range h.buffer {
		if frame.Seq > since {
			missed = append(missed, frame)
		}
	}
	id := h.nextID
	h.nextID++
	h.sinks[id] = sink
	h.mu.Unlock()

	for _, frame := range missed {
		if err := sink(frame); err != nil {
			h.unsubscribe(id)
			return func() {}
		}
	}
	return func() { h.unsubscribe(id) }
}

func (h *Hub) unsubscribe(id int) {
	h.mu.Lock()
	delete(h.sinks, id)
	h.mu.Unlock()
}

// Broadcast 发一帧；seq 由这里统一发号，调用方不许自己定。
func (h *Hub) Broadcast(event string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	h.mu.Lock()
	h.seq++
	frame := Frame{Seq: h.seq, Event: event, Data: data, SentAt: time.Now().UnixMilli()}
	h.buffer = append(h.buffer, frame)
	if len(h.buffer) > h.maxKeep {
		h.buffer = h.buffer[len(h.buffer)-h.maxKeep:]
	}
	targets := make([]int, 0, len(h.sinks))
	for id := range h.sinks {
		targets = append(targets, id)
	}
	h.mu.Unlock()

	// 逐个写：一个客户端卡住不能拖住其他客户端。
	for _, id := range targets {
		h.mu.Lock()
		sink, ok := h.sinks[id]
		h.mu.Unlock()
		if !ok {
			continue
		}
		if err := sink(frame); err != nil {
			h.unsubscribe(id)
		}
	}
}

// Seq 返回当前帧号：日志与调试用。
func (h *Hub) Seq() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seq
}

// EngineTag 给聊天帧补上引擎标记：界面要能一眼看出这帧属于谁。
func EngineTag(data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	data["engine_kind"] = string(domain.EnginePi)
	return data
}

// Encode 把一帧编成 SSE 记录（默认 message 事件，data 是整帧 JSON）。
func Encode(frame Frame) (string, error) {
	payload, err := json.Marshal(frame)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: message\ndata: %s\n\n", payload), nil
}
