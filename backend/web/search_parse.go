package web

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// parseDuckDuckGo 从 DuckDuckGo 的 HTML 结果页里抽出结果。
// 免密钥搜索走的是 HTML 端点（JSON 端点要授权），
// 所以这里自己从 DOM 里取结果链接与摘要。
func parseDuckDuckGo(rawHTML string, limit int) []SearchResult {
	document, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	links := collectByClass(document, "result__a")
	snippets := collectByClass(document, "result__snippet")
	out := make([]SearchResult, 0, limit)
	for index, link := range links {
		if len(out) >= limit {
			break
		}
		target := NormalizeDuckURL(link.href)
		if target == "" {
			continue
		}
		snippet := ""
		if index < len(snippets) {
			snippet = snippets[index].text
		}
		out = append(out, SearchResult{
			ResultTitle:   strings.TrimSpace(link.text),
			ResultURL:     target,
			ResultSnippet: snippet,
			ResultSource:  hostOf(target),
		})
	}
	return out
}

// linkInfo 是一个链接的文本与地址。
type linkInfo struct {
	// text 链接文字
	text string
	// href 链接地址
	href string
}

// collectByClass 收集所有带指定 class 的元素，按文档顺序。
// DuckDuckGo 的结果标题与摘要是分开的两组元素，靠顺序配对。
func collectByClass(root *html.Node, className string) []linkInfo {
	out := make([]linkInfo, 0, 16)
	for node := root; node != nil; node = node.NextSibling {
		collectInNode(node, className, &out)
	}
	return out
}

// collectInNode 递归收集一个节点子树里的匹配元素。
func collectInNode(node *html.Node, className string, out *[]linkInfo) {
	if node == nil {
		return
	}
	if node.Type == html.ElementNode && hasClass(node, className) {
		text := strings.TrimSpace(textContent(node))
		*out = append(*out, linkInfo{text: text, href: attrValue(node, "href")})
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectInNode(child, className, out)
	}
}

// hasClass 判断元素是否带某个 class。
func hasClass(node *html.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key != "class" {
			continue
		}
		for _, item := range strings.Fields(attr.Val) {
			if item == className {
				return true
			}
		}
	}
	return false
}

// attrValue 取元素的某个属性值。
func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// textContent 取一个节点的纯文本。
func textContent(node *html.Node) string {
	var out strings.Builder
	for current := node; current != nil; current = current.NextSibling {
		collectText(current, &out)
	}
	return out.String()
}

// collectText 递归收集文本。
func collectText(node *html.Node, out *strings.Builder) {
	if node.Type == html.TextNode {
		out.WriteString(node.Data)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectText(child, out)
	}
}

// NormalizeDuckURL 把 DuckDuckGo 的跳转链接还原成真实地址。
// 它给的是 //duckduckgo.com/l/?uddg=<编码后的真实地址>，
// 直接用会拿到一个追踪链接。
func NormalizeDuckURL(href string) string {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "//") {
		trimmed = "https:" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if target := parsed.Query().Get("uddg"); target != "" {
		return target
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}

// hostOf 取地址的主机名；取不到给空串。
func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
