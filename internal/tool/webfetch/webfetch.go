// Package webfetch 提供网页抓取工具：标准库 http.Client，正文提取用正则剥离。
package webfetch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// maxBodyBytes 单次响应体上限（5MB）。
const maxBodyBytes = 5 << 20

// maxResultLen 返回给 LLM 的正文上限（防超长结果污染上下文）。
const maxResultLen = 50_000

// Go regexp 不支持反向引用，分别剥离各标签对。
var (
	scriptTagRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	hiddenTagRe   = regexp.MustCompile(`(?is)<(noscript|svg|iframe)[^>]*>.*?</(noscript|svg|iframe)>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]+>`)
	wsRe          = regexp.MustCompile(`\s+`)
	contentTypeRe = regexp.MustCompile(`(?i)^(text/html|text/plain|application/json|text/[\w.+-]+)`)
)

// WebFetchTool 抓取 URL 并提取可读正文。
type WebFetchTool struct {
	client *http.Client
}

// New 构造 WebFetchTool（30s 超时）。
func New() *WebFetchTool {
	return &WebFetchTool{client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *WebFetchTool) Name() string              { return "webfetch" }
func (t *WebFetchTool) RiskLevel() tool.RiskLevel { return tool.RiskNetwork }
func (t *WebFetchTool) Description() string {
	return "抓取网页内容并提取正文文本（支持 HTML/JSON/纯文本，上限 5MB）。"
}

func (t *WebFetchTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["url"],
			"properties": {
				"url": {"type": "string", "description": "要抓取的 URL"},
				"maxBytes": {"type": "integer", "description": "响应体上限字节，缺省 5MB"}
			}
		}`),
	}
}

type fetchReq struct {
	URL      string `json:"url"`
	MaxBytes int    `json:"maxBytes"`
}

// Execute 抓取并提取正文。
func (t *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req fetchReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "webfetch args parse failed", err)}
	}
	if req.URL == "" {
		return tool.ToolResult{Err: pkg.New(4004, "webfetch url is required", "")}
	}
	limit := maxBodyBytes
	if req.MaxBytes > 0 && req.MaxBytes < limit {
		limit = req.MaxBytes
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "build webfetch request failed", err)}
	}
	httpReq.Header.Set("User-Agent", "WorkBaby/0.1 (+webfetch)")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "webfetch failed", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return tool.ToolResult{Err: pkg.Wrap(4006, "webfetch http error", nil), Content: "HTTP " + resp.Status}
	}
	ct := resp.Header.Get("Content-Type")
	if !contentTypeRe.MatchString(ct) {
		return tool.ToolResult{Err: pkg.New(4006, "webfetch content-type not supported", ct)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "webfetch read body failed", err)}
	}
	if len(body) > limit {
		return tool.ToolResult{Err: pkg.New(4008, "webfetch body too large, truncated", "")}
	}

	text := extractText(body, ct)
	if len(text) > maxResultLen {
		text = text[:maxResultLen] + "\n... (truncated)"
	}
	return tool.ToolResult{Content: text}
}

// extractText 按 Content-Type 提取可读文本；HTML 剥离 script/style/tag 后压缩空白。
func extractText(body []byte, contentType string) string {
	s := string(body)
	if strings.Contains(contentType, "html") {
		s = scriptTagRe.ReplaceAllString(s, " ")
		s = styleTagRe.ReplaceAllString(s, " ")
		s = hiddenTagRe.ReplaceAllString(s, " ")
		s = tagRe.ReplaceAllString(s, " ")
		s = wsRe.ReplaceAllString(s, " ")
		return strings.TrimSpace(s)
	}
	// text/* 与 application/json 直接返回；JSON 压缩空白便于阅读
	if strings.Contains(contentType, "json") {
		var v any
		if json.Unmarshal(body, &v) == nil {
			if b, err := json.MarshalIndent(v, "", "  "); err == nil {
				return string(b)
			}
		}
	}
	return strings.TrimSpace(s)
}
