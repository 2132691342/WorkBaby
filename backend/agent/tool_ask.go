package agent

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/backend/pkg"
)

// AskMethod 是提问的形态。
type AskMethod string

const (
	// AskConfirm 确认：答「是」或「否」
	AskConfirm AskMethod = "confirm"
	// AskSelect 单选：从若干选项里挑一个
	AskSelect AskMethod = "select"
	// AskInput 单行输入
	AskInput AskMethod = "input"
	// AskEditor 多行输入
	AskEditor AskMethod = "editor"
)

// AskRequest 是一次提问。
type AskRequest struct {
	// AskId 提问 ID；回答时原样带回
	AskId string
	// AskMethod 提问形态
	AskMethod AskMethod
	// AskTitle 标题
	AskTitle string
	// AskMessage 正文
	AskMessage string
	// AskPlaceholder 输入占位
	AskPlaceholder string
	// AskPrefill 预填内容
	AskPrefill string
	// AskOptions 可选项
	AskOptions []string
	// AskTimeoutMs 超时毫秒；0 表示不超时
	AskTimeoutMs int
}

// AskResponse 是用户的回答。
type AskResponse struct {
	// Value 文本类回答
	Value string
	// Confirmed 确认类回答
	Confirmed bool
	// Cancelled 用户取消了提问
	Cancelled bool
}

// Asker 是提问的出口：实现方把问题送到界面并把回答收回来。
// 引擎只认这个接口——它不关心界面长什么样、答案从哪条路回来。
type Asker interface {
	// Ask 发起一次提问并等回答；用户取消时返回 Cancelled 为真的响应
	Ask(ctx context.Context, req AskRequest) AskResponse
}

// AskerFunc 是 Asker 的函数形态。
type AskerFunc func(ctx context.Context, req AskRequest) AskResponse

// Ask 发起一次提问并等回答。
func (f AskerFunc) Ask(ctx context.Context, req AskRequest) AskResponse {
	return f(ctx, req)
}

// PendingAsks 是在等的提问：界面刷新时靠它把还没回答的卡补回来。
// 提问跨了一次运行边界（用户可能切走再回来），所以要单独记着。
type PendingAsks struct {
	// mu 保护待答列表
	mu sync.Mutex
	// items 待答提问，按提问次序
	items []AskRequest
}

// NewPendingAsks 造一个待答列表。
func NewPendingAsks() *PendingAsks { return &PendingAsks{} }

// Add 记下一条待答提问。
func (p *PendingAsks) Add(req AskRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.items {
		if p.items[i].AskId == req.AskId {
			p.items[i] = req
			return
		}
	}
	p.items = append(p.items, req)
}

// Remove 摘掉一条已回答的提问。
func (p *PendingAsks) Remove(askId string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.items[:0]
	for _, req := range p.items {
		if req.AskId != askId {
			out = append(out, req)
		}
	}
	p.items = out
}

// Items 取待答提问副本。
func (p *PendingAsks) Items() []AskRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]AskRequest, len(p.items))
	copy(out, p.items)
	return out
}

// askTool 让模型向用户提问：走工具而不是直接回话，界面才能给卡片（选项 / 确认 / 多行编辑），
// 回话只是一句文字，用户得自己把选项敲回来。
type askTool struct {
	// reg 所属注册表；提问出口挂在它上面
	reg *Registry
}

// Spec 工具声明。
func (t *askTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "ask",
		Description: "向用户提问并等对方回答。只有当你确实需要对方做选择或提供信息、" +
			"且猜错的代价高时才用；能从文件或命令拿到答案就别问。" +
			"method 可选 confirm（是/否）、select（从 options 里选一个）、input（一行）、editor（多行）。" +
			"select 必须给 options。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method":      map[string]any{"type": "string", "description": "confirm / select / input / editor"},
				"title":       map[string]any{"type": "string", "description": "标题，一句话"},
				"message":     map[string]any{"type": "string", "description": "正文，说明为什么问、问什么"},
				"placeholder": map[string]any{"type": "string", "description": "输入框占位提示"},
				"prefill":     map[string]any{"type": "string", "description": "预填内容"},
				"options": map[string]any{
					"type":        "array",
					"description": "可选项；method 为 select 时必填",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"method"},
		},
	}
}

// Run 发起提问并等回答。
func (t *askTool) Run(ctx context.Context, _ *Registry, args map[string]any) Result {
	asker := t.reg.Asker
	if asker == nil {
		return errorf("这个会话没有提问通道")
	}
	method := AskMethod(strings.TrimSpace(stringArg(args, "method")))
	switch method {
	case AskConfirm, AskSelect, AskInput, AskEditor:
	case "":
		method = AskInput
	default:
		return errorf("method 只能是 confirm / select / input / editor")
	}
	options := make([]string, 0, 4)
	if raw, ok := args["options"].([]any); ok {
		for _, item := range raw {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				options = append(options, text)
			}
		}
	}
	if method == AskSelect && len(options) == 0 {
		return errorf("method 是 select 时必须给 options")
	}
	req := AskRequest{
		AskId:          pkg.NewID(pkg.PrefixAsk),
		AskMethod:      method,
		AskTitle:       strings.TrimSpace(stringArg(args, "title")),
		AskMessage:     strings.TrimSpace(stringArg(args, "message")),
		AskPlaceholder: strings.TrimSpace(stringArg(args, "placeholder")),
		AskPrefill:     stringArg(args, "prefill"),
		AskOptions:     options,
	}
	response := asker.Ask(ctx, req)
	if response.Cancelled {
		// 取消不是失败：用户有权不答。把它当一条「用户没答」告诉模型，
		// 让它自己决定是继续还是收手。
		return Result{Text: "用户取消了这次提问，没有给出答案。"}
	}
	switch method {
	case AskConfirm:
		if response.Confirmed {
			return Result{Text: "用户回答：是。"}
		}
		return Result{Text: "用户回答：否。"}
	case AskSelect:
		return Result{Text: "用户选择了：" + response.Value}
	default:
		if strings.TrimSpace(response.Value) == "" {
			return errorf("用户没有填内容")
		}
		return Result{Text: "用户的回答：" + response.Value}
	}
}
