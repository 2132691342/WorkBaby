package agent

import (
	"sync"

	"WorkBaby/internal/llm"
)

// Queue 是插话队列。内核只认「轮间注入」一种语义——
// 叫它插话还是排队由 UI 决定，内核不需要为此分叉。
type Queue struct {
	mu    sync.Mutex
	items []llm.Message
}

// NewQueue 构造空队列。
func NewQueue() *Queue { return &Queue{} }

// Enqueue 入队。
func (q *Queue) Enqueue(m llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, m)
}

// Drain 取队首一条：用户连打三条消息不会被一次性糊给模型。
func (q *Queue) Drain() []llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	out := []llm.Message{q.items[0]}
	q.items = q.items[1:]
	return out
}

// HasItems 报告队列是否还有内容。
func (q *Queue) HasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) > 0
}
