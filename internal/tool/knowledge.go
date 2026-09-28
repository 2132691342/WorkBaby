package tool

import (
	"context"
	"fmt"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// KnowledgeSearchFunc 是知识库检索的函数签名，避免 tool 包依赖 knowledge 包。
type KnowledgeSearchFunc func(ctx context.Context, query string, limit int) ([]domain.SearchHitVO, error)

// KnowledgeTool 把知识库检索暴露给内核：模型自己决定什么时候查资料。
type KnowledgeTool struct {
	Search KnowledgeSearchFunc
}

func (t *KnowledgeTool) Name() string               { return "knowledge_search" }
func (t *KnowledgeTool) Label() string              { return "搜索知识库" }
func (t *KnowledgeTool) Description() string        { return "在用户提供的私有资料里检索相关内容" }
func (t *KnowledgeTool) PromptSnippet() string      { return "knowledge_search(query, limit?): 检索用户的私有文档，回答资料类问题前先调用" }
func (t *KnowledgeTool) PromptGuidelines() []string { return nil }
func (t *KnowledgeTool) RequiresApproval() bool     { return false }
func (t *KnowledgeTool) ExecutionMode() ExecutionMode {
	return ExecutionParallel
}

func (t *KnowledgeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "检索关键词或短句"},
			"limit": map[string]any{"type": "integer", "description": "返回条数，默认 5"},
		},
		"required": []string{"query"},
	}
}

func (t *KnowledgeTool) Execute(ctx context.Context, in Input) (*Result, error) {
	query, _ := in.Args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return nil, pkg.New(6001, "请告诉我要检索什么", "")
	}
	limit := 5
	if n, ok := in.Args["limit"].(float64); ok && n > 0 {
		limit = int(n)
	}
	hits, err := t.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return &Result{Title: "没有找到相关内容", Content: "知识库里没有与「" + query + "」相关的内容。"}, nil
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] %s（%s）\n%s\n\n", i+1, h.Title, h.Path, h.Content)
	}
	return &Result{
		Title:   fmt.Sprintf("找到 %d 条相关内容", len(hits)),
		Content: strings.TrimRight(b.String(), "\n"),
	}, nil
}
