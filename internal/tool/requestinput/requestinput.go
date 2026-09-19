// Package requestinput 提供 request_input 工具：模型缺关键信息时暂停 run 向用户提问，
// 用户回复经同一暂停通道回填（approval_records 持久化，可跨重启排空）。
package requestinput

import (
	"context"
	"encoding/json"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Tool 补充输入工具。
type Tool struct{}

// New 构造。
func New() *Tool { return &Tool{} }

var paramsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["question"],
	"properties": {
		"question": {"type": "string", "description": "要问用户的问题，需具体、可回答（用户看不到对话之外的上下文）"}
	}
}`)

func (t *Tool) Name() string { return "request_input" }
func (t *Tool) Description() string {
	return "当你缺少继续工作所需的关键信息（偏好、凭据位置、歧义澄清）时，向用户提问并等待回复。优先于猜测。"
}
func (t *Tool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: paramsSchema}
}

// RiskLevel 纯交互，无副作用。
func (t *Tool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }

// Execute 阻塞等待用户回复；未注入能力时返回可见错误（模型据此改道自行假设）。
func (t *Tool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "request_input args parse failed", err)}
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		return tool.ToolResult{Err: pkg.New(4004, "request_input 需要非空 question", "")}
	}
	r := tool.InputRequesterFromCtx(ctx)
	if r == nil {
		return tool.ToolResult{
			Content: "request_input 不可用：当前环境未开启用户交互通道，请基于合理假设继续并说明假设。",
			Err:     pkg.New(5009, "补充输入能力未注入", ""),
		}
	}
	answer, ok := r.RequestInput(ctx, req.Question)
	if !ok {
		return tool.ToolResult{
			Content: "用户未在限时内回复。请基于合理假设继续并说明假设。",
			Err:     pkg.New(5009, "用户未回复（超时或取消）", ""),
		}
	}
	return tool.ToolResult{Content: "用户回复：" + answer}
}
