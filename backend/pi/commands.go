// PI 的工作流命令：把协议上的原始命令收成有语义的方法，上层不自己拼 map。
package pi

import (
	"context"
	"encoding/json"
)

// State 是 get_state 的响应。
type State struct {
	Model struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		API           string   `json:"api"`
		Provider      string   `json:"provider"`
		BaseURL       string   `json:"baseUrl"`
		Reasoning     bool     `json:"reasoning"`
		Input         []string `json:"input"`
		ContextWindow int      `json:"contextWindow"`
		MaxTokens     int      `json:"maxTokens"`
	} `json:"model"`
	ThinkingLevel         string `json:"thinkingLevel"`
	IsStreaming           bool   `json:"isStreaming"`
	IsCompacting          bool   `json:"isCompacting"`
	SteeringMode          string `json:"steeringMode"`
	FollowUpMode          string `json:"followUpMode"`
	SessionFile           string `json:"sessionFile"`
	SessionID             string `json:"sessionId"`
	SessionName           string `json:"sessionName"`
	AutoCompactionEnabled bool   `json:"autoCompactionEnabled"`
	MessageCount          int    `json:"messageCount"`
	PendingMessageCount   int    `json:"pendingMessageCount"`
}

// SessionStats 是 get_session_stats 的响应。
type SessionStats struct {
	TotalTokens int     `json:"totalTokens"`
	Input       int     `json:"input"`
	Output      int     `json:"output"`
	CacheRead   int     `json:"cacheRead"`
	CacheWrite  int     `json:"cacheWrite"`
	Cost        float64 `json:"cost"`
	Context     int     `json:"context"`
	Messages    int     `json:"messages"`
}

// PromptResult 是 prompt 的响应数据。
type PromptResult struct {
	// Disposition 取 started（开启了新 run）/ queued（运行中排队）/ handled（被扩展消费）。
	Disposition string `json:"disposition"`
}

// QueueResult 是 clear_queue 的响应数据。
type QueueResult struct {
	Steering []string `json:"steering"`
	FollowUp []string `json:"followUp"`
}

// ForkResult 是 fork / clone 的响应数据。
type ForkResult struct {
	Cancelled   bool   `json:"cancelled"`
	SessionID   string `json:"sessionId"`
	SessionFile string `json:"sessionFile"`
}

// GetState 读当前会话状态。
func (c *Client) GetState(ctx context.Context) (*State, error) {
	raw, err := c.Call(ctx, "get_state", nil)
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, ErrCommandFailed
	}
	return &st, nil
}

// GetSessionStats 读会话累计用量。
func (c *Client) GetSessionStats(ctx context.Context) (*SessionStats, error) {
	raw, err := c.Call(ctx, "get_session_stats", nil)
	if err != nil {
		return nil, err
	}
	var st SessionStats
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, ErrCommandFailed
	}
	return &st, nil
}

// Prompt 发一条用户消息。运行中时必须带 streamingBehavior（steer / followUp）。
func (c *Client) Prompt(ctx context.Context, message, streamingBehavior string, images []Image) (*PromptResult, error) {
	body := map[string]any{"message": message}
	if streamingBehavior != "" {
		body["streamingBehavior"] = streamingBehavior
	}
	if len(images) > 0 {
		body["images"] = images
	}
	raw, err := c.Call(ctx, "prompt", body)
	if err != nil {
		return nil, err
	}
	var res PromptResult
	_ = json.Unmarshal(raw, &res)
	return &res, nil
}

// Steer 插话：当前轮工具跑完后生效。
func (c *Client) Steer(ctx context.Context, message string, images []Image) (*QueueResult, error) {
	body := map[string]any{"message": message}
	if len(images) > 0 {
		body["images"] = images
	}
	raw, err := c.Call(ctx, "steer", body)
	if err != nil {
		return nil, err
	}
	var res QueueResult
	_ = json.Unmarshal(raw, &res)
	return &res, nil
}

// FollowUp 排队：等助手彻底停下才投递。
func (c *Client) FollowUp(ctx context.Context, message string, images []Image) (*QueueResult, error) {
	body := map[string]any{"message": message}
	if len(images) > 0 {
		body["images"] = images
	}
	raw, err := c.Call(ctx, "follow_up", body)
	if err != nil {
		return nil, err
	}
	var res QueueResult
	_ = json.Unmarshal(raw, &res)
	return &res, nil
}

// Abort 中断当前 run；它会等会话真正空闲才响应，所以用长超时。
func (c *Client) Abort(ctx context.Context) error {
	_, err := c.CallLong(ctx, "abort", nil)
	return err
}

// ClearQueue 取回尚未消费的插话与排队文本。停止前必须先调它：
// 不取回就 abort，用户排队的那句话会凭空消失。
func (c *Client) ClearQueue(ctx context.Context) (*QueueResult, error) {
	raw, err := c.Call(ctx, "clear_queue", nil)
	if err != nil {
		return nil, err
	}
	var res QueueResult
	_ = json.Unmarshal(raw, &res)
	return &res, nil
}

// NewSession 开一条新的 PI 会话。
func (c *Client) NewSession(ctx context.Context, parentSession string) error {
	body := map[string]any{}
	if parentSession != "" {
		body["parentSession"] = parentSession
	}
	_, err := c.Call(ctx, "new_session", body)
	return err
}

// SwitchSession 切到指定会话文件。
func (c *Client) SwitchSession(ctx context.Context, sessionFile string) error {
	_, err := c.Call(ctx, "switch_session", map[string]any{"sessionPath": sessionFile})
	return err
}

// SetModel 切换到指定服务商与模型。
func (c *Client) SetModel(ctx context.Context, provider, modelID string) error {
	_, err := c.Call(ctx, "set_model", map[string]any{"provider": provider, "modelId": modelID})
	return err
}

// SetThinkingLevel 设置思考档位。
func (c *Client) SetThinkingLevel(ctx context.Context, level string) error {
	_, err := c.Call(ctx, "set_thinking_level", map[string]any{"level": level})
	return err
}

// SetSessionName 同步会话显示名到 PI（PI 的 /resume 也用它）。
func (c *Client) SetSessionName(ctx context.Context, name string) error {
	_, err := c.Call(ctx, "set_session_name", map[string]any{"name": name})
	return err
}

// Fork 从指定条目分叉出一条新会话。
func (c *Client) Fork(ctx context.Context, entryID string) (*ForkResult, error) {
	raw, err := c.Call(ctx, "fork", map[string]any{"entryId": entryID})
	if err != nil {
		return nil, err
	}
	var res ForkResult
	_ = json.Unmarshal(raw, &res)
	return &res, nil
}

// Compact 手动触发一次上下文压缩。
func (c *Client) Compact(ctx context.Context) error {
	_, err := c.CallLong(ctx, "compact", nil)
	return err
}

// GetMessages 取当前上下文的全部消息（不用于渲染，渲染走 JSONL）。
func (c *Client) GetMessages(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "get_messages", nil)
}

// Image 是随消息发送的图片（base64，不含 data: 前缀）。
type Image struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// NewImage 构造一条图片载荷。
func NewImage(mime, base64Data string) Image {
	return Image{Type: "image", Data: base64Data, MimeType: mime}
}
