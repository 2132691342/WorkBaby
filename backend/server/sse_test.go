// hub 的两条保命语义：慢消费者必须保住 done（丢了就永远「运行中」），
// 同类 delta 必须合流且保序（否则正文打乱）。端到端的重放语义（Last-Event-ID 补帧）
// 在 api 包的流链路测试里验证。
package server

import (
	"testing"

	"WorkBaby/backend/domain"
)

// drain 取走当前已入队的全部事件，不阻塞。
func drain(c *Client) []domain.Envelope {
	var out []domain.Envelope
	for {
		select {
		case ev, ok := <-c.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestHubDeliveryChain(t *testing.T) {
	// 段一：灌满缓冲（全是可挤的 delta）后再推 done，done 必须挤掉 delta 后送达，
	// 而不是把客户端断开——前端被判定为慢消费者时，丢 done 的表现是「永远停在运行中」。
	hubA := NewHub()
	cA := hubA.Subscribe("s1")
	defer hubA.Unsubscribe(cA)
	for i := 0; i < clientBuffer; i++ {
		hubA.Publish("s1", domain.Envelope{Seq: int64(i + 1), Event: domain.EventChatDelta, Data: domain.DeltaData{Delta: "x"}})
	}
	hubA.Publish("s1", domain.Envelope{Seq: int64(clientBuffer + 1), Event: domain.EventChatDone, Data: domain.DoneData{}})

	deltas, done := 0, false
	for _, ev := range drain(cA) {
		switch ev.Event {
		case domain.EventChatDelta:
			deltas++
		case domain.EventChatDone:
			done = true
		}
	}
	if !done {
		t.Fatalf("缓冲满后 done 事件丢失：前端会永远停在「运行中」")
	}
	if deltas != 1 {
		t.Fatalf("积压 delta 应挤到只剩最后一条，实际 %d", deltas)
	}

	// 段二：三条同类 delta 在合流窗口内应合并成一条且正文保序；
	// 非 delta 事件先冲刷挂起 delta，再发自己。
	hubB := NewHub()
	cB := hubB.Subscribe("s1")
	defer hubB.Unsubscribe(cB)
	for _, txt := range []string{"你", "好", "呀"} {
		hubB.Publish("s1", domain.Envelope{Seq: 1, Event: domain.EventChatDelta,
			Data: domain.DeltaData{EntryID: "e1", Kind: "text", Delta: txt}})
	}
	hubB.Publish("s1", domain.Envelope{Seq: 2, Event: domain.EventChatDone, Data: domain.DoneData{}})

	merged, dones := 0, 0
	var text string
	for _, ev := range drain(cB) {
		switch ev.Event {
		case domain.EventChatDelta:
			merged++
			if d, ok := ev.Data.(domain.DeltaData); ok {
				text += d.Delta
			}
		case domain.EventChatDone:
			dones++
		}
	}
	if merged != 1 || text != "你好呀" {
		t.Fatalf("delta 应合并为一条且正文完整，实际 %d 条 text=%q", merged, text)
	}
	if dones != 1 {
		t.Fatalf("done 应送达 1 次，实际 %d", dones)
	}
}
