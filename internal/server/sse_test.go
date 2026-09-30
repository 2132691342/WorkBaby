// SSE 广播中心的慢消费者护栏：delta 可挤、关键事件不可丢。
// 前端被判定为慢消费者时，丢掉 done/error 的表现是「永远停在运行中」。
package server

import (
	"testing"

	"WorkBaby/internal/domain"
)

func TestPublishSqueezesDeltasKeepsDone(t *testing.T) {
	hub := NewHub()
	c := hub.Subscribe("s1")
	defer hub.Unsubscribe(c)

	// 灌满缓冲：全是可以挤掉的 delta
	for i := 0; i < clientBuffer; i++ {
		hub.Publish("s1", domain.Envelope{Seq: int64(i + 1), Event: domain.EventChatDelta, Data: domain.DeltaData{Delta: "x"}})
	}
	// 再推一条 done：必须能挤掉 delta 后送达，而不是把客户端断开
	hub.Publish("s1", domain.Envelope{Seq: int64(clientBuffer + 1), Event: domain.EventChatDone, Data: domain.DoneData{}})

	var got []domain.Envelope
	for {
		select {
		case ev, ok := <-c.Events():
			if !ok {
				t.Fatal("通道提前关闭：done 事件无法送达")
			}
			got = append(got, ev)
			continue
		default:
		}
		break
	}
	deltas := 0
	done := false
	for _, ev := range got {
		switch ev.Event {
		case domain.EventChatDelta:
			deltas++
		case domain.EventChatDone:
			done = true
		}
	}
	if !done {
		t.Fatalf("缓冲满后 done 事件丢失：前端会永远停在「运行中」（收到 %d 条）", len(got))
	}
	if deltas != 1 {
		t.Fatalf("积压 delta 应挤到只剩最后一条，实际 %d", deltas)
	}
}

func TestPublishCoalescesDeltas(t *testing.T) {
	hub := NewHub()
	c := hub.Subscribe("s1")
	defer hub.Unsubscribe(c)

	// 三条同类 delta 应合并成一条（80ms 窗口内），正文语义等价
	for _, txt := range []string{"你", "好", "呀"} {
		hub.Publish("s1", domain.Envelope{Seq: 1, Event: domain.EventChatDelta,
			Data: domain.DeltaData{EntryID: "e1", Kind: "text", Delta: txt}})
	}
	// 非 delta 事件：先冲刷挂起 delta，再发自己
	hub.Publish("s1", domain.Envelope{Seq: 2, Event: domain.EventChatDone, Data: domain.DoneData{}})

	var deltas, dones int
	var text string
	for {
		select {
		case ev := <-c.Events():
			switch ev.Event {
			case domain.EventChatDelta:
				deltas++
				if d, ok := ev.Data.(domain.DeltaData); ok {
					text += d.Delta
				}
			case domain.EventChatDone:
				dones++
			}
			continue
		default:
		}
		break
	}
	if deltas != 1 || text != "你好呀" {
		t.Fatalf("delta 应合并为一条且正文完整，实际 %d 条 text=%q", deltas, text)
	}
	if dones != 1 {
		t.Fatalf("done 应送达 1 次，实际 %d", dones)
	}
}

func TestReplayReturnsEventsAfterSeq(t *testing.T) {
	hub := NewHub()
	for i := 1; i <= 5; i++ {
		hub.Publish("s", domain.Envelope{Seq: int64(i), Event: domain.EventChatDelta})
	}
	got := hub.Replay("s", 3)
	if len(got) != 2 || got[0].Seq != 4 || got[1].Seq != 5 {
		t.Fatalf("重放应返回 seq>3 的事件，实际 %+v", got)
	}
}
