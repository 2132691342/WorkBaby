package agent

import (
	"context"
	"strings"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/web"
)

// WebKit 是联网能力的出口。
// 引擎只认这个接口，不认具体的搜索服务：换服务商不该改引擎代码。
type WebKit interface {
	// Search 联网搜索，返回若干条结果
	Search(ctx context.Context, query string, limit int) ([]web.SearchResult, error)
	// Fetch 抓取一个地址并抽出可读正文
	Fetch(ctx context.Context, url string) (string, error)
}

// WebKitFunc 是 WebKit 的函数形态。
type WebKitFunc struct {
	// SearchFunc 搜索实现
	SearchFunc func(ctx context.Context, query string, limit int) ([]web.SearchResult, error)
	// FetchFunc 抓取实现
	FetchFunc func(ctx context.Context, url string) (string, error)
}

// Search 执行搜索。
func (k WebKitFunc) Search(ctx context.Context, query string, limit int) ([]web.SearchResult, error) {
	if k.SearchFunc == nil {
		return nil, pkg.Err(pkg.CodeNotFound, "没有配置联网搜索")
	}
	return k.SearchFunc(ctx, query, limit)
}

// Fetch 执行抓取。
func (k WebKitFunc) Fetch(ctx context.Context, url string) (string, error) {
	if k.FetchFunc == nil {
		return "", pkg.Err(pkg.CodeNotFound, "没有配置网页抓取")
	}
	return k.FetchFunc(ctx, url)
}

// webSearchTool 让模型联网搜索。
type webSearchTool struct {
	// kit 联网出口
	kit WebKit
}

// Spec 工具声明。
func (t *webSearchTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "web_search",
		Description: "联网搜索。返回若干条结果，每条含标题、地址与摘要。" +
			"需要网上的事实、文档、报错信息或最新资料时用它。" +
			"搜到结果后再用 web_fetch 读原文——摘要通常不够回答具体问题。",
		Schema: schemaOf(map[string]any{
			"query": "搜索词。带上具体的关键词与限定条件效果更好",
			"limit": "可选，最多返回几条，默认 8，上限 20",
		}, "query"),
	}
}

// Run 执行搜索。
func (t *webSearchTool) Run(ctx context.Context, _ *Registry, args map[string]any) Result {
	query := strings.TrimSpace(stringArg(args, "query"))
	if query == "" {
		return errorf("query 不能为空")
	}
	limit := intArg(args, "limit", 8)
	if limit <= 0 {
		limit = 8
	}
	results, err := t.kit.Search(ctx, query, limit)
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	if len(results) == 0 {
		return Result{Text: "没有搜到结果，换个说法再试。"}
	}
	var out strings.Builder
	out.WriteString("搜索「")
	out.WriteString(query)
	out.WriteString("」的结果：\n\n")
	for index, item := range results {
		out.WriteString(itoa(index + 1))
		out.WriteString(". ")
		out.WriteString(item.ResultTitle)
		out.WriteString("\n   ")
		out.WriteString(item.ResultURL)
		if snippet := strings.TrimSpace(item.ResultSnippet); snippet != "" {
			out.WriteString("\n   ")
			out.WriteString(pkg.Truncate(snippet, 400))
		}
		out.WriteString("\n")
	}
	return Result{
		Text: out.String(),
		Details: map[string]any{
			"query":        query,
			"result_count": len(results),
			"urls":         resultURLs(results),
		},
	}
}

// resultURLs 取结果里的地址列表。
func resultURLs(results []web.SearchResult) []any {
	out := make([]any, 0, len(results))
	for _, item := range results {
		out = append(out, item.ResultURL)
	}
	return out
}

// webFetchTool 让模型读一个网页。
type webFetchTool struct {
	// kit 联网出口
	kit WebKit
}

// Spec 工具声明。
func (t *webFetchTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "web_fetch",
		Description: "抓取一个网址并读它的正文（HTML 会转成可读文本，PDF 会抽出文字）。" +
			"先 web_search 找到地址，再用这个读原文。" +
			"网址必须是 http 或 https，且不能指向内网地址。",
		Schema: schemaOf(map[string]any{
			"url": "要读的网址",
		}, "url"),
	}
}

// Run 执行抓取。
func (t *webFetchTool) Run(ctx context.Context, _ *Registry, args map[string]any) Result {
	url := strings.TrimSpace(stringArg(args, "url"))
	if url == "" {
		return errorf("url 不能为空")
	}
	text, err := t.kit.Fetch(ctx, url)
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	if strings.TrimSpace(text) == "" {
		return Result{Text: "这个地址没有读到可读内容（可能是图片、扫描件或需要登录）。"}
	}
	return Result{
		Text: text,
		Details: map[string]any{
			"url":        url,
			"char_count": len([]rune(text)),
		},
	}
}
