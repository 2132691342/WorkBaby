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

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
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
	NumPredict int     `json:"num_predict,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", strings.NewReader(string(raw)))
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
		return nil, pkg.New(mapStatus(resp.StatusCode), "模型服务返回错误", trim(string(b)))
	}

	events := make(chan llm.Event, 64)
	started := time.Now()
	go func() {
		defer resp.Body.Close()
		defer close(events)
		c.consume(ctx, resp.Body, events, started)
	}()
	return events, nil
}

func (c *Client) consume(ctx context.Context, body io.Reader, events chan llm.Event, started time.Time) {
	usage := &llm.Usage{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			emit(events, llm.Event{Type: llm.EventDone, StopReason: llm.StopAborted})
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
			emit(events, llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: errString(ck.Error)})
			return
		}
		if ck.Message.Thinking != "" {
			emit(events, llm.Event{Type: llm.EventThinking, Delta: ck.Message.Thinking})
		}
		if ck.Message.Content != "" {
			emit(events, llm.Event{Type: llm.EventDelta, Delta: ck.Message.Content})
		}
		for i, tc := range ck.Message.ToolCalls {
			id := "call_" + itoa(len(ck.Message.ToolCalls)) + "_" + itoa(i)
			emit(events, llm.Event{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{
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
			emit(events, llm.Event{Type: llm.EventDone, StopReason: mapReason(ck.DoneReason), Usage: usage})
			return
		}
	}
	usage.Total = usage.Input + usage.Output
	usage.LatencyMs = time.Since(started).Milliseconds()
	emit(events, llm.Event{Type: llm.EventDone, StopReason: llm.StopStop, Usage: usage})
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
	if req.MaxTokens > 0 || req.Temperature > 0 {
		body.Options = &options{NumPredict: req.MaxTokens, Temperature: req.Temperature}
	}
	return body
}

func emit(events chan llm.Event, e llm.Event) {
	select {
	case events <- e:
	default:
	}
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

func mapStatus(code int) int {
	switch {
	case code == 401 || code == 403:
		return 3104
	case code == 404:
		return 3105
	case code == 429:
		return 3106
	case code >= 500:
		return 3107
	default:
		return 3103
	}
}
