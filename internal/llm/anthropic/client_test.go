package anthropic

// 线协议一致性测试：假上游回放 Anthropic 事件流，断言流解析的归一化输出
// （块生命周期、thinking 分离、tool_use 参数累积、缓存口径归一）。

import (
	"context"
	"net/http"
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

// TestStreamConformance Anthropic 事件流的解析契约。
func TestStreamConformance(t *testing.T) {
	t.Run("text_thinking_tool_call_and_usage", func(t *testing.T) {
		srv := llmtest.SSE(t,
			`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":1}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"想一想"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu1","name":"get_weather"}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"北京\"}"}}`,
			`data: {"type":"content_block_stop","index":2}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":4,"cache_creation_input_tokens":2}}`,
			`data: {"type":"message_stop"}`,
		)
		c := New("test", srv.URL, "sk-ant")
		ch, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "claude"})
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
		assert.Equal(t, "Hello", content)
		assert.Equal(t, "想一想", thinking, "thinking 与正文走独立通道")
		require.Len(t, calls, 1, "tool_use 块闭合时产出完整调用")
		assert.Equal(t, "tu1", calls[0].ID)
		assert.Equal(t, "get_weather", calls[0].Name)
		assert.JSONEq(t, `{"city":"北京"}`, string(calls[0].Arguments))
		assert.Equal(t, "tool_use", finish)
		require.NotNil(t, usage)
		// Anthropic input_tokens 不含缓存；归一化为「完整 prompt」口径（10+4+2=16）
		assert.Equal(t, 16, usage.InputTokens)
		assert.Equal(t, 4, usage.CacheReadTokens)
		assert.Equal(t, 2, usage.CacheWriteTokens)
		assert.Equal(t, 21, usage.TotalTokens)
	})

	t.Run("rate_limit_error_carries_retry_after", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "5")
		srv := llmtest.Error(t, http.StatusTooManyRequests, `{"error":{"message":"rate limited"}}`, h)
		c := New("test", srv.URL, "sk-ant")
		_, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "claude"})
		require.Error(t, err)
		assert.Equal(t, 5*time.Second, llm.RetryAfter(err))
		assert.True(t, llm.IsTransient(err))
	})
}
