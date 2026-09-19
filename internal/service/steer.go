package service

import (
	"sync"

	"WorkBaby/internal/llm"
)

// steerQueue 会话级注入队列：run 中进行时用户新发的消息先入队，由 harness 的
// steering（本轮工具跑完后）与 follow-up（本轮收尾后）两条注入缝消费；随 run 结束清空。
type steerQueue struct {
	mu   sync.Mutex
	msgs map[string][]*llm.Message
}

func newSteerQueue() *steerQueue {
	return &steerQueue{msgs: map[string][]*llm.Message{}}
}

// push 入队一条消息。
func (q *steerQueue) push(sessionID string, msg *llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.msgs[sessionID] = append(q.msgs[sessionID], msg)
}

// drain 取走该会话全部排队消息（注入缝消费；取空即删桶）。
func (q *steerQueue) drain(sessionID string) []*llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.msgs[sessionID]
	if len(out) == 0 {
		return nil
	}
	delete(q.msgs, sessionID)
	return out
}

// clear 丢弃该会话排队消息（run 结束 / 被取消）。
func (q *steerQueue) clear(sessionID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.msgs, sessionID)
}
