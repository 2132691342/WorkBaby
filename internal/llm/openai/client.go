// Package openai 实现 OpenAI 兼容协议（/v1/chat/completions）；覆盖 DeepSeek / Moonshot / 智谱 / GLM / LM Studio / Ollama 兼容层。
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/toolcall"
	"WorkBaby/internal/pkg"
)

// Client OpenAI 兼容 HTTP 客户端；自研、不用 SDK。
type Client struct {
	providerName  string
	BaseURL       string
	APIKey        string
	HTTPClient    *http.Client
	thinkingStyle llm.ThinkingStyle
}

// WithThinkingStyle 指定思维参数方言（由 Provider 配置的 thinking_style 或自动探测得出）。
func (c *Client) WithThinkingStyle(s llm.ThinkingStyle) *Client {
	c.thinkingStyle = s
	return c
}

// New 构造 OpenAI 兼容客户端。BaseURL 由调用方完整提供（含版本段，如 /v1）；
// client 层不做补全或关键字嗅探，路径不合规由上游明确报错便于排查。
func New(name, baseURL, apiKey string) *Client {
	return &Client{
		providerName: name,
		BaseURL:      strings.TrimRight(baseURL, "/"),
		APIKey:       apiKey,
		HTTPClient:   llm.NewHTTPClient(),
	}
}

func (c *Client) Kind() llm.ProviderKind { return "openai" }

// Name 实现 llm.Provider 接口。
func (c *Client) Name() string { return c.providerName }

// Chat 非流式。
func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	body := c.buildBody(req, false)
	resp, err := llm.DoJSON(ctx, c.HTTPClient, "POST", c.BaseURL+"/chat/completions", c.headers(), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, llm.NewUpstreamError("模型服务返回", resp.StatusCode, raw)
	}
	var oc OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&oc); err != nil {
		return nil, pkg.Wrap(3032, "decode openai response failed", err)
	}
	return c.toChatResponse(&oc)
}

// Stream 流式。
func (c *Client) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	body := c.buildBody(req, true)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(mustMarshal(body)))
	if err != nil {
		return nil, pkg.Wrap(3031, "build stream request failed", err)
	}
	for k, v := range c.headers() {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, pkg.Wrap(3005, "openai stream failed", err)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		appErr := llm.NewUpstreamError("模型服务返回", resp.StatusCode, raw)
		return nil, llm.WithRetryAfter(appErr, llm.ParseRetryAfter(resp.StatusCode, resp.Header.Get))
	}

	out, send := llm.NewChunkStream(ctx, 32)
	acc := toolcall.NewAccumulator()

	go func() {
		defer close(out)
		defer resp.Body.Close()
		r := bufio.NewReaderSize(resp.Body, 64*1024)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					send(llm.StreamChunk{Err: pkg.Wrap(3005, "stream read failed", err)})
				}
				return
			}
			l := strings.TrimSpace(string(line))
			if l == "" {
				continue
			}
			if !strings.HasPrefix(l, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(l, "data:"))
			if data == "[DONE]" {
				// 流结束兜底：把未闭合的 tool call 全部吐出
				for _, c := range acc.Dump() {
					tc := c
					if !send(llm.StreamChunk{ToolCall: &tc}) {
						return
					}
				}
				return
			}
			var s OpenAIStreamResponse
			if err := json.Unmarshal([]byte(data), &s); err != nil {
				if !send(llm.StreamChunk{Err: pkg.Wrap(3033, "decode stream chunk failed", err)}) {
					return
				}
				continue
			}
			for _, ch := range s.Choices {
				delta := llm.Message{Role: llm.RoleAssistant}
				if ch.Delta.Content != "" {
					delta.Content = ch.Delta.Content
				}
				if ch.Delta.ReasoningContent != "" {
					delta.Thinking = ch.Delta.ReasoningContent
				}
				if len(delta.Content) > 0 || len(delta.Thinking) > 0 {
					if !send(llm.StreamChunk{Delta: delta}) {
						return
					}
				}
				for _, t := range ch.Delta.ToolCalls {
					otc := toolcall.OpenAIToolCall{Index: t.Index, ID: t.ID, Type: t.Type, Function: toolcall.OpenAIFunctionCall{Name: t.Function.Name, Arguments: t.Function.Arguments}}
					done := acc.Feed(otc)
					for _, c := range done {
						tc := c
						if !send(llm.StreamChunk{ToolCall: &tc}) {
							return
						}
					}
				}
				if ch.FinishReason != nil {
					fr := *ch.FinishReason
					if !send(llm.StreamChunk{FinishReason: &fr}) {
						return
					}
				}
			}
			if s.Usage != nil {
				cached := 0
				if s.Usage.PromptTokensDetails != nil {
					cached = s.Usage.PromptTokensDetails.CachedTokens
				}
				// 与 Anthropic 口径对齐：InputTokens = 完整 prompt（含缓存读），
				// 缓存是输入的子维度。这样 dashboard 的「实际计费」= InputTokens - CacheReadTokens 在两家协议下同口径。
				u := llm.TokenUsage{
					InputTokens:     s.Usage.PromptTokens,
					OutputTokens:    s.Usage.CompletionTokens,
					TotalTokens:     s.Usage.TotalTokens,
					CacheReadTokens: cached,
				}
				// 防御性校正：GLM 等 OpenAI 兼容实现偶发把 cached_tokens 报成等于 prompt_tokens，
				// 这会让前端命中率永远 100%、token 总数虚高。清零 + warn，让仪表盘数据真实。
				if u.CacheReadTokens > u.InputTokens {
					pkg.L.Warn("openai usage: cache_read exceeds input (server misreport); clamping to 0",
						"input", u.InputTokens, "cache_read", u.CacheReadTokens)
					u.CacheReadTokens = 0
				}
				if !send(llm.StreamChunk{FinalUsage: &u}) {
					return
				}
			}
		}
	}()
	return out, nil
}

