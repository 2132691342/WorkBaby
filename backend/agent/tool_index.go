package agent

import (
	"context"
	"strings"

	"WorkBaby/backend/pkg"
)

// indexSearchTool 让模型用预索引检索工作区。
// 它比 grep 快得多：大仓库里一次全盘 grep 要几秒，
// 而模型每轮要搜好几次——索引把这件事提前算好了。
type indexSearchTool struct {
	// indexer 检索出口
	indexer Indexer
}

// Spec 工具声明。
func (t *indexSearchTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "index_search",
		Description: "在当前工作区的预索引里检索文件与符号。快得多，" +
			"大仓库里优先用它定位文件，再用 read 读内容、用 grep 找具体行。" +
			"只搜得到文件名与符号名；要看文件里的某一行请用 grep。",
		Schema: schemaOf(map[string]any{
			"query": "检索词。文件名、符号名或路径片段",
			"limit": "可选，最多返回几条，默认 15，上限 50",
		}, "query"),
	}
}

// Run 执行检索。
func (t *indexSearchTool) Run(ctx context.Context, _ *Registry, args map[string]any) Result {
	query := strings.TrimSpace(stringArg(args, "query"))
	if query == "" {
		return errorf("query 不能为空")
	}
	limit := intArg(args, "limit", 15)
	if limit <= 0 {
		limit = 15
	}
	hits, err := t.indexer.Search(ctx, query, limit)
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	if len(hits) == 0 {
		return Result{Text: "索引里没找到「" + query + "」。换个关键词，或用 find 按文件名找。"}
	}
	var out strings.Builder
	out.WriteString("检索「")
	out.WriteString(query)
	out.WriteString("」命中 ")
	out.WriteString(itoa(len(hits)))
	out.WriteString(" 条：\n\n")
	for _, hit := range hits {
		out.WriteString("- ")
		out.WriteString(hit.HitPath)
		if hit.HitKind == "symbol" && hit.HitSymbol != "" {
			out.WriteString(":")
			out.WriteString(itoa(hit.HitLine))
			out.WriteString("  ")
			out.WriteString(hit.HitSymbol)
		}
		if hit.HitReason != "" {
			out.WriteString("（")
			out.WriteString(hit.HitReason)
			out.WriteString("）")
		}
		out.WriteString("\n")
	}
	return Result{
		Text: out.String(),
		Details: map[string]any{
			"query":     query,
			"hit_count": len(hits),
		},
	}
}
