// Anthropic 适配层链路：无签名 thinking 不能回传（厂商特有硬约束）、
// 上游停滞经空闲看门狗收尾、事件缓冲满不丢弃。
// 坏了的表现：多轮对话从第二轮起卡死；上游停滞时界面永远停在「正在思考」；长回答丢字。
package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"WorkBaby/backend/llm"
)

// bodyOf 把请求编码后回读成本地结构体，便于按字段断言。
func bodyOf(t *testing.T, req llm.Request) requestBody {
	t.Helper()
	c := New(llm.ClientConfig{APIKey: "k"})
	raw, err := json.Marshal(c.encode(req))
	if err != nil {
		t.Fatal(err)
	}
	var body requestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

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

// 适配层链路：请求编码的厂商硬约束（无签名 thinking 不回传）与流式解析的两条底线
// （停滞要看门狗收尾、缓冲满不丢事件）。
func TestAnthropicAdapter(t *testing.T) {
	// 无签名 thinking 回传会被网关以 invalid_request_error 拒绝，
	// 症状是多轮对话从第二轮起「答完一轮就卡死」。
	t.Run("协议适配：thinking 编码 / 缓冲背压 / 停滞看门狗", func(t *testing.T) {
		body := bodyOf(t, llm.Request{Model: "m", Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "你好"},
			{Role: llm.RoleAssistant, Thinking: "思考过程", Content: "回答"},
			{Role: llm.RoleUser, Content: "继续"},
		}})
		for _, m := range body.Messages {
			for _, b := range m.Content {
				if b.Type == "thinking" {
					t.Fatal("无签名的 thinking 块被回传，网关会以 invalid_request_error 拒绝")
				}
			}
		}
		// 背压与停滞：收得慢时一帧都不能丢（一次吐 200 帧，通道只有 64 个位置）；
		// 上游停滞必须经看门狗以 error 收尾并关通道，不能永远等下去。
		burst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < 200; i++ {
				_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n"))
			}
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		}))
		defer burst.Close()

		bc := New(llm.ClientConfig{BaseURL: burst.URL, APIKey: "k"})
		events, err := bc.Stream(context.Background(), llm.Request{Model: "m", MaxTokens: 16})
		if err != nil {
			t.Fatal(err)
		}
		deltas, done := 0, false
		for ev := range events {
			switch ev.Type {
			case llm.EventDelta:
				deltas++
			case llm.EventDone:
				done = true
			}
		}
		if deltas != 200 || !done {
			t.Fatalf("事件被丢弃：delta=%d/200 done=%v", deltas, done)
		}

		old := llm.StreamIdleTimeout()
		llm.SetStreamIdleTimeout(300 * time.Millisecond)
		defer llm.SetStreamIdleTimeout(old)

		stalling := stallingServer(t)
		defer stalling.Close()

		c := New(llm.ClientConfig{BaseURL: stalling.URL, APIKey: "k"})
		stalled, err := c.Stream(context.Background(), llm.Request{Model: "m", MaxTokens: 16})
		if err != nil {
			t.Fatal(err)
		}
		sawErr := false
		deadline := time.After(5 * time.Second)
		for {
			select {
			case ev, ok := <-stalled:
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
	})
}
