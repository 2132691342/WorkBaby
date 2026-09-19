// Package ollama 实现 Ollama 原生 /api/chat（ndjson 流式）和 /api/tags。
//
// 与 OpenAI 兼容层（Ollama v1）的关系：用户配置 baseURL 指向 Ollama 时推荐用
// Kind=openai + baseURL=http://localhost:11434/v1 走 OpenAI 适配，
// 这里仅在 Kind=ollama 时使用。
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/toolcall"
	"WorkBaby/internal/pkg"
)

// Client Ollama 原生客户端。
type Client struct {
	providerName string
	BaseURL      string
	HTTPClient   *http.Client
}

// New 构造 Ollama 客户端（默认 http://127.0.0.1:11434）。
func New(name, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	return &Client{
		providerName: name,
		BaseURL:      strings.TrimRight(baseURL, "/"),
		HTTPClient:   llm.NewHTTPClient(),
	}
}

func (c *Client) Kind() llm.ProviderKind { return "ollama" }

// Name 实现 llm.Provider 接口。
func (c *Client) Name() string { return c.providerName }

// Chat 非流式（内部仍是发 stream=false，请求体一致）。
func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	body := c.buildBody(req, false)
	resp, err := llm.DoJSON(ctx, c.HTTPClient, "POST", c.BaseURL+"/api/chat", map[string]string{"Content-Type": "application/json"}, body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, pkg.Wrap(3100, "ollama chat", llm.MapHTTPStatus(resp.StatusCode))
	}
	var r OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, pkg.Wrap(3032, "decode ollama response", err)
	}
	return c.toChatResponse(&r)
}

// Stream 流式（ndjson：每行一条 JSON，done:true 终止）。
func (c *Client) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	body := c.buildBody(req, true)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/chat", bytes.NewReader(mustMarshal(body)))
	if err != nil {
		return nil, pkg.Wrap(3031, "build ollama stream", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, pkg.Wrap(3005, "ollama stream http", err)
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		err := pkg.Wrap(3100, "ollama stream", llm.MapHTTPStatus(resp.StatusCode))
		return nil, llm.WithRetryAfter(err, llm.ParseRetryAfter(resp.StatusCode, resp.Header.Get))
	}

	out, send := llm.NewChunkStream(ctx, 32)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		r := bufio.NewReaderSize(resp.Body, 64*1024)
		for {
			line, rerr := r.ReadBytes('\n')
			if rerr != nil {
				if rerr != io.EOF {
					send(llm.StreamChunk{Err: pkg.Wrap(3005, "ollama stream read", rerr)})
				}
				return
			}
			l := strings.TrimSpace(string(line))
			if l == "" {
				continue
			}
			var s OllamaStream
			if err := json.Unmarshal([]byte(l), &s); err != nil {
				if !send(llm.StreamChunk{Err: pkg.Wrap(3033, "decode ollama chunk", err)}) {
					return
				}
				continue
			}
			if s.Message.Content != "" || s.Message.Thinking != "" {
				if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Content: s.Message.Content, Thinking: s.Message.Thinking}}) {
					return
				}
			}
			for _, tc := range s.Message.ToolCalls {
				otc := toolcall.OpenAIToolCall{Index: 0, ID: tc.ID, Type: "function", Function: toolcall.OpenAIFunctionCall{Name: tc.Function.Name, Arguments: toJSON(tc.Function.Arguments)}}
				calls := toolcall.NewAccumulator().Feed(otc)
				for _, c := range calls {
					cc := c
					if !send(llm.StreamChunk{ToolCall: &cc}) {
						return
					}
				}
			}
			if s.Done {
				if s.DoneReason != "" {
					fr := s.DoneReason
					if !send(llm.StreamChunk{FinishReason: &fr}) {
						return
					}
				}
				u := llm.TokenUsage{
					InputTokens:     s.PromptEvalCount,
					OutputTokens:    s.EvalCount,
					CacheReadTokens: s.CacheReadCount,
				}
				// 防御性校正：同 openai/anthropic，避免上游把 cache 报成 input 全部导致命中率虚高。
				if u.CacheReadTokens > u.InputTokens {
					u.CacheReadTokens = 0
				}
				u.TotalTokens = u.InputTokens + u.OutputTokens
				if !send(llm.StreamChunk{FinalUsage: &u}) {
					return
				}
				return
			}
		}
	}()
	return out, nil
}

