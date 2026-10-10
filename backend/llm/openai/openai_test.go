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

// 适配层链路：请求编码的协议家族差异（推理家族 vs 普通模型）与流式解析的三条底线
// （usage 帧顺序、工具调用单次下发、断流必须收尾）。
func TestOpenAIAdapter(t *testing.T) {
	// 推理家族只认 max_completion_tokens 且拒绝采样参数，用错字段直接 400；
	// 网关前缀（openai/gpt-5）不参与判定，否则同一模型换个网关就 400。
	t.Run("输出上限字段按协议家族选择", func(t *testing.T) {
		temp := 0.25
		for _, c := range []struct {
			model     string
			reasoning bool
		}{
			{model: "o3-mini", reasoning: true},
			{model: "openai/gpt-5", reasoning: true},
			{model: "deepseek-chat"},
		} {
			body := decodeBody(t, llm.Request{Model: c.model, MaxTokens: 1000, Temperature: &temp})
			newField, ok := body["max_completion_tokens"].(float64)
			oldField, hasOld := body["max_tokens"].(float64)
			_, hasTemp := body["temperature"]
			if c.reasoning {
				if !ok || int(newField) != 1000 || hasOld || hasTemp {
					t.Fatalf("%s: 推理家族只认 max_completion_tokens 且拒绝采样参数，实际 %v", c.model, body)
				}
				continue
			}
			if !hasOld || int(oldField) != 1000 || ok || !hasTemp {
				t.Fatalf("%s: 普通模型应用 max_tokens 并正常下发采样参数，实际 %v", c.model, body)
			}
		}
	})

	// 流式解析：usage 是独立一帧排在 finish_reason 之后（提前收尾会让计量与水位全变 0），
	// finish_reason 那一帧定型工具调用但不能吞掉 usage 帧、也不能重复下发。
	t.Run("流式解析：usage 帧与工具调用", func(t *testing.T) {
		usage := serve(t,
			`data: {"choices":[{"delta":{"content":"你好"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":128,"completion_tokens":37,"total_tokens":165,`+
				`"cache_read_input_tokens":400,"prompt_tokens_details":{"cached_tokens":450}}}`,
			`data: [DONE]`,
		)
		var done llm.Event
		for _, ev := range collect(t, usage) {
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

		called := serve(t,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ls","arguments":"{}"}}]}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			`data: [DONE]`,
		)
		var calls []llm.ToolCall
		done = llm.Event{}
		for _, ev := range collect(t, called) {
			if ev.Type == llm.EventToolCall && ev.ToolCall != nil {
				calls = append(calls, *ev.ToolCall)
			}
			if ev.Type == llm.EventDone {
				done = ev
			}
		}
		if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "ls" {
			t.Fatalf("工具调用应恰好下发 1 次且内容正确，实际 %+v", calls)
		}
		if done.Usage == nil || done.Usage.Total != 15 {
			t.Fatalf("工具调用收尾时 usage 丢了: %+v", done.Usage)
		}

		// 上游中途断流（没有 finish_reason、没有 usage）也要以收尾事件结束，
		// 否则内核一直等下去，界面表现为「助手卡住不动」；断流前的内容照常送达。
		broken := serve(t, `data: {"choices":[{"delta":{"content":"半句"}}]}`)
		events := collect(t, broken)
		var sawText bool
		for _, ev := range events {
			if ev.Type == llm.EventDelta && ev.Delta == "半句" {
				sawText = true
			}
		}
		if !sawText || len(events) == 0 || events[len(events)-1].Type != llm.EventDone {
			t.Fatalf("断流后应保留已送达内容并以收尾事件结束，实际 %d 条、末尾 %v", len(events), events[len(events)-1].Type)
		}
	})
}
