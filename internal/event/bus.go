// Package event 是应用内事件总线：发布-订阅；订阅可按事件名精确或通配前缀（"chat.*"）注册。
//
// 本包不感知 Wails，纯 Go；api 层做"应用内事件 → runtime.EventsEmit"的桥接（见 api/handler.go）。
package event

import (
	"sync"
)

// Handler 是订阅回调；payload 由发布者构造的任意 map[string]any 或结构体。
type Handler func(event string, payload any)

// Predicate 决定一条事件是否派发给该 Handler；返回 true 即派发。
type Predicate func(event string) bool

// Bus 是一个无界的发布订阅中心；handler 在调用方 goroutine 内同步执行。
type Bus struct {
	mu     sync.RWMutex
	subs   []sub
	nextID int
}

type sub struct {
	id      int
	pred    Predicate
	handler Handler
}

// New 返回默认 Bus。
func New() *Bus { return &Bus{} }

// Subscribe 注册订阅；返回取消函数。
func (b *Bus) Subscribe(pred Predicate, h Handler) func() {
	b.mu.Lock()
	b.nextID++
	s := sub{id: b.nextID, pred: pred, handler: h}
	b.subs = append(b.subs, s)
	id := b.nextID
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		out := b.subs[:0]
		for _, x := range b.subs {
			if x.id != id {
				out = append(out, x)
			}
		}
		b.subs = out
	}
}

// Publish 派发到所有匹配订阅。
func (b *Bus) Publish(event string, payload any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.pred(event) {
			s.handler(event, payload)
		}
	}
}

// MatchExact 精确匹配 helper。
func MatchExact(name string) Predicate { return func(e string) bool { return e == name } }

// MatchPrefix 前缀匹配 helper（如 "chat." 匹配 "chat.stream"）。
func MatchPrefix(prefix string) Predicate {
	return func(e string) bool {
		if len(e) < len(prefix) {
			return false
		}
		return e[:len(prefix)] == prefix
	}
}
