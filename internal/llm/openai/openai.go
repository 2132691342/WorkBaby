// Package openai 适配 OpenAI Chat Completions 及全部兼容服务。
package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// Client 是一个 OpenAI 兼容端点。
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New 构造客户端；baseURL 为空时回落到官方地址。
func New(cfg llm.ClientConfig) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &Client{baseURL: base, apiKey: cfg.APIKey, http: &http.Client{Timeout: 0}}
}

type chatMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []toolCallOut `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type toolCallOut struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type toolDefOut struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolDefOut  `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
	StreamOpts  *streamOpts   `json:"stream_options,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string       `json:"content"`
			Reasoning string       `json:"reasoning_content"`
			ToolCalls []toolCallIn `json:"tool_calls"`
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
		// 部分厂商把缓存命中挂在 usage 顶层（Anthropic 口径的 cache_read_input_tokens），
		// 同一份响应里两种写法都出现过，两个都认。
		CacheReadInputTokens int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type toolCallIn struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Stream 发起流式请求；失败时返回错误，上层据此决定重试或放弃。
func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	body, err := c.encode(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", body)
	if err != nil {
		return nil, pkg.Wrap(3001, "构造请求失败", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, pkg.Wrap(3002, "调用模型服务失败", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, readError(resp)
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

// pendingCall 累积一个工具调用的增量片段；OpenAI 把 id / name / args 拆在多个 delta 里下发。
type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

func (c *Client) consume(ctx context.Context, body io.Reader, events chan llm.Event, started time.Time) {
	pendingCalls := map[int]*pendingCall{}
	usage := &llm.Usage{}
	var think llm.ThinkSplitter
	stop := ""

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			usage.LatencyMs = time.Since(started).Milliseconds()
			emit(events, llm.Event{Type: llm.EventDone, StopReason: llm.StopAborted, Usage: usage})
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
		if payload == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			emit(events, llm.Event{Type: llm.EventError, StopReason: llm.StopError,
				Err: pkg.New(3106, chunk.Error.Message, "")})
			return
		}
		if chunk.Usage != nil {
			usage.Input = chunk.Usage.PromptTokens
			usage.Output = chunk.Usage.CompletionTokens
			usage.Total = chunk.Usage.TotalTokens
			if d := chunk.Usage.PromptTokensDetails; d != nil {
				usage.Cached = d.CachedTokens
			}
			if n := chunk.Usage.CacheReadInputTokens; n > usage.Cached {
				usage.Cached = n
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		if d.Reasoning != "" {
			emit(events, llm.Event{Type: llm.EventThinking, Delta: d.Reasoning})
		}
		if d.Content != "" {
			// 部分模型没有独立的 reasoning 字段，直接把 <think>…</think> 写进
			// content。不拆开的话用户会看到裸标签，思考过程也永远折叠不了。
			t, body := think.Split(d.Content)
			if t != "" {
				emit(events, llm.Event{Type: llm.EventThinking, Delta: t})
			}
			if body != "" {
				emit(events, llm.Event{Type: llm.EventDelta, Delta: body})
			}
		}
		for _, tc := range d.ToolCalls {
			p, ok := pendingCalls[tc.Index]
			if !ok {
				p = &pendingCall{}
				pendingCalls[tc.Index] = p
			}
			if tc.ID != "" {
				p.id = tc.ID
			}
			if tc.Function.Name != "" {
				p.name += tc.Function.Name
			}
			p.args.WriteString(tc.Function.Arguments)
		}
		if fr := chunk.Choices[0].FinishReason; fr != nil {
			stop = mapReason(*fr)
			for _, idx := range sortedIndexes(pendingCalls) {
				p := pendingCalls[idx]
				emit(events, llm.Event{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{
					ID:   p.id,
					Name: p.name,
					Args: decodeArgs(p.args.String()),
				}})
			}
			// 这里不能收尾：usage 是独立一帧，排在 finish_reason 之后、[DONE] 之前。
			// 提前 return 会把整帧丢掉，计量与上下文水位从此全是 0。
			// 清空是为了万一上游又补发 delta 时不重复下发同一个工具调用。
			pendingCalls = map[int]*pendingCall{}
		}
	}
	if stop == "" {
		stop = llm.StopStop
	}
	usage.LatencyMs = time.Since(started).Milliseconds()
	emit(events, llm.Event{Type: llm.EventDone, StopReason: stop, Usage: usage})
}

func mapReason(reason string) string {
	switch reason {
	case "length":
		return llm.StopLength
	case "tool_calls":
		return llm.StopToolUse
	case "stop":
		return llm.StopStop
	default:
		return llm.StopStop
	}
}

func decodeArgs(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// sortedIndexes 按 index 升序遍历，保证工具调用顺序与模型意图一致。
func sortedIndexes(m map[int]*pendingCall) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func emit(events chan llm.Event, e llm.Event) {
	select {
	case events <- e:
	default:
	}
}

func (c *Client) encode(req llm.Request) (io.Reader, error) {
	msgs := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		cm := chatMessage{Role: m.Role, Content: m.Content}
		switch m.Role {
		case llm.RoleTool:
			cm.ToolCallID = m.ToolCallID
		case llm.RoleAssistant:
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Args)
				cm.ToolCalls = append(cm.ToolCalls, toolCallOut{
					ID: tc.ID, Type: "function",
					Function: functionCall{Name: tc.Name, Arguments: string(args)},
				})
			}
			if len(m.ToolCalls) > 0 && cm.Content == "" {
				cm.Content = ""
			}
		}
		msgs = append(msgs, cm)
	}
	var tools []toolDefOut
	for _, t := range req.Tools {
		tools = append(tools, toolDefOut{Type: "function", Function: functionDef{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	payload := chatRequest{
		Model: req.Model, Messages: msgs, Tools: tools, Stream: true,
		StreamOpts: &streamOpts{IncludeUsage: true},
		MaxTokens:  req.MaxTokens, Temperature: req.Temperature,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, pkg.Wrap(3001, "序列化请求失败", err)
	}
	return strings.NewReader(string(raw)), nil
}

func readError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return pkg.New(mapStatus(resp.StatusCode), "模型服务返回错误", msg)
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
