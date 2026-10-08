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

	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
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

// chatMessage 的 Content 用 any：纯文本是 string，带图片时是 content part 数组
// （OpenAI 多模态形态）。响应解析走 streamChunk，不经过这个结构。
type chatMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []toolCallOut `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

// contentPart 是多模态消息的一个片段：文本或图片。
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
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
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []toolDefOut  `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
	StreamOpts *streamOpts `json:"stream_options,omitempty"`
	// 输出上限二选一：推理家族只认 max_completion_tokens，其余只认 max_tokens。
	MaxTokens           int      `json:"max_tokens,omitempty"`
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	TopP                *float64 `json:"top_p,omitempty"`
}

// reasoningOnly 判断推理协议家族（o1/o3/o4 与 gpt-5 系）：这些模型拒绝
// max_tokens / temperature / top_p。网关透传时模型名即协议契约，前缀不影响。
func reasoningOnly(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:] // 网关常带前缀（openai/o3-mini）
	}
	for _, p := range []string{"o1", "o3", "o4", "gpt-5"} {
		if name == p || strings.HasPrefix(name, p+"-") || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
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
	// 请求挂在 Feed 的内部 ctx 上：空闲看门狗取消它才能真正掐断连接。
	feed := llm.NewFeed(ctx, 64)
	httpReq, err := http.NewRequestWithContext(feed.Ctx(), http.MethodPost, c.baseURL+"/chat/completions", body)
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

	started := time.Now()
	go func() {
		defer resp.Body.Close()
		defer feed.Close()
		c.consume(feed, resp.Body, started)
	}()
	go feed.WatchIdle()
	return feed.Events(), nil
}

// pendingCall 累积一个工具调用的增量片段；OpenAI 把 id / name / args 拆在多个 delta 里下发。
type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

func (c *Client) consume(feed *llm.Feed, body io.Reader, started time.Time) {
	ctx := feed.Ctx()
	pendingCalls := map[int]*pendingCall{}
	usage := &llm.Usage{}
	var think llm.ThinkSplitter
	stop := ""

	scanner := bufio.NewScanner(body)
	// 单行上限 16MB：模型把大段文件内容塞进一个工具参数时会走到这里，
	// 4MB 会在正常的长文件任务上报「连接中断」。
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		feed.Ping()
		if ctx.Err() != nil && !feed.TimedOut() {
			usage.LatencyMs = time.Since(started).Milliseconds()
			feed.Send(llm.Event{Type: llm.EventDone, StopReason: llm.StopAborted, Usage: usage})
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
			feed.Send(llm.Event{Type: llm.EventError, StopReason: llm.StopError,
				Err: pkg.New(3106, llm.FriendlyUpstreamError(chunk.Error.Message), "")})
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
			feed.Send(llm.Event{Type: llm.EventThinking, Delta: d.Reasoning})
		}
		if d.Content != "" {
			// 部分模型没有独立的 reasoning 字段，直接把 <think>…</think> 写进
			// content。不拆开的话用户会看到裸标签，思考过程也永远折叠不了。
			t, body := think.Split(d.Content)
			if t != "" {
				feed.Send(llm.Event{Type: llm.EventThinking, Delta: t})
			}
			if body != "" {
				feed.Send(llm.Event{Type: llm.EventDelta, Delta: body})
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
				feed.Send(llm.Event{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{
					ID:   p.id,
					Name: p.name,
					Args: decodeArgs(p.args.String()),
				}})
			}
			// 不能在这里收尾：usage 是独立一帧，排在 finish_reason 之后、[DONE] 之前。
			// 清空 pending 是为了上游补发 delta 时不重复下发同一个调用。
			pendingCalls = map[int]*pendingCall{}
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
	// 有些兼容网关不发 finish_reason 直接 [DONE]：循环结束时把累积的工具调用
	// 补发出去，否则模型声明的调用全部静默丢失。
	for _, idx := range sortedIndexes(pendingCalls) {
		p := pendingCalls[idx]
		feed.Send(llm.Event{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{
			ID:   p.id,
			Name: p.name,
			Args: decodeArgs(p.args.String()),
		}})
		pendingCalls[idx] = nil
	}
	if stop == "" {
		stop = llm.StopStop
	}
	usage.LatencyMs = time.Since(started).Milliseconds()
	feed.Send(llm.Event{Type: llm.EventDone, StopReason: stop, Usage: usage})
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

func (c *Client) encode(req llm.Request) (io.Reader, error) {
	msgs := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		cm := chatMessage{Role: m.Role, Content: m.Content}
		// 识图输入：user 消息带图片时 content 切换成多模态数组形态
		if m.Role == llm.RoleUser && len(m.Images) > 0 {
			parts := []contentPart{{Type: "text", Text: m.Content}}
			for _, im := range m.Images {
				parts = append(parts, contentPart{
					Type:     "image_url",
					ImageURL: &imageURL{URL: "data:" + im.MIME + ";base64," + im.Base64},
				})
			}
			cm.Content = parts
		}
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
	// 与 anthropic 侧同一条纪律：0 会被 omitempty 整个吃掉，等于把输出上限
	// 交给上游默认值。正常路径由 service 层按模型能力填好，0 是上层 bug。
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = llm.DefaultMaxTokens
	}
	payload := chatRequest{
		Model: req.Model, Messages: msgs, Tools: tools, Stream: true,
		StreamOpts: &streamOpts{IncludeUsage: true},
	}
	if reasoningOnly(req.Model) {
		// 推理家族：只认 max_completion_tokens，且拒绝非默认 temperature / top_p。
		payload.MaxCompletionTokens = maxTokens
	} else {
		payload.MaxTokens = maxTokens
		payload.Temperature = req.Temperature
		payload.TopP = req.TopP
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
	return pkg.Wrap(llm.MapStatus(resp.StatusCode), "模型服务返回错误",
		&llm.StatusError{
			Status:     resp.StatusCode,
			RetryAfter: llm.ParseRetryAfter(resp.Header),
			Body:       msg,
		})
}
