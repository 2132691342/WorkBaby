package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openaiStreamer 走 /chat/completions 的 SSE 流。
// 覆盖全部 OpenAI 兼容端点（DeepSeek / Moonshot / Ollama / vLLM / 各家网关），
// 一个适配器打天下；协议差异只在字段上，不在语义上。
type openaiStreamer struct {
	// http 复用连接池的客户端
	http *http.Client
}

// NewOpenAIStreamer 造一个 OpenAI 兼容适配器。
// 超时不设整体值：一轮回复可能长达几分钟，只能靠 ctx 中止。
func NewOpenAIStreamer() Streamer {
	return &openaiStreamer{http: &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
		},
	}}
}

// wireRequest 是 /chat/completions 的请求体。
type wireRequest struct {
	// Model 模型 ID
	Model string `json:"model"`
	// Messages 上下文
	Messages []wireMessage `json:"messages"`
	// Tools 工具声明；无工具时省掉整个字段
	Tools []wireTool `json:"tools,omitempty"`
	// Stream 恒为真：引擎只走流式
	Stream bool `json:"stream"`
	// StreamOptions 让上游在末尾补一帧用量
	StreamOptions *wireStreamOptions `json:"stream_options,omitempty"`
	// MaxTokens 输出上限
	MaxTokens int `json:"max_tokens,omitempty"`
	// Temperature 采样温度
	Temperature *float64 `json:"temperature,omitempty"`
}

// wireStreamOptions 是流式附加开关。
type wireStreamOptions struct {
	// IncludeUsage 末尾补一帧 usage
	IncludeUsage bool `json:"include_usage"`
}

// wireMessage 是上下文里的一条消息。
type wireMessage struct {
	// Role system / user / assistant / tool
	Role string `json:"role"`
	// Content 文本或内容块数组；纯文本走字符串
	Content any `json:"content"`
	// ToolCalls 助手要求的工具调用
	ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
	// ToolCallId 工具结果对应的调用
	ToolCallId string `json:"tool_call_id,omitempty"`
}

// wireToolCall 是一次工具调用。
type wireToolCall struct {
	// Id 调用 ID
	Id string `json:"id"`
	// Type 恒为 function
	Type string `json:"type"`
	// Function 函数名与入参
	Function wireFunction `json:"function"`
}

// wireFunction 是函数名与入参。
type wireFunction struct {
	// Name 函数名
	Name string `json:"name"`
	// Arguments 入参 JSON 原文
	Arguments string `json:"arguments"`
}

// wireTool 是工具声明。
type wireTool struct {
	// Type 恒为 function
	Type string `json:"type"`
	// Function 函数签名
	Function wireToolFunction `json:"function"`
}

// wireToolFunction 是函数签名。
type wireToolFunction struct {
	// Name 工具名
	Name string `json:"name"`
	// Description 什么时候该用它
	Description string `json:"description"`
	// Parameters 入参 JSON Schema
	Parameters map[string]any `json:"parameters"`
}