// Models 拉取 /v1/models。
func (c *Client) Models(ctx context.Context) ([]llm.ModelInfo, error) {
	var resp OpenAIModelsResponse
	if _, err := llm.DoJSON(ctx, c.HTTPClient, "GET", c.BaseURL+"/models", c.headers(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]llm.ModelInfo, 0, len(resp.Data))
	for _, m := range resp.Data {
		out = append(out, llm.ModelInfo{ID: m.ID})
	}
	return out, nil
}

// Ping 连通性测试：发最小 chat 请求并消费 stream 头。
func (c *Client) Ping(ctx context.Context) error {
	req := &llm.ChatRequest{
		Model:     "ping",
		Messages:  []*llm.Message{llm.UserMessage("ping")},
		MaxTokens: intPtr(1),
	}
	body := c.buildBody(req, false)
	resp, err := llm.DoJSON(ctx, c.HTTPClient, "POST", c.BaseURL+"/chat/completions", c.headers(), body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// 某些 provider 不接受 "ping" model 名；用 400/404 也算连通（看到 server 就行）
		if resp.StatusCode == 400 || resp.StatusCode == 404 || resp.StatusCode == 422 {
			return nil
		}
		raw, _ := io.ReadAll(resp.Body)
		return llm.NewUpstreamError("连通测试失败", resp.StatusCode, raw)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) headers() map[string]string {
	h := map[string]string{"Authorization": "Bearer " + c.APIKey}
	return h
}

// buildBody 构造请求体；合并 ExtraBody；thinking 转换。
//
// 不做 temperature 钳位：GLM 等部分厂商要求 ≤1，由 provider 自身校验并
// 返回明确错误（400 + message）；settings 层 input 框:max=2 兜底。
func (c *Client) buildBody(req *llm.ChatRequest, stream bool) map[string]any {
	body := map[string]any{
		"model":    req.Model,
		"messages": toReqMessages(req.Messages),
		"stream":   stream,
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}
	if len(req.Stop) > 0 {
		body["stop"] = req.Stop
	}
	// 思维参数按方言渲染：未知上游默认不发（none），避免不被识别的字段直接 400。
	for k, v := range llm.RenderThinking(c.thinkingStyle, req.Thinking) {
		body[k] = v
	}
	for k, v := range req.ExtraBody {
		body[k] = v
	}
	if len(req.Tools) > 0 {
		body["tools"] = toReqTools(req.Tools)
	}
	if req.User != "" {
		body["user"] = req.User
	}
	// 上游自动按前缀缓存；prompt_cache_key 让同一会话固定落在同一缓存分片，避免被其他流量挤掉。
	// 只有官方端点认这个字段，第三方兼容层传未知字段可能 400，按 host 判定后再发。
	if req.SessionID != "" && isOfficialEndpoint(c.BaseURL) {
		body["prompt_cache_key"] = req.SessionID
	}
	return body
}

// isOfficialEndpoint 判断 BaseURL 是否指向官方 OpenAI（第三方兼容层不认 prompt_cache_key）。
func isOfficialEndpoint(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "api.openai.com"
}

func (c *Client) toChatResponse(r *OpenAIResponse) (*llm.ChatResponse, error) {
	if len(r.Choices) == 0 {
		return nil, pkg.New(3034, "openai returned no choices", "")
	}
	ch := r.Choices[0]
	msg := llm.Message{Role: llm.RoleAssistant, Content: ch.Message.Content, Thinking: ch.Message.ReasoningContent}
	usage := llm.TokenUsage{InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens, TotalTokens: r.Usage.TotalTokens}
	if r.Usage != nil && r.Usage.PromptTokensDetails != nil {
		usage.CacheReadTokens = r.Usage.PromptTokensDetails.CachedTokens
	}
	// 防御性校正：同 ChatStream，避免上游误报导致命中率 100%。
	if usage.CacheReadTokens > usage.InputTokens {
		pkg.L.Warn("openai usage: cache_read exceeds input (server misreport); clamping to 0",
			"input", usage.InputTokens, "cache_read", usage.CacheReadTokens)
		usage.CacheReadTokens = 0
	}
	// OpenAI 兼容协议：prompt_tokens 已经包含 cached_tokens（与 Anthropic 语义不同），
	// 这里保持 InputTokens = prompt_tokens 不变，确保 dashboard 实际计费口径与 Anthropic 一致。
	return &llm.ChatResponse{
		Message:    msg,
		ToolCalls:  toolcall.FromOpenAI(ch.Message.ToolCalls),
		Usage:      usage,
		StopReason: ch.FinishReason,
	}, nil
}

// ===== OpenAI 协议 DTO =====

type reqMessage struct {
	Role       llm.RoleType  `json:"role"`
	Content    any           `json:"content"` // string 或 []reqContentPart（多模态）
	Name       string        `json:"name,omitempty"`
	ToolCalls  []reqToolCall `json:"tool_calls,omitempty"`   // assistant 消息携带
	ToolCallID string        `json:"tool_call_id,omitempty"` // tool 消息携带
}

// reqContentPart OpenAI 多模态片段（text / image_url）。
type reqContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type reqToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type reqTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// openAIContent 有多模态片段时展开为 parts 数组，否则用纯文本（兼容老上游）。
func openAIContent(m *llm.Message) any {
	if len(m.Parts) == 0 {
		return m.Content
	}
	parts := make([]reqContentPart, 0, len(m.Parts)+1)
	if m.Content != "" {
		parts = append(parts, reqContentPart{Type: "text", Text: m.Content})
	}
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			parts = append(parts, reqContentPart{Type: "text", Text: p.Text})
		case "image_url":
			if u := p.ImageDataURL(); u != "" {
				part := reqContentPart{Type: "image_url"}
				part.ImageURL = &struct {
					URL string `json:"url"`
				}{URL: u}
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		return m.Content
	}
	return parts
}

func toReqMessages(ms []*llm.Message) []reqMessage {
	out := make([]reqMessage, 0, len(ms))
	for _, m := range ms {
		rm := reqMessage{Role: m.Role, Content: openAIContent(m), Name: m.Name, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			rt := reqToolCall{ID: tc.ID, Type: tc.Type}
			rt.Function.Name = tc.Function.Name
			rt.Function.Arguments = tc.Function.Arguments
			rm.ToolCalls = append(rm.ToolCalls, rt)
		}
		out = append(out, rm)
	}
	return out
}

func toReqTools(ts []llm.ToolDefinition) []reqTool {
	out := make([]reqTool, 0, len(ts))
	for _, t := range ts {
		var rt reqTool
		rt.Type = "function"
		rt.Function.Name = t.Name
		rt.Function.Description = t.Description
		rt.Function.Parameters = t.Parameters
		out = append(out, rt)
	}
	return out
}

type OpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role             llm.RoleType              `json:"role"`
			Content          string                    `json:"content"`
			ReasoningContent string                    `json:"reasoning_content,omitempty"`
			ToolCalls        []toolcall.OpenAIToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

type OpenAIStreamResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             llm.RoleType `json:"role"`
			Content          string       `json:"content"`
			ReasoningContent string       `json:"reasoning_content,omitempty"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

type OpenAIModelsResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// ===== helpers =====

func mustMarshal(v any) []byte {
	bs, _ := json.Marshal(v)
	return bs
}

func intPtr(i int) *int { return &i }
