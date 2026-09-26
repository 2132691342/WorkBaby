// Package anthropic 实现 Anthropic Messages API（/v1/messages，SSE 流式）。
// 归一化：thinking 块 → Message.Thinking，tool_use 块 → NormalizedToolCall；
// prompt caching 在 system 末段、工具定义、历史前缀三处打断点。
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

const (
	APIVersion = "2023-06-01"
	// minCacheTokens Anthropic prompt caching 的最小可缓存前缀（Sonnet/Opus 1024 token）。
	// 低于此长度打断点不会命中，只会白占断点配额。
	minCacheTokens = 1024
)

// Client Anthropic Messages API 实现。
type Client struct {
	providerName string
	BaseURL      string // 可改中转；默认 https://api.anthropic.com
	APIKey       string
	HTTP         *http.Client
}

// New 构造客户端。
func New(name, baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		providerName: name,
		BaseURL:      baseURL,
		APIKey:       apiKey,
		HTTP:         llm.NewHTTPClient(),
	}
}

func (c *Client) Kind() llm.ProviderKind { return "anthropic" }

// Name 实现 llm.Provider 接口。
func (c *Client) Name() string { return c.providerName }

// Chat 非流式。
func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	body := c.buildBody(req, false)
	resp, err := llm.DoJSON(ctx, c.HTTP, "POST", c.BaseURL+"/v1/messages", c.headers(), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, pkg.Wrap(3100, "anthropic chat", llm.MapHTTPStatus(resp.StatusCode))
	}
	var ar AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, pkg.Wrap(3032, "decode anthropic response failed", err)
	}
	return c.toChatResponse(&ar)
}

// Stream 流式（SSE，event-stream）。
func (c *Client) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	body := c.buildBody(req, true)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(llm.MustMarshal(body)))
	if err != nil {
		return nil, pkg.Wrap(3031, "build anthropic stream failed", err)
	}
	for k, v := range c.headers() {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, pkg.Wrap(3005, "anthropic stream http failed", err)
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		err := pkg.Wrap(3100, "anthropic stream", llm.MapHTTPStatus(resp.StatusCode))
		return nil, llm.WithRetryAfter(err, llm.ParseRetryAfter(resp.StatusCode, resp.Header.Get))
	}

	out, send := llm.NewChunkStream(ctx, 32)

	type openBlock struct {
		Index int
		Type  string
		ID    string
		Name  string
	}

	go func() {
		defer close(out)
		defer resp.Body.Close()
		opens := map[int]*openBlock{}
		// content block 累积 buffer
		var toolInputBuf strings.Builder
		var textBuf strings.Builder
		var thinkBuf strings.Builder
		var lastUsage *AnthropicUsage
		var stopReason string

		r := bufio.NewReaderSize(resp.Body, 64*1024)
		for {
			line, rerr := r.ReadBytes('\n')
			if rerr != nil {
				if rerr != io.EOF {
					send(llm.StreamChunk{Err: pkg.Wrap(3005, "anthropic stream read", rerr)})
				}
				return
			}
			l := strings.TrimRight(string(line), "\r\n")
			if l == "" {
				continue
			}
			if !strings.HasPrefix(l, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(l, "data:"))
			var ev AnthropicEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			switch ev.Type {
			case "message_start":
				if ev.Message != nil {
					lastUsage = &ev.Message.Usage
				}
			case "content_block_start":
				if ev.ContentBlock != nil {
					opens[ev.Index] = &openBlock{Index: ev.Index, Type: ev.ContentBlock.Type, ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
				}
			case "content_block_delta":
				if ev.Delta == nil {
					continue
				}
				switch ev.Delta.Type {
				case "text_delta":
					if ev.Delta.Text != "" {
						textBuf.WriteString(ev.Delta.Text)
						if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Content: ev.Delta.Text}}) {
							return
						}
					}
				case "thinking_delta":
					thinkBuf.WriteString(ev.Delta.Thinking)
					if ev.Delta.Thinking != "" {
						if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Thinking: ev.Delta.Thinking}}) {
							return
						}
					}
				case "input_json_delta":
					toolInputBuf.WriteString(ev.Delta.PartialJSON)
				case "signature_delta":
					// ignore (Anthropic signature 字段不影响业务)
				}
			case "content_block_stop":
				ob := opens[ev.Index]
				if ob == nil {
					continue
				}
				switch ob.Type {
				case "tool_use":
					argRaw := toolInputBuf.String()
					if argRaw == "" {
						argRaw = "{}"
					}
					toolInputBuf.Reset()
					tc := llm.NormalizedToolCall{ID: ob.ID, Name: ob.Name, Arguments: json.RawMessage(argRaw)}
					if !send(llm.StreamChunk{ToolCall: &tc}) {
						return
					}
				}
			case "message_delta":
				if ev.Delta != nil && ev.Delta.StopReason != "" {
					stopReason = ev.Delta.StopReason
				}
				if ev.Usage != nil {
					lastUsage = ev.Usage
				}
			case "message_stop":
				if stopReason != "" {
					if !send(llm.StreamChunk{FinishReason: &stopReason}) {
						return
					}
				}
				if lastUsage != nil {
					// Anthropic 的 input_tokens 不含缓存读/写；归一化为「完整 prompt」口径
					// （与 OpenAI 的 prompt_tokens 含 cached_tokens 语义对齐，缓存为输入的子维度）
					u := llm.TokenUsage{
						InputTokens:      lastUsage.InputTokens + lastUsage.CacheReadInputTokens + lastUsage.CacheCreationInputTokens,
						OutputTokens:     lastUsage.OutputTokens,
						CacheReadTokens:  lastUsage.CacheReadInputTokens,
						CacheWriteTokens: lastUsage.CacheCreationInputTokens,
					}
					u.TotalTokens = u.InputTokens + u.OutputTokens
					if !send(llm.StreamChunk{FinalUsage: &u}) {
						return
					}
				}
				return
			case "ping", "error":
				// ignore / 已通过 HTTP 状态表达
			}
		}
	}()
	return out, nil
}

