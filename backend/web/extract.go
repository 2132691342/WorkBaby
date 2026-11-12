package web

import (
	"strings"

	"golang.org/x/net/html"
)

// ExtractReadable 把一段 HTML 抽成可读文本。
// 目标是「模型能读」：去掉导航、广告、脚本这些噪声，留下正文与标题结构。
// 用 x/net/html 解析（它已在依赖树里，不新增模块），跳过规则自己定。
func ExtractReadable(rawHTML string) string {
	document, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// 解析不了就退回剥标签：半个正文也比什么都没有强。
		return stripTags(rawHTML)
	}
	root := pickContentRoot(document)
	var out strings.Builder
	walkNode(root, &out, &extractState{})
	return tidyBlankLines(out.String())
}

// pickContentRoot 选出正文所在的那块子树。
// 优先 article / main：页面外壳里的导航与侧栏就不进来了。
// 都没有就用 body——那时的噪声靠跳过规则处理。
func pickContentRoot(document *html.Node) *html.Node {
	best := findElement(document, "article")
	if best == nil {
		best = findElement(document, "main")
	}
	if best == nil {
		best = findElement(document, "body")
	}
	if best == nil {
		return document
	}
	return best
}

// findElement 找出第一个指定标签的元素。
func findElement(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

// extractState 是遍历过程中的可变状态。
type extractState struct {
	// skipDepth 正在跳过的深度
	skipDepth int
	// skipTag 正在跳过的标签名
	skipTag string
	// listDepth 列表嵌套层数
	listDepth int
	// preformatted 代码块深度
	preformatted int
	// pendingNewlines 待补的空行数
	pendingNewlines int
}

// walkNode 递归遍历单个节点及其兄弟。
func walkNode(node *html.Node, out *strings.Builder, state *extractState) {
	if node == nil {
		return
	}
	switch node.Type {
	case html.TextNode:
		if state.skipDepth > 0 {
			return
		}
		writeText(out, node.Data, state)
	case html.ElementNode:
		handleElement(node, out, state)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walkNode(child, out, state)
		}
		handleEndTag(node.Data, out, state)
	case html.CommentNode:
		// 注释对模型没有价值。
	}
}

// handleElement 处理开始标签。
func handleElement(node *html.Node, out *strings.Builder, state *extractState) {
	name := strings.ToLower(node.Data)
	if state.skipTag != "" {
		// 跳过期间只认同一标签的嵌套：同名标签要多退一层才能配平。
		if name == state.skipTag {
			state.skipDepth++
		}
		return
	}
	if isSkippedTag(name) {
		state.skipTag = name
		state.skipDepth = 1
		return
	}
	switch name {
	case "br":
		state.pendingNewlines = 2
	case "p", "div", "section", "article", "header", "footer", "aside",
		"ul", "ol", "table", "tr", "blockquote", "figure":
		state.pendingNewlines = maxInt(state.pendingNewlines, 2)
	case "li":
		state.pendingNewlines = maxInt(state.pendingNewlines, 1)
		writePending(out, state)
		out.WriteString(strings.Repeat("  ", state.listDepth))
		out.WriteString("- ")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		state.pendingNewlines = 2
		writePending(out, state)
		out.WriteString("\n\n")
		out.WriteString(strings.Repeat("#", headingLevel(name)))
		out.WriteString(" ")
	case "pre", "code":
		state.preformatted++
	}
}

// handleEndTag 处理结束标签。
func handleEndTag(name string, out *strings.Builder, state *extractState) {
	lowered := strings.ToLower(name)
	if state.skipTag != "" {
		if lowered == state.skipTag {
			state.skipDepth--
			if state.skipDepth <= 0 {
				state.skipTag = ""
				state.skipDepth = 0
			}
		}
		return
	}
	switch lowered {
	case "p", "div", "section", "article", "li", "tr", "blockquote":
		state.pendingNewlines = maxInt(state.pendingNewlines, 2)
	case "h1", "h2", "h3", "h4", "h5", "h6":
		state.pendingNewlines = maxInt(state.pendingNewlines, 2)
	case "ul", "ol":
		state.listDepth = maxInt(0, state.listDepth-1)
		state.pendingNewlines = maxInt(state.pendingNewlines, 2)
	case "pre", "code":
		if state.preformatted > 0 {
			state.preformatted--
		}
	}
}

// writeText 写一段文本：代码块内保留原样，其余压掉多余空白。
func writeText(out *strings.Builder, raw string, state *extractState) {
	if state.preformatted > 0 {
		out.WriteString(raw)
		return
	}
	collapsed := collapseSpace(raw)
	if collapsed == "" {
		return
	}
	writePending(out, state)
	out.WriteString(collapsed)
}

// writePending 补上待写的空行。
func writePending(out *strings.Builder, state *extractState) {
	if state.pendingNewlines <= 0 {
		return
	}
	out.WriteString(strings.Repeat("\n", state.pendingNewlines))
	state.pendingNewlines = 0
}

// headingLevel 取标题级别。
func headingLevel(name string) int {
	switch name {
	case "h1":
		return 1
	case "h2":
		return 2
	case "h3":
		return 3
	case "h4":
		return 4
	case "h5":
		return 5
	default:
		return 6
	}
}

// skippedTags 是要整段跳过的标签：脚本、样式与页面外壳。
// nav / aside / header / footer 是页面导航与页眉页脚——
// 它们在正文之外，对模型读内容没有帮助，却占着上下文。
var skippedTags = map[string]struct{}{
	"script": {}, "style": {}, "noscript": {}, "svg": {}, "canvas": {},
	"iframe": {}, "object": {}, "embed": {}, "form": {}, "button": {},
	"select": {}, "option": {}, "textarea": {}, "template": {},
	"head": {}, "meta": {}, "link": {}, "nav": {}, "aside": {},
	"header": {}, "footer": {},
}

// isSkippedTag 判断一个标签是否要整段跳过。
func isSkippedTag(name string) bool {
	_, ok := skippedTags[name]
	return ok
}

// stripTags 是解析失败时的兜底：只去标签，保留文字。
func stripTags(rawHTML string) string {
	var out strings.Builder
	depth := 0
	for _, ch := range rawHTML {
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(ch)
			}
		}
	}
	return tidyBlankLines(collapseSpace(out.String()))
}

// collapseSpace 把连续空白压成一个空格。
func collapseSpace(text string) string {
	var out strings.Builder
	space := false
	for _, ch := range text {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			space = true
			continue
		}
		if space && out.Len() > 0 {
			out.WriteByte(' ')
		}
		space = false
		out.WriteRune(ch)
	}
	return out.String()
}

// tidyBlankLines 把两个以上的连续空行压成一个。
func tidyBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blanks := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blanks++
			if blanks > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blanks = 0
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// maxInt 取两数较大者。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