// Models GET /api/tags。
func (c *Client) Models(ctx context.Context) ([]llm.ModelInfo, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return nil, pkg.Wrap(3005, "ollama /api/tags", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, pkg.Wrap(3100, "ollama /api/tags", llm.MapHTTPStatus(resp.StatusCode))
	}
	var tr OllamaTags
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, pkg.Wrap(3032, "decode ollama tags", err)
	}
	out := make([]llm.ModelInfo, 0, len(tr.Models))
	for _, m := range tr.Models {
		out = append(out, llm.ModelInfo{ID: m.Name})
	}
	return out, nil
}

// Ping 简单 GET 根路径。
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return pkg.Wrap(3005, "ollama ping", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// ollamaImages 抽取图片片段的裸 base64（Ollama 不接受 data URI 前缀）。
func ollamaImages(m *llm.Message) []string {
	out := []string{}
	for _, p := range m.Parts {
		_, data := llm.SplitDataURL(p.ImageDataURL())
		if data != "" {
			out = append(out, data)
		}
	}
	return out
}

func (c *Client) buildBody(req *llm.ChatRequest, stream bool) map[string]any {
	msgs := []map[string]any{}
	for _, m := range req.Messages {
		entry := map[string]any{"role": string(m.Role), "content": m.Content}
		// Ollama 图片走独立的 images 字段（裸 base64，不带 data URI 前缀）
		if imgs := ollamaImages(m); len(imgs) > 0 {
			entry["images"] = imgs
		}
		if len(m.ToolCalls) > 0 {
			entry["tool_calls"] = toolcall.ToOpenAI(toNormalized(m.ToolCalls))
		}
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
		}
		msgs = append(msgs, entry)
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": msgs,
		"stream":   stream,
	}
	if req.Temperature != nil {
		body["options"] = map[string]any{"temperature": *req.Temperature}
	}
	if req.TopP != nil {
		body["options"] = map[string]any{"top_p": *req.TopP}
	}
	if req.MaxTokens != nil {
		body["options"] = map[string]any{"num_predict": *req.MaxTokens}
	}
	if len(req.Tools) > 0 {
		body["tools"] = toOllamaTools(req.Tools)
	}
	for k, v := range req.ExtraBody {
		body[k] = v
	}
	return body
}

func (c *Client) toChatResponse(r *OllamaResponse) (*llm.ChatResponse, error) {
	calls := []llm.NormalizedToolCall{}
	for _, tc := range r.Message.ToolCalls {
		calls = append(calls, llm.NormalizedToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(toJSON(tc.Function.Arguments)),
		})
	}
	usage := llm.TokenUsage{
		InputTokens:  r.PromptEvalCount,
		OutputTokens: r.EvalCount,
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	stop := r.DoneReason
	return &llm.ChatResponse{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: r.Message.Content, Thinking: r.Message.Thinking},
		ToolCalls:  calls,
		Usage:      usage,
		StopReason: stop,
	}, nil
}

// ===== Ollama DTO =====

type OllamaResponse struct {
	Model           string        `json:"model"`
	Message         OllamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

type OllamaStream struct {
	Model           string        `json:"model"`
	Message         OllamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
	CacheReadCount  int           `json:"cache_read_count"`
}

type OllamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Thinking  string           `json:"thinking,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	ID       string         `json:"id"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type OllamaTags struct {
	Models []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"models"`
}

func toOllamaTools(ts []llm.ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.Parameters,
		}})
	}
	return out
}

func toNormalized(calls []llm.ToolCall) []llm.NormalizedToolCall {
	out := make([]llm.NormalizedToolCall, 0, len(calls))
	for i, tc := range calls {
		id := tc.Function.Name + "_" + itoaSimple(i)
		if tc.Type != "" {
			_ = tc.Type
		}
		out = append(out, llm.NormalizedToolCall{
			ID:        id,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	return out
}

func toJSON(v any) string {
	bs, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(bs)
}

func mustMarshal(v any) []byte { bs, _ := json.Marshal(v); return bs }

func itoaSimple(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
