// OpenAI 流式解析链路：usage 帧顺序、工具调用单次下发、断流收尾、缓存计量。
// usage 是独立一帧，排在 finish_reason 之后、[DONE] 之前——提前收尾会让计量与上下文水位全变 0。
package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"WorkBaby/backend/llm"
)

// serve 以固定的 SSE 帧序列应答一次 chat/completions。
func serve(t *testing.T, frames ...string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, frame := range frames {
			fmt.Fprint(w, frame+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return New(llm.ClientConfig{BaseURL: srv.URL})
}

func collect(t *testing.T, c *Client) []llm.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events, err := c.Stream(ctx, llm.Request{Model: "m"})
	if err != nil {
		t.Fatalf("发起流式请求失败: %v", err)
	}
	var out []llm.Event
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func TestOpenAIStreamChain(t *testing.T) {
	t.Run("usage 帧晚于 finish_reason 仍能拿到", func(t *testing.T) {
		// usage 是独立帧，出现在 finish_reason 之后，读不到就永远显示 0（缓存命中率）；
		// 缓存字段有两种写法（顶层与 prompt_tokens_details 下），同时出现取大的。
		c := serve(t,
			`data: {"choices":[{"delta":{"content":"你好"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":128,"completion_tokens":37,"total_tokens":165,`+
				`"cache_read_input_tokens":400,"prompt_tokens_details":{"cached_tokens":450}}}`,
			`data: [DONE]`,
		)
		events := collect(t, c)

		var done llm.Event
		for _, ev := range events {
			if ev.Type == llm.EventDone {
				done = ev
			}
		}
		if done.Usage == nil {
			t.Fatal("收尾事件没有带 usage")
		}
		if done.Usage.Input != 128 || done.Usage.Output != 37 || done.Usage.Total != 165 {
			t.Fatalf("usage 丢了：%+v", *done.Usage)
		}
		if done.Usage.Cached != 450 {
			t.Fatalf("缓存应取两种写法的较大值 450，实际 %d", done.Usage.Cached)
		}
		if done.StopReason != llm.StopStop {
			t.Fatalf("停止原因应为 stop，实际 %q", done.StopReason)
		}
	})

	// finish_reason 那一帧要定型工具调用，但不能因此把 usage 帧吞掉，也不能重复下发。
	t.Run("工具调用单次下发", func(t *testing.T) {
		c := serve(t,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ls","arguments":"{}"}}]}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			`data: [DONE]`,
		)
		events := collect(t, c)

		var calls []llm.ToolCall
		var done llm.Event
		for _, ev := range events {
			if ev.Type == llm.EventToolCall && ev.ToolCall != nil {
				calls = append(calls, *ev.ToolCall)
			}
			if ev.Type == llm.EventDone {
				done = ev
			}
		}
		if len(calls) != 1 {
			t.Fatalf("工具调用应恰好下发 1 次，实际 %d", len(calls))
		}
		if calls[0].ID != "call_1" || calls[0].Name != "ls" {
			t.Fatalf("工具调用内容不对: %+v", calls[0])
		}
		if done.Usage == nil || done.Usage.Total != 15 {
			t.Fatalf("工具调用收尾时 usage 丢了: %+v", done.Usage)
		}
	})

	// 上游中途断流（没有 finish_reason、没有 usage）也要收到收尾事件，
	// 否则内核会一直等下去，界面上表现为「助手卡住不动」。
	t.Run("断流仍收尾", func(t *testing.T) {
		c := serve(t,
			`data: {"choices":[{"delta":{"content":"半句"}}]}`,
		)
		events := collect(t, c)

		if len(events) == 0 {
			t.Fatal("断流后一个事件都没有")
		}
		if events[len(events)-1].Type != llm.EventDone {
			t.Fatalf("最后一个事件应是收尾，实际 %v", events[len(events)-1].Type)
		}
	})
}
