// Package anthropic 适配 Anthropic Messages 流式协议。
package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
)

const apiVersion = "2023-06-01"

// Client 是一个 Anthropic 端点。
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New 构造客户端；baseURL 为空时回落到官方地址。
func New(cfg llm.ClientConfig) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &Client{baseURL: base, apiKey: cfg.APIKey, http: &http.Client{}}
}

type block struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	Thinking string         `json:"thinking,omitempty"`
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
	ToolUseID string        `json:"tool_use_id,omitempty"`
	Content  string         `json:"content,omitempty"`
	IsError  bool           `json:"is_error,omitempty"`
	// 图片块（识图输入）：type=image 时必填
	Source *imageSource `json:"source,omitempty"`
}

// imageSource 是 base64 内联的图片来源。
type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type messageIn struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type requestBody struct {
	Model       string      `json:"model"`
	System      string      `json:"system,omitempty"`
	Messages    []messageIn `json:"messages"`
	Tools       []toolDef   `json:"tools,omitempty"`
	MaxTokens   int         `json:"max_tokens"`
	Stream      bool        `json:"stream"`
	Temperature *float64    `json:"temperature,omitempty"`
	TopP        *float64    `json:"top_p,omitempty"`
}

type event struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	ContentBlock *struct {
		Type  string         `json:"type"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
		Text  string         `json:"text"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta,omitempty"`
	Message *struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Stream 发起流式请求。
func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	if c.apiKey == "" {
		return nil, pkg.Wrap(3104, "缺少 API Key", llm.ErrNoAPIKey)
	}
	raw, err := json.Marshal(c.encode(req))
	if err != nil {
		return nil, pkg.Wrap(3001, "序列化请求失败", err)
	}
	// 请求挂在 Feed 的内部 ctx 上：空闲看门狗取消它才能真正掐断连接。
	feed := llm.NewFeed(ctx, 64)
	httpReq, err := http.NewRequestWithContext(feed.Ctx(), http.MethodPost, c.baseURL+"/v1/messages", strings.NewReader(string(raw)))
	if err != nil {
		return nil, pkg.Wrap(3001, "构造请求失败", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, pkg.Wrap(3002, "调用模型服务失败", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, pkg.Wrap(llm.MapStatus(resp.StatusCode), "模型服务返回错误",
			&llm.StatusError{
				Status:     resp.StatusCode,
				RetryAfter: llm.ParseRetryAfter(resp.Header),
				Body:       trim(llm.FriendlyUpstreamError(string(b))),
			})
	}

	started := time.Now()
	go func() {
		defer resp.Body.Close()
		defer feed.Close()
		c.consume(feed, resp.Body, started)
	}()
	go feed.WatchIdle()
	return feed.Events(), nil
}

func (c *Client) consume(feed *llm.Feed, body io.Reader, started time.Time) {
	ctx := feed.Ctx()
	usage := &llm.Usage{}
	calls := map[int]*llm.ToolCall{}
	argsBuf := map[int]*strings.Builder{}
	stop := llm.StopStop

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		feed.Ping()
		if ctx.Err() != nil && !feed.TimedOut() {
			feed.Send(llm.Event{Type: llm.EventDone, StopReason: llm.StopAborted})
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		if ev.Error != nil {
			feed.Send(llm.Event{Type: llm.EventError, StopReason: llm.StopError,
				Err: pkg.New(3106, llm.FriendlyUpstreamError(ev.Error.Message), "")})
			return
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				usage.Input = ev.Message.Usage.InputTokens
				usage.Cached = ev.Message.Usage.CacheReadInputTokens
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				calls[ev.Index] = &llm.ToolCall{ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
				argsBuf[ev.Index] = &strings.Builder{}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				feed.Send(llm.Event{Type: llm.EventDelta, Delta: ev.Delta.Text})
			case "thinking_delta":
				feed.Send(llm.Event{Type: llm.EventThinking, Delta: ev.Delta.Thinking})
			case "input_json_delta":
				if b, ok := argsBuf[ev.Index]; ok {
					b.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if tc, ok := calls[ev.Index]; ok {
				tc.Args = decodeArgs(argsBuf[ev.Index])
				feed.Send(llm.Event{Type: llm.EventToolCall, ToolCall: tc})
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				stop = mapReason(ev.Delta.StopReason)
			}
			if ev.Usage != nil {
				usage.Output = ev.Usage.OutputTokens
			}
		case "message_stop":
			usage.Total = usage.Input + usage.Output
			usage.LatencyMs = time.Since(started).Milliseconds()
			feed.Send(llm.Event{Type: llm.EventDone, StopReason: stop, Usage: usage})
			return
		}
	}
	// 读失败分三类：看门狗掐的、调用方取消的、连接真断了。只有前两类之外才该报错。
	if err := scanner.Err(); err != nil {
		switch {
		case feed.TimedOut():
			feed.Send(llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: llm.IdleError()})
		case ctx.Err() == nil:
			feed.Send(llm.Event{Type: llm.EventError, StopReason: llm.StopError,
				Err: pkg.Wrap(3002, "模型服务连接中断", err)})
		}
		return
	}
	if ctx.Err() != nil && !feed.TimedOut() {
		return
	}
	usage.Total = usage.Input + usage.Output
	usage.LatencyMs = time.Since(started).Milliseconds()
	feed.Send(llm.Event{Type: llm.EventDone, StopReason: stop, Usage: usage})
}

func mapReason(reason string) string {
	switch reason {
	case "max_tokens":
		return llm.StopLength
	case "tool_use":
		return llm.StopToolUse
	default:
		return llm.StopStop
	}
}

func decodeArgs(b *strings.Builder) map[string]any {
	if b == nil || strings.TrimSpace(b.String()) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// encode 组装请求体，守住 Anthropic 协议的四条硬约束：
// 无签名 thinking 不回传、内容不为空串、角色严格交替（合并同角色）、首条是 user。
func (c *Client) encode(req llm.Request) requestBody {
	msgs := make([]messageIn, 0, len(req.Messages))
	merge := func(role string, blocks []block) {
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Content = append(msgs[n-1].Content, blocks...)
			return
		}
		msgs = append(msgs, messageIn{Role: role, Content: blocks})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case llm.RoleTool:
			content := m.Content
			if strings.TrimSpace(content) == "" {
				content = "（无输出）"
			}
			merge("user", []block{{
				Type: "tool_result", ToolUseID: m.ToolCallID, Content: content, IsError: m.IsError,
			}})
		case llm.RoleAssistant:
			blocks := []block{}
			if m.Content != "" {
				blocks = append(blocks, block{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, block{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Args})
			}
			if len(blocks) > 0 {
				merge("assistant", blocks)
			}
		default:
			text := m.Content
			if strings.TrimSpace(text) == "" {
				text = "（继续）"
			}
			blocks := []block{{Type: "text", Text: text}}
			// 识图输入：base64 内联图片块，放在文本之后
			for _, im := range m.Images {
				blocks = append(blocks, block{
					Type: "image",
					Source: &imageSource{
						Type: "base64", MediaType: im.MIME, Data: im.Base64,
					},
				})
			}
			merge("user", blocks)
		}
	}
	// 首条必须是 user：压缩或异常路径可能让历史以 assistant 开头
	if len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = append([]messageIn{{Role: "user", Content: []block{{Type: "text", Text: "（继续）"}}}}, msgs...)
	}
	var tools []toolDef
	for _, t := range req.Tools {
		tools = append(tools, toolDef{Name: t.Name, Description: t.Description, InputSchema: t.Parameters})
	}
	// max_tokens 在 anthropic 协议里是必填字段，不能省。0 说明上层没填好，
	// 用一个够用的兜底值而不是 4096——推理型模型会把它全花在思考上。
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = llm.DefaultMaxTokens
	}
	return requestBody{
		Model: req.Model, System: req.System, Messages: msgs, Tools: tools,
		MaxTokens: maxTokens, Stream: true,
		Temperature: req.Temperature, TopP: req.TopP,
	}
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

