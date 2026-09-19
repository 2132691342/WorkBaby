package openai

// 线协议一致性测试：假上游回放 OpenAI 兼容 SSE 帧，断言流解析的归一化输出。
// 协议漂移（字段改名/分帧变化）在这里最先暴露，而不是在用户对话里。

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

// TestStreamConformance OpenAI 兼容 SSE 的解析契约。
func TestStreamConformance(t *testing.T) {
	t.Run("text_thinking_usage", func(t *testing.T) {
		srv := llmtest.SSE(t,
			`data: {"choices":[{"delta":{"content":"你好"}}]}`,
			`data: {"choices":[{"delta":{"reasoning_content":"思考中"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":30}}}`,
			`data: [DONE]`,
		)
		c := New("test", srv.URL, "sk-test")
		ch, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "m"})
		require.NoError(t, err)
		chunks := drainStream(t, ch)

		var content, thinking string
		var usage *llm.TokenUsage
		var finish string
		for _, c := range chunks {
			if c.Err != nil {
				t.Fatalf("unexpected stream error: %v", c.Err)
			}
			content += c.Delta.Content
			thinking += c.Delta.Thinking
			if c.FinalUsage != nil {
				usage = c.FinalUsage
			}
			if c.FinishReason != nil {
				finish = *c.FinishReason
			}
		}
		assert.Equal(t, "你好", content)
		assert.Equal(t, "思考中", thinking)
		assert.Equal(t, "stop", finish)
		require.NotNil(t, usage)
		// 缓存是输入的子维度：Input 为完整 prompt 口径
		assert.Equal(t, 100, usage.InputTokens)
		assert.Equal(t, 30, usage.CacheReadTokens)
		assert.Equal(t, 20, usage.OutputTokens)
		assert.Equal(t, 120, usage.TotalTokens)
	})

	t.Run("tool_call_accumulated_across_deltas", func(t *testing.T) {
		srv := llmtest.SSE(t,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"a.t"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"xt\",\"content\":\"hi\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		)
		c := New("test", srv.URL, "sk-test")
		ch, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "m"})
		require.NoError(t, err)
		chunks := drainStream(t, ch)

		var calls []llm.NormalizedToolCall
		for _, c := range chunks {
			if c.ToolCall != nil {
				calls = append(calls, *c.ToolCall)
			}
		}
		require.Len(t, calls, 1, "跨帧分片必须累积为一次完整调用")
		assert.Equal(t, "c1", calls[0].ID)
		assert.Equal(t, "write_file", calls[0].Name)
		assert.JSONEq(t, `{"path":"a.txt","content":"hi"}`, string(calls[0].Arguments))
	})

	t.Run("rate_limit_error_carries_retry_after", func(t *testing.T) {
		h := http.Header{}
		h.Set("Retry-After", "7")
		srv := llmtest.Error(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, h)
		c := New("test", srv.URL, "sk-test")
		_, err := c.Stream(context.Background(), &llm.ChatRequest{Model: "m"})
		require.Error(t, err)
		assert.Equal(t, 7*time.Second, llm.RetryAfter(err), "Retry-After 必须结构化抵达重试器")
		assert.True(t, llm.IsTransient(err), "429 必须归入瞬时类")
	})
}
