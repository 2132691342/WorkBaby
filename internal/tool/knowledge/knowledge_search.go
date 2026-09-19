// Package knowledge 提供 knowledge_search 工具：
// Agent 检索本地知识库的唯一入口，结果格式化为 [n] 文档名 · 标题 + 内容，便于模型引用出处。
package knowledge

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/rag"
	"WorkBaby/internal/tool"
)

// SearchTool 知识库检索工具。
type SearchTool struct {
	rt rag.Retriever
}

// New 构造检索工具。
func New(rt rag.Retriever) *SearchTool { return &SearchTool{rt: rt} }

func (t *SearchTool) Name() string              { return "knowledge_search" }
func (t *SearchTool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }

func (t *SearchTool) Description() string {
	return "在本地知识库中检索与问题相关的文档片段，返回带出处的正文。回答涉及用户上传资料时优先使用。"
}

func (t *SearchTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["query"],
			"properties": {
				"query": {"type": "string", "description": "检索关键词或短语（中文按连续子串匹配）"},
				"topK": {"type": "integer", "description": "返回条数，缺省 5，上限 20"}
			}
		}`),
	}
}

type searchReq struct {
	Query string `json:"query"`
	TopK  int    `json:"topK"`
}

// Execute 检索并格式化结果。
func (t *SearchTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req searchReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "knowledge_search args parse failed", err)}
	}
	if req.Query == "" {
		return tool.ToolResult{Err: pkg.New(4004, "knowledge_search query is required", "")}
	}
	hits, err := t.rt.Search(ctx, req.Query, req.TopK)
	if err != nil {
		// rag 层已返回 7004 AppError，保留原码（前端按段位分流）
		return tool.ToolResult{Err: err}
	}
	return tool.ToolResult{
		Content: rag.FormatHits(hits),
		Data:    map[string]any{"hits": hits, "count": len(hits)},
	}
}
