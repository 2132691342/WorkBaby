// 插话与跟进的共用队列：轮次之间取走，一次一条。
package agent

import (
	"sync"

	"WorkBaby/backend/llm"
)

// queueCap 队列上限：入队者是人手打字的插话，正常永远到不了这个量级。
// 不设上限时脚本或连点可以无限堆积，每条都要在轮间注入进上下文，内存与 token 双爆。
const queueCap = 50

// Queue 是插话队列。内核只认「轮间注入」一种语义——
// 叫它插话还是排队由 UI 决定，内核不需要为此分叉。
type Queue struct {
	mu    sync.Mutex
	items []llm.Message
}

// NewQueue 构造空队列。
func NewQueue() *Queue { return &Queue{} }

// Enqueue 入队；队列满时丢弃最旧的一条并返回 false，让调用方能告知用户。
func (q *Queue) Enqueue(m llm.Message) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	ok := true
	if len(q.items) >= queueCap {
		// 丢弃最旧：保留用户最近的意图比保留最早的更重要。
		// 用 copy 而不是重切片，避免底层数组一直持有已丢弃的消息。
		copy(q.items, q.items[1:])
		q.items = q.items[:len(q.items)-1]
		ok = false
	}
	q.items = append(q.items, m)
	return ok
}

// Drain 取队首一条：用户连打三条消息不会被一次性糊给模型。
func (q *Queue) Drain() []llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	out := []llm.Message{q.items[0]}
	// 前移而不是重切片：重切片会让底层数组一直攥着已消费的消息
	//（可能带 base64 图片），长会话下数组只增不减。
	copy(q.items, q.items[1:])
	q.items = q.items[:len(q.items)-1]
	return out
}

// HasItems 报告队列是否还有内容。
func (q *Queue) HasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) > 0
}
