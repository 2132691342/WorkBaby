// Package ollama 适配 Ollama 原生 /api/chat 流式协议（NDJSON，非 SSE）。
package ollama

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

// Client 是一个本地或远程 Ollama 服务。
type Client struct {
	baseURL string
	http    *http.Client
}

// New 构造客户端；baseURL 为空时回落到本机默认端口。
func New(cfg llm.ClientConfig) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return &Client{baseURL: base, http: &http.Client{}}
}

type messageIn struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	ToolCalls []toolCallIn  `json:"tool_calls,omitempty"`
	// Images 是 base64 图片列表（不含 data: 前缀），Ollama 的多模态形态
	Images []string `json:"images,omitempty"`
}

type toolCallIn struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type requestBody struct {
	Model    string      `json:"model"`
	Messages []messageIn `json:"messages"`
	Tools    []any       `json:"tools,omitempty"`
	Stream   bool        `json:"stream"`
	System   string      `json:"system,omitempty"`
	Options  *options    `json:"options,omitempty"`
}

type options struct {
	NumPredict  int      `json:"num_predict,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
}

type chunk struct {
	Message struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Thinking  string `json:"thinking"`
		ToolCalls []struct {
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done           bool   `json:"done"`
	DoneReason     string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount      int    `json:"eval_count"`
	Error          string `json:"error"`
}

// Stream 发起流式请求。
func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	body := c.encode(req)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, pkg.Wrap(3001, "序列化请求失败", err)
	}
	// 请求挂在 Feed 的内部 ctx 上：空闲看门狗取消它才能真正掐断连接。
	feed := llm.NewFeed(ctx, 64)
	httpReq, err := http.NewRequestWithContext(feed.Ctx(), http.MethodPost, c.baseURL+"/api/chat", strings.NewReader(string(raw)))
	if err != nil {
		return nil, pkg.Wrap(3001, "构造请求失败", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
				Body:       trim(string(b)),
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
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	// 工具调用序号跨 chunk 累计：按当前 chunk 的列表长度取号，
	// 调用分多个 chunk 下发时会生成重复 ID，声明与结果就配对错乱了。
	callSeq := 0
	for scanner.Scan() {
		feed.Ping()
		if ctx.Err() != nil && !feed.TimedOut() {
			feed.Send(llm.Event{Type: llm.EventDone, StopReason: llm.StopAborted})
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ck chunk
		if err := json.Unmarshal([]byte(line), &ck); err != nil {
			continue
		}
		if ck.Error != "" {
			feed.Send(llm.Event{Type: llm.EventError, StopReason: llm.StopError,
				Err: errString(llm.FriendlyUpstreamError(ck.Error))})
			return
		}
		if ck.Message.Thinking != "" {
			feed.Send(llm.Event{Type: llm.EventThinking, Delta: ck.Message.Thinking})
		}
		if ck.Message.Content != "" {
			feed.Send(llm.Event{Type: llm.EventDelta, Delta: ck.Message.Content})
		}
		for i, tc := range ck.Message.ToolCalls {
			id := "call_" + itoa(callSeq) + "_" + itoa(i)
			callSeq++
			feed.Send(llm.Event{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{
				ID: id, Name: tc.Function.Name, Args: tc.Function.Arguments,
			}})
		}
		if ck.PromptEvalCount > 0 {
			usage.Input = ck.PromptEvalCount
		}
		if ck.EvalCount > 0 {
			usage.Output = ck.EvalCount
		}
		if ck.Done {
			usage.Total = usage.Input + usage.Output
			usage.LatencyMs = time.Since(started).Milliseconds()
			feed.Send(llm.Event{Type: llm.EventDone, StopReason: mapReason(ck.DoneReason), Usage: usage})
			return
		}
	}
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
	feed.Send(llm.Event{Type: llm.EventDone, StopReason: llm.StopStop, Usage: usage})
}

func mapReason(reason string) string {
	switch reason {
	case "length":
		return llm.StopLength
	default:
		return llm.StopStop
	}
}

func (c *Client) encode(req llm.Request) requestBody {
	msgs := make([]messageIn, 0, len(req.Messages))
	for _, m := range req.Messages {
		mi := messageIn{Role: m.Role, Content: m.Content}
		for _, im := range m.Images {
			mi.Images = append(mi.Images, im.Base64)
		}
		if m.Role == llm.RoleTool {
			mi.Role = "tool"
		}
		for _, tc := range m.ToolCalls {
			mi.ToolCalls = append(mi.ToolCalls, toolCallIn{})
			mi.ToolCalls[len(mi.ToolCalls)-1].Function.Name = tc.Name
			mi.ToolCalls[len(mi.ToolCalls)-1].Function.Arguments = tc.Args
		}
		msgs = append(msgs, mi)
	}
	body := requestBody{Model: req.Model, Messages: msgs, Stream: true, System: req.System}
	for _, t := range req.Tools {
		var td toolDef
		td.Type = "function"
		td.Function.Name = t.Name
		td.Function.Description = t.Description
		td.Function.Parameters = t.Parameters
		body.Tools = append(body.Tools, td)
	}
	if req.MaxTokens > 0 || req.Temperature != nil || req.TopP != nil {
		body.Options = &options{NumPredict: req.MaxTokens, Temperature: req.Temperature, TopP: req.TopP}
	}
	return body
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

