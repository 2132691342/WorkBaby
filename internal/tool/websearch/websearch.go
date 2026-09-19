// Package websearch 提供多引擎网页搜索；引擎接口 + DuckDuckGo 免配置实现。
package websearch

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// SearchHit 统一搜索结果。
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Engine 搜索引擎适配接口（由装配方选择实现）。
type Engine interface {
	Name() string
	Search(ctx context.Context, query string, topK int) ([]SearchHit, error)
}

// WebSearchTool 搜索工具。
type WebSearchTool struct {
	engine Engine
}

// New 构造搜索工具（默认 DuckDuckGo 免配置）。
func New() *WebSearchTool { return &WebSearchTool{engine: NewDuckDuckGo()} }

// WithEngine 替换引擎（测试或配置使用）。
func (t *WebSearchTool) WithEngine(e Engine) *WebSearchTool { t.engine = e; return t }

func (t *WebSearchTool) Name() string              { return "websearch" }
func (t *WebSearchTool) RiskLevel() tool.RiskLevel { return tool.RiskNetwork }
func (t *WebSearchTool) Description() string {
	return "搜索网页（默认 DuckDuckGo，免配置）；返回标题、链接、摘要。"
}

func (t *WebSearchTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["query"],
			"properties": {
				"query": {"type": "string", "description": "搜索关键词"},
				"topK": {"type": "integer", "description": "返回条数，缺省 5，上限 10"}
			}
		}`),
	}
}

type searchReq struct {
	Query string `json:"query"`
	TopK  int    `json:"topK"`
}

// Execute 执行搜索并返回结果文本。
func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req searchReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "websearch args parse failed", err)}
	}
	if req.Query == "" {
		return tool.ToolResult{Err: pkg.New(4004, "websearch query is required", "")}
	}
	if req.TopK <= 0 || req.TopK > 10 {
		req.TopK = 5
	}
	hits, err := t.engine.Search(ctx, req.Query, req.TopK)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "websearch failed", err)}
	}
	bs, err := json.Marshal(hits)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "marshal search hits failed", err)}
	}
	return tool.ToolResult{Content: string(bs), Data: map[string]any{"hits": hits}}
}
