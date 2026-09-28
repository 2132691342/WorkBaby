package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
)

// webSearchTool 用 DuckDuckGo 的 HTML 端点搜索，免 Key。
type webSearchTool struct{}

func (webSearchTool) Name() string                { return "web_search" }
func (webSearchTool) Label() string               { return "搜网页" }
func (webSearchTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (webSearchTool) RequiresApproval() bool      { return false }
func (webSearchTool) Description() string         { return "联网搜索，返回标题、链接与摘要。" }
func (webSearchTool) PromptSnippet() string       { return "联网搜索" }
func (webSearchTool) PromptGuidelines() []string {
	return []string{"问到最新信息、外部资料时才搜索；本地文件里能找到的不要联网。"}
}

func (webSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索关键词"},
			"limit": map[string]any{"type": "integer", "description": "返回条数，默认 5"},
		},
		"required": []string{"query"},
	}
}

func (t webSearchTool) Execute(ctx context.Context, in Input) (*Result, error) {
	query := Str(in.Args, "query")
	if query == "" {
		return nil, ErrMissingArg
	}
	limit := Int(in.Args, "limit", 5)

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		"https://html.duckduckgo.com/html/",
		strings.NewReader(url.Values{"q": {query}}.Encode()))
	if err != nil {
		return nil, pkg.Wrap(4004, "构造搜索请求失败", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 WorkBaby")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, pkg.Wrap(4005, "搜索失败，检查网络", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, pkg.Wrap(4005, "读取搜索结果失败", err)
	}

	items := parseResults(string(body), limit)
	if len(items) == 0 {
		return &Result{Content: "（没搜到结果）", Title: "搜网页（无结果）", Detail: ""}, nil
	}
	out := strings.Join(items, "\n") + "\n"
	return &Result{
		Content: Cut(out, in.Deps.TmpDir, "search"),
		Title:   fmt.Sprintf("搜到 %d 条结果", len(items)),
		Detail:  out,
	}, nil
}

var (
	resultTitleRe = regexp.MustCompile(`class="result__a"[^>]*>(.*?)</a>`)
	resultSnipRe  = regexp.MustCompile(`class="result__snippet"[^>]*>(.*?)</a>`)
	resultLinkRe  = regexp.MustCompile(`class="result__a"\s+href="([^"]+)"`)
	tagRe         = regexp.MustCompile(`<[^>]+>`)
	blockRe       = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	spaceRunRe    = regexp.MustCompile(` {2,}`)
	blankRunRe    = regexp.MustCompile(`\n{3,}`)
)

// parseResults 从 DuckDuckGo 的 HTML 里抽标题与摘要；解析失败不报错，返回空结果即可。
func parseResults(html string, limit int) []string {
	titles := resultTitleRe.FindAllStringSubmatch(html, -1)
	snips := resultSnipRe.FindAllStringSubmatch(html, -1)
	links := resultLinkRe.FindAllStringSubmatch(html, -1)
	out := []string{}
	for i := 0; i < len(titles) && len(out) < limit; i++ {
		title := cleanText(titles[i][1])
		if title == "" {
			continue
		}
		snippet := ""
		if i < len(snips) {
			snippet = cleanText(snips[i][1])
		}
		link := ""
		if i < len(links) {
			link = unwrapDuckURL(links[i][1])
		}
		line := fmt.Sprintf("%d. %s\n   %s\n   %s", len(out)+1, title, link, snippet)
		out = append(out, strings.TrimRight(line, "\n"))
	}
	return out
}

func cleanText(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return squeezeSpace(s)
}

// squeezeSpace 压掉网页排版留下的连续空格。
func squeezeSpace(s string) string {
	return strings.TrimSpace(spaceRunRe.ReplaceAllString(s, " "))
}

// unwrapDuckURL 还原 DuckDuckGo 的跳转链接。
func unwrapDuckURL(raw string) string {
	if i := strings.Index(raw, "uddg="); i >= 0 {
		if u, err := url.QueryUnescape(raw[i+5:]); err == nil {
			if j := strings.Index(u, "&"); j >= 0 {
				u = u[:j]
			}
			return u
		}
	}
	return raw
}

// webFetchTool 抓网页正文。
type webFetchTool struct{}

func (webFetchTool) Name() string                { return "web_fetch" }
func (webFetchTool) Label() string               { return "打开网页" }
func (webFetchTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (webFetchTool) RequiresApproval() bool      { return false }
func (webFetchTool) Description() string         { return "抓取一个网页并转成纯文本，便于阅读与总结。" }
func (webFetchTool) PromptSnippet() string       { return "打开网页看内容" }
func (webFetchTool) PromptGuidelines() []string {
	return []string{"只抓用户给出的具体网址，不要自己猜域名。"}
}

func (webFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":        map[string]any{"type": "string", "description": "网页地址"},
			"max_chars":  map[string]any{"type": "integer", "description": "最多保留多少字符，默认 20000"},
		},
		"required": []string{"url"},
	}
}

func (t webFetchTool) Execute(ctx context.Context, in Input) (*Result, error) {
	rawURL := Str(in.Args, "url")
	if rawURL == "" {
		return nil, ErrMissingArg
	}
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, pkg.New(4006, "网址不正确", rawURL)
	}
	maxChars := Int(in.Args, "max_chars", 20000)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, pkg.Wrap(4007, "构造抓取请求失败", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 WorkBaby")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, pkg.Wrap(4007, "打开网页失败", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, pkg.Wrap(4007, "读取网页失败", err)
	}

	text := cleanHTML(string(body))
	if len(text) > maxChars {
		text = CutBytes(text, maxChars) + "\n……（已截断）"
	}
	if strings.TrimSpace(text) == "" {
		text = "（这个页面没有可提取的文本）"
	}
	return &Result{
		Content: text,
		Title:   "打开了一个网页",
		Detail:  text,
	}, nil
}

// cleanHTML 去脚本样式与标签，再压掉多余空白。标签换成空格而不是直接删，
// 否则相邻两段正文会粘成一个词。
func cleanHTML(h string) string {
	h = blockRe.ReplaceAllString(h, " ")
	h = tagRe.ReplaceAllString(h, " ")
	h = strings.ReplaceAll(h, "&nbsp;", " ")
	h = strings.ReplaceAll(h, "&amp;", "&")
	h = blankRunRe.ReplaceAllString(h, "\n\n")
	return squeezeSpace(h)
}
