// Package memorywrite 提供 memory_write 工具：模型主动把需要长期保留的信息写入记忆文件。
//
// 记忆只有一份（MEMORY.md），因此工具只需一个动作：追加一条到某个分节。
// 旧的 long_term / fact 双形态已收敛——「记在哪一类」曾是模型最容易出错的决策点。
package memorywrite

import (
	"context"
	"encoding/json"
	"strings"

	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Tool 记忆写入工具。
type Tool struct{ mem *memory.Service }

// New 构造。
func New(mem *memory.Service) *Tool { return &Tool{mem: mem} }

func (t *Tool) Name() string              { return "memory_write" }
func (t *Tool) RiskLevel() tool.RiskLevel { return tool.RiskWriteLocal }

func (t *Tool) Meta() tool.ToolMeta {
	return tool.ToolMeta{MaxResultChars: 500, UIHint: "memory"}
}

func (t *Tool) Description() string {
	return "把需要长期保留的信息写入记忆文件。用户明确要求「记住」，或你推断出稳定偏好、项目约定时使用。"
}

func (t *Tool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["content"],
			"properties": {
				"section": {"type": "string", "enum": ["用户偏好", "事实", "流程", "项目约定"], "description": "记忆分节；不确定时用「事实」"},
				"content": {"type": "string", "description": "要记住的内容，一句话说清，不要长篇大论"}
			}
		}`),
	}
}

type writeReq struct {
	Section string `json:"section"`
	Content string `json:"content"`
}

func (t *Tool) Execute(_ context.Context, args json.RawMessage) tool.ToolResult {
	var req writeReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4010, "memory_write 参数解析失败", err)}
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		return tool.ToolResult{Err: pkg.New(4011, "memory_write 内容不能为空", "")}
	}
	if t.mem == nil {
		return tool.ToolResult{Err: pkg.New(4012, "记忆服务未就绪", "")}
	}
	if err := t.mem.Append(req.Section, req.Content); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4012, "写入记忆失败", err)}
	}
	return tool.ToolResult{
		Content: "已记入长期记忆（" + memory.NormalizeSection(req.Section) + "）。",
		Data:    map[string]any{"file": t.mem.File()},
	}
}
