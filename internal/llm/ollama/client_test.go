package ollama

// 线协议一致性测试：假上游回放 ollama ndjson 流，断言流解析的归一化输出
// （一次性完整 tool call、thinking 字段、done 帧用量与结束原因）。

import (
	"context"
	"testing"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/llmtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func drainStream(t *testing.T, ch <-chan llm.StreamChunk) []llm.StreamChunk {
	t.Helper()
	var out []llm.StreamChunk
	deadline := time.After(3 * time.Second)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, c)
		case <-deadline:
			t.Fatalf("stream not closed in time")
		}
	}
}

// TestStreamConformance ollama ndjson 的解析契约。
func TestStreamConformance(t *testing.T) {
	t.Run("text_thinking_tool_call_usage", func(t *testing.T) {
		srv := llmtest.NDJSON(t,
			`{"model":"qwen","message":{"role":"assistant","content":"你好"}}`,
			`{"model":"qwen","message":{"role":"assistant","thinking":"想一想"}}`,
			`{"model":"qwen","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"get_time","arguments":{"tz":"Asia/Shanghai"}}}]}}`,
			`{"model":"qwen","message":{"role":"assistant"},"done":true,"done_reason":"stop","prompt_eval_count":50,"eval_count":9,"cache_read_count":10}`,
		)
		c := New("test", srv.URL)
		ch, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "qwen"})
		require.NoError(t, err)
		chunks := drainStream(t, ch)

		var content, thinking string
		var calls []llm.NormalizedToolCall
		var usage *llm.TokenUsage
		var finish string
		for _, c := range chunks {
			if c.Err != nil {
				t.Fatalf("unexpected stream error: %v", c.Err)
			}
			content += c.Delta.Content
			thinking += c.Delta.Thinking
			if c.ToolCall != nil {
				calls = append(calls, *c.ToolCall)
			}
			if c.FinalUsage != nil {
				usage = c.FinalUsage
			}
			if c.FinishReason != nil {
				finish = *c.FinishReason
			}
		}
		assert.Equal(t, "你好", content)
		assert.Equal(t, "想一想", thinking)
		require.Len(t, calls, 1, "ollama 工具调用一次性完整产出")
		assert.Equal(t, "c1", calls[0].ID)
		assert.Equal(t, "get_time", calls[0].Name)
		assert.JSONEq(t, `{"tz":"Asia/Shanghai"}`, string(calls[0].Arguments))
		assert.Equal(t, "stop", finish)
		require.NotNil(t, usage)
		assert.Equal(t, 50, usage.InputTokens)
		assert.Equal(t, 10, usage.CacheReadTokens)
		assert.Equal(t, 9, usage.OutputTokens)
		assert.Equal(t, 59, usage.TotalTokens)
	})
}
