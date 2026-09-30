// 上游流的三条硬约束：停滞要能收尾、事件不许丢、取消要立刻生效。
// 这三条坏任何一条，症状都是「莫名卡住」或「工具跑了但界面停住」。
package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"WorkBaby/internal/llm"
)

// stallingServer 先吐一条增量，然后掐住连接不发也不关——模拟代理停滞。
func stallingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"好\"}}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
}

func TestStreamRecoversFromIdleStall(t *testing.T) {
	old := llm.StreamIdleTimeout
	llm.StreamIdleTimeout = 300 * time.Millisecond
	defer func() { llm.StreamIdleTimeout = old }()

	srv := stallingServer(t)
	defer srv.Close()

	c := New(llm.ClientConfig{BaseURL: srv.URL, APIKey: "k"})
	events, err := c.Stream(context.Background(), llm.Request{Model: "m", MaxTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	// 收尾判据：必须以 EventError 结束且通道关闭，不能永远等下去。
	sawErr := false
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if !sawErr {
					t.Fatal("流结束但没上报空闲超时错误")
				}
				return
			}
			if ev.Type == llm.EventError && ev.Err != nil {
				sawErr = true
			}
		case <-deadline:
			t.Fatal("上游停滞 5 秒仍未收尾：空闲看门狗失效")
		}
	}
}

// bufferedEvents 慢消费事件：只收不读，模拟内核被落库拖慢。
func TestStreamDoesNotDropEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 200; i++ {
			_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n"))
		}
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	c := New(llm.ClientConfig{BaseURL: srv.URL, APIKey: "k"})
	events, err := c.Stream(context.Background(), llm.Request{Model: "m", MaxTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	deltas := 0
	done := false
	for ev := range events {
		switch ev.Type {
		case llm.EventDelta:
			deltas++
		case llm.EventDone:
			done = true
		}
	}
	// 旧实现的 select/default 会静默丢满缓冲后的事件，EventDone 一丢就状态错乱。
	if deltas != 200 || !done {
		t.Fatalf("事件被丢弃：delta=%d/200 done=%v", deltas, done)
	}
}