// wireChunk 是流式分片。
type wireChunk struct {
	// Choices 候选（只取第一个）
	Choices []struct {
		// Delta 增量
		Delta struct {
			// Content 正文增量
			Content string `json:"content"`
			// ReasoningContent 思考增量（各家字段名不同，统一收口）
			ReasoningContent string `json:"reasoning_content"`
			// Reasoning 思考增量的另一个常见拼法
			Reasoning string `json:"reasoning"`
			// ToolCalls 工具调用增量
			ToolCalls []struct {
				// Index 调用下标：分片靠它归并
				Index int `json:"index"`
				// Id 调用 ID（只在首个分片出现）
				Id string `json:"id"`
				// Function 函数增量
				Function struct {
					// Name 函数名（只在首个分片出现）
					Name string `json:"name"`
					// Arguments 入参增量
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		// FinishReason 结束原因
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage 用量（只有 stream_options.include_usage 才会来）
	Usage *struct {
		// PromptTokens 输入 token
		PromptTokens int `json:"prompt_tokens"`
		// CompletionTokens 输出 token
		CompletionTokens int `json:"completion_tokens"`
		// PromptTokensDetails 输入明细（缓存命中在这里）
		PromptTokensDetails *struct {
			// CachedTokens 命中缓存的输入 token
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	// Error 有的网关用 200 + error 字段报错
	Error *struct {
		// Message 原话
		Message string `json:"message"`
	} `json:"error"`
}

// Stream 发起一次流式调用。
func (s *openaiStreamer) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	body, err := json.Marshal(buildWireRequest(req))
	if err != nil {
		return nil, fmt.Errorf("请求编码失败: %w", err)
	}
	url := strings.TrimSuffix(strings.TrimSpace(req.Model.BaseUrl), "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("请求构造失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if key := strings.TrimSpace(req.Model.ApiKey); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := s.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("连不上接口: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		resp.Body.Close()
		return nil, fmt.Errorf("接口返回 %d：%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	out := make(chan StreamEvent, 64)
	go s.pump(ctx, resp.Body, out)
	return out, nil
}

// buildWireRequest 把归一请求翻成上游请求体。
func buildWireRequest(req Request) wireRequest {
	messages := make([]wireMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, wireMessage{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		messages = append(messages, toWireMessage(msg))
	}
	wire := wireRequest{
		Model:         req.Model.ModelId,
		Messages:      messages,
		Stream:        true,
		StreamOptions: &wireStreamOptions{IncludeUsage: true},
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
	}
	for _, spec := range req.Tools {
		wire.Tools = append(wire.Tools, wireTool{
			Type: "function",
			Function: wireToolFunction{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  spec.Schema,
			},
		})
	}
	return wire
}

// toWireMessage 把一条归一消息翻成上游消息。
func toWireMessage(msg Message) wireMessage {
	switch msg.Role {
	case RoleTool:
		return wireMessage{
			Role:       "tool",
			Content:    msg.Text(),
			ToolCallId: msg.ToolCallId,
		}
	case RoleAssistant:
		calls := msg.ToolCalls()
		if len(calls) == 0 {
			return wireMessage{Role: "assistant", Content: msg.Text()}
		}
		out := wireMessage{Role: "assistant", Content: msg.Text()}
		for _, call := range calls {
			args := call.ToolArgs
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, wireToolCall{
				Id:       call.CallId,
				Type:     "function",
				Function: wireFunction{Name: call.ToolName, Arguments: args},
			})
		}
		return out
	case RoleUser:
		return wireMessage{Role: "user", Content: userContent(msg)}
	}
	return wireMessage{Role: string(msg.Role), Content: msg.Text()}
}

// userContent 组装用户消息的 content：有图片就是内容块数组。
func userContent(msg Message) any {
	parts := make([]map[string]any, 0, len(msg.Blocks))
	for _, block := range msg.Blocks {
		switch block.Kind {
		case KindText:
			parts = append(parts, map[string]any{"type": "text", "text": block.Text})
		case KindImage:
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": "data:" + block.ImageMime + ";base64," + block.ImageBase64},
			})
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// 纯文本仍走字符串：部分兼容端点不认识内容块数组。
	if len(parts) == 1 {
		if text, ok := parts[0]["text"].(string); ok {
			return text
		}
	}
	return parts
}

// pump 读 SSE 分片并翻译成归一事件；结束时关闭通道。
func (s *openaiStreamer) pump(ctx context.Context, body io.ReadCloser, out chan<- StreamEvent) {
	defer close(out)
	defer body.Close()

	// 不指定 buffer：SSE 单帧可能有一整段长文本，默认 64KB 上限会截断。
	reader := bufio.NewReaderSize(body, 1<<20)
	builder := &assistantBuilder{}
	finish := ""

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			payload := bytes.TrimSpace(line)
			// 只认 data: 行；event/id/注释（心跳）一律跳过。
			if after, ok := bytes.CutPrefix(payload, []byte("data:")); ok {
				text := strings.TrimSpace(string(after))
				if text == "[DONE]" {
					break
				}
				if chunk, ok := parseChunk(text); ok {
					if chunk.Error != nil && chunk.Error.Message != "" {
						builder.fail(chunk.Error.Message)
						break
					}
					emitChunk(out, chunk)
					if reason := chunkFinishReason(chunk); reason != "" {
						finish = reason
					}
				}
			}
		}
		if err != nil {
			if err != io.EOF && err != context.Canceled {
				builder.fail("流中断：" + err.Error())
			}
			break
		}
	}
	if finish == "" {
		finish = builder.stop
	}
	if finish == "" {
		finish = "stop"
	}
	// 失败原话要跟着结束事件一起出去：上层据此把错误显示给用户。
	select {
	case out <- StreamEvent{Kind: EventStop, StopReason: finish, Text: builder.errorMessage()}:
	case <-ctx.Done():
	}
}

// parseChunk 解一帧 JSON；坏帧跳过，不打断整条流。
func parseChunk(text string) (wireChunk, bool) {
	var chunk wireChunk
	if err := json.Unmarshal([]byte(text), &chunk); err != nil {
		return chunk, false
	}
	return chunk, true
}

// chunkFinishReason 取分片里的结束原因。
func chunkFinishReason(chunk wireChunk) string {
	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			return choice.FinishReason
		}
	}
	return ""
}

// emitChunk 把一个分片翻成若干归一事件。
// index 一律透传上游的分片下标：并行工具调用的分片可能交错到达，
// 按到达顺序拼入参会把两个函数的参数串成一段非法 JSON。
func emitChunk(out chan<- StreamEvent, chunk wireChunk) {
	if chunk.Usage != nil {
		usage := &Usage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
		}
		if detail := chunk.Usage.PromptTokensDetails; detail != nil {
			usage.CacheReadTokens = detail.CachedTokens
		}
		out <- StreamEvent{Kind: EventUsage, Usage: usage}
	}
	if len(chunk.Choices) == 0 {
		return
	}
	delta := chunk.Choices[0].Delta
	// 正文与思考用 -1 作下标：它们不分片交错，由 builder 按顺序自己切开。
	if text := delta.ReasoningContent + delta.Reasoning; text != "" {
		out <- StreamEvent{Kind: EventThinking, Index: -1, Text: text}
	}
	if text := delta.Content; text != "" {
		out <- StreamEvent{Kind: EventText, Index: -1, Text: text}
	}
	for _, call := range delta.ToolCalls {
		if call.Id != "" || call.Function.Name != "" {
			out <- StreamEvent{Kind: EventToolStart, Index: call.Index, CallId: call.Id, ToolName: call.Function.Name}
		}
		if args := call.Function.Arguments; args != "" {
			out <- StreamEvent{Kind: EventToolDelta, Index: call.Index, Text: args}
		}
	}
	// 分片里带 finish_reason 时不在这里发：循环末尾统一发一次结束事件。
}