// Models Anthropic 没官方 /v1/models；用静态列表兜底（用户配置 model 即可）。
func (c *Client) Models(_ context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{
		{ID: "claude-sonnet-4-5"},
		{ID: "claude-3-7-sonnet-latest"},
		{ID: "claude-3-5-sonnet-latest"},
		{ID: "claude-3-5-haiku-latest"},
		{ID: "claude-opus-4-1"},
	}, nil
}

// Ping 非流式 chat 试 "ping"。
func (c *Client) Ping(ctx context.Context) error {
	body := c.buildBody(&llm.ChatRequest{Model: "claude-3-5-haiku-latest", Messages: []*llm.Message{llm.UserMessage("ping")}, MaxTokens: llm.IntPtr(1)}, false)
	resp, err := llm.DoJSON(ctx, c.HTTP, "POST", c.BaseURL+"/v1/messages", c.headers(), body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// 不识别 model 也算连通
		if resp.StatusCode == 400 || resp.StatusCode == 404 {
			return nil
		}
		return pkg.Wrap(3100, "anthropic ping", llm.MapHTTPStatus(resp.StatusCode))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) headers() map[string]string {
	return map[string]string{
		"x-api-key":         c.APIKey,
		"anthropic-version": APIVersion,
	}
}

// anthropicUserContent 用户消息内容块；图片片段转 Anthropic source(base64) 形态。
func anthropicUserContent(m *llm.Message) []map[string]any {
	parts := make([]map[string]any, 0, len(m.Parts)+1)
	if m.Content != "" {
		parts = append(parts, map[string]any{"type": "text", "text": m.Content})
	}
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			parts = append(parts, map[string]any{"type": "text", "text": p.Text})
		case "image_url":
			u := p.ImageDataURL()
			mime, data := llm.SplitDataURL(u)
			if data == "" {
				continue
			}
			if mime == "" {
				mime = "image/png"
			}
			parts = append(parts, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mime,
					"data":       data,
				},
			})
		}
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"type": "text", "text": m.Content})
	}
	return parts
}

