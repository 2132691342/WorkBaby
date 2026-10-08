// OpenAI 适配层链路：请求编码的协议家族差异（推理家族 vs 普通模型），
// 以及流式解析的硬约束（usage 帧顺序、工具调用单次下发、断流收尾）。
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// decodeBody 编码一次请求并解回 map，逐字段断言。
func decodeBody(t *testing.T, req llm.Request) map[string]any {
	t.Helper()
	c := New(llm.ClientConfig{BaseURL: "http://127.0.0.1:1"})
	body, err := c.encode(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// 编码协议：推理家族只认 max_completion_tokens 且拒绝采样参数，用错字段直接 400。
func TestEncodeProtocolFamilies(t *testing.T) {
	temp := 0.25

	t.Run("推理家族用 max_completion_tokens", func(t *testing.T) {
		body := decodeBody(t, llm.Request{Model: "o3-mini", MaxTokens: 1000, Temperature: &temp})
		if _, ok := body["max_tokens"]; ok {
			t.Fatal("o 系模型不应下发 max_tokens，会直接 400")
		}
		if _, ok := body["temperature"]; ok {
			t.Fatal("o 系模型不应下发 temperature，会直接 400")
		}
		if got, ok := body["max_completion_tokens"].(float64); !ok || int(got) != 1000 {
			t.Fatalf("max_completion_tokens 缺失或不对: %v", body["max_completion_tokens"])
		}
	})

	t.Run("普通模型仍用 max_tokens", func(t *testing.T) {
		body := decodeBody(t, llm.Request{Model: "deepseek-chat", MaxTokens: 1000, Temperature: &temp})
		if got, ok := body["max_tokens"].(float64); !ok || int(got) != 1000 {
			t.Fatalf("max_tokens 缺失或不对: %v", body["max_tokens"])
		}
		if _, ok := body["max_completion_tokens"]; ok {
			t.Fatal("普通模型不应下发 max_completion_tokens")
		}
		if _, ok := body["temperature"]; !ok {
			t.Fatal("普通模型的 temperature 应正常下发")
		}
	})

	t.Run("网关前缀不影响家族判定", func(t *testing.T) {
		body := decodeBody(t, llm.Request{Model: "openai/gpt-5", MaxTokens: 500})
		if _, ok := body["max_completion_tokens"]; !ok {
			t.Fatal("带网关前缀的 gpt-5 应命中推理协议")
		}
	})
}

// 流式解析：usage 是独立一帧排在 finish_reason 之后，提前收尾会让计量与水位全变 0。
func TestStreamChain(t *testing.T) {
	t.Run("usage 帧晚于 finish_reason 仍能拿到", func(t *testing.T) {
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