func (c *Client) buildBody(req *llm.ChatRequest, stream bool) map[string]any {
	// system 单独提取（Anthropic Messages API 顶层 system 字段）。
	systemParts := []map[string]any{}
	convo := []map[string]any{}
	var systemText strings.Builder
	for _, m := range req.Messages {
		if m.Role == llm.RoleSystem {
			systemParts = append(systemParts, map[string]any{"type": "text", "text": m.Content})
			systemText.WriteString(m.Content)
			continue
		}
		switch m.Role {
		case llm.RoleUser:
			convo = append(convo, map[string]any{"role": "user", "content": anthropicUserContent(m)})
		case llm.RoleAssistant:
			content := []map[string]any{}
			if m.Thinking != "" {
				content = append(content, map[string]any{"type": "thinking", "thinking": m.Thinking})
			}
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				fn := tc.Function
				argRaw := fn.Arguments
				if argRaw == "" {
					argRaw = "{}"
				}
				content = append(content, map[string]any{"type": "tool_use", "id": tc.ID, "name": fn.Name, "input": json.RawMessage(argRaw)})
			}
			if len(content) == 0 {
				content = append(content, map[string]any{"type": "text", "text": ""})
			}
			convo = append(convo, map[string]any{"role": "assistant", "content": content})
		case llm.RoleTool:
			convo = append(convo, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
		default:
			_ = m.Role
		}
	}
	body := map[string]any{
		"model":      req.Model,
		"messages":   convo,
		"max_tokens": 1024,
		"stream":     stream,
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		body["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		body["stop_sequences"] = req.Stop
	}
	// system 断点只打在最后一段：断点是「缓存到此为止」的前缀语义，一段就覆盖全部 system。
	// 逐段打既浪费配额（上限 4，tools 与历史前缀各占 1）又不会扩大命中面。
	// 低于最小可缓存长度时不打：短 system 缓存无收益，还占用历史前缀的刷新时机。
	if len(systemParts) > 0 {
		if pkg.EstimateTextTokens(systemText.String()) >= minCacheTokens {
			systemParts[len(systemParts)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		}
		body["system"] = systemParts
	}
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		budget := 4096
		if req.Thinking.BudgetTokens > 0 {
			budget = req.Thinking.BudgetTokens
		}
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	}
	if len(req.Tools) > 0 {
		// 工具定义末尾打 cache_control：tools 跨轮稳定（不重排、不改名），前缀命中
		tools := toAnthropicTools(req.Tools)
		if len(tools) > 0 {
			tools[len(tools)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		}
		body["tools"] = tools
	}
	// 最近一条 message 打 cache_control：让 messages 历史前缀（含所有 tool 轮次）稳定命中
	// 这是 Anthropic prompt caching 的关键策略（参见 https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching）
	if len(convo) > 0 {
		last := convo[len(convo)-1]
		if content, ok := last["content"].([]map[string]any); ok && len(content) > 0 {
			content[len(content)-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		}
	}
	for k, v := range req.ExtraBody {
		body[k] = v
	}
	return body
}

func (c *Client) toChatResponse(r *AnthropicResponse) (*llm.ChatResponse, error) {
	content := strings.Builder{}
	thinking := strings.Builder{}
	tools := []llm.NormalizedToolCall{}
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			content.WriteString(b.Text)
		case "thinking":
			thinking.WriteString(b.Thinking)
		case "tool_use":
			raw := string(b.Input)
			if raw == "" {
				raw = "{}"
			}
			tools = append(tools, llm.NormalizedToolCall{ID: b.ID, Name: b.Name, Arguments: json.RawMessage(raw)})
		}
	}
	// 同上：input 归一化为完整 prompt（含缓存读/写），缓存作为输入子维度
	usage := llm.TokenUsage{
		InputTokens:      r.Usage.InputTokens + r.Usage.CacheReadInputTokens + r.Usage.CacheCreationInputTokens,
		OutputTokens:     r.Usage.OutputTokens,
		CacheReadTokens:  r.Usage.CacheReadInputTokens,
		CacheWriteTokens: r.Usage.CacheCreationInputTokens,
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	// 防御性校正：cache_read+cache_write 不应超过 input（输入里 cache 部分是子集）。
	// 万一上游误报，清零避免前端命中率与计费口径失真。
	if usage.CacheReadTokens+usage.CacheWriteTokens > usage.InputTokens {
		pkg.L.Warn("anthropic usage: cache tokens exceed input; clamping to 0",
			"input", usage.InputTokens,
			"cache_read", usage.CacheReadTokens,
			"cache_write", usage.CacheWriteTokens)
		usage.CacheReadTokens = 0
		usage.CacheWriteTokens = 0
	}
	return &llm.ChatResponse{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: content.String(), Thinking: thinking.String()},
		ToolCalls:  tools,
		Usage:      usage,
		StopReason: r.StopReason,
	}, nil
}

// ===== Anthropic DTO =====

type AnthropicResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Role       string `json:"role"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text,omitempty"`
		Thinking string          `json:"thinking,omitempty"`
		ID       string          `json:"id,omitempty"`
		Name     string          `json:"name,omitempty"`
		Input    json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	Usage AnthropicUsage `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type AnthropicEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage AnthropicUsage `json:"usage"`
	} `json:"message,omitempty"`
	Index        int `json:"index,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Usage *AnthropicUsage `json:"usage,omitempty"`
}

func toAnthropicTools(ts []llm.ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.Parameters,
		})
	}
	return out
}

// ===== helpers (mustMarshal / intPtr 已迁出到 internal/llm/providerbase.go) =====
