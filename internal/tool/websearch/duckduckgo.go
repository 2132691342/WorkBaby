package websearch

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
)

// DuckDuckGo 免配置搜索：抓 html.duckduckgo.com/html/ 结果页解析。
type DuckDuckGo struct {
	client *http.Client
}

// NewDuckDuckGo 构造 DuckDuckGo 引擎（10s 超时）。
func NewDuckDuckGo() *DuckDuckGo {
	return &DuckDuckGo{client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *DuckDuckGo) Name() string { return "duckduckgo" }

// resultLinkRe 匹配 result__a（标题链接）与 result__snippet（摘要）。
var (
	resultLinkRe = regexp.MustCompile(`(?s)<a[^>]+class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetRe    = regexp.MustCompile(`(?s)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	tagRe        = regexp.MustCompile(`(?s)<[^>]+>`)
	wsRe         = regexp.MustCompile(`\s+`)
	ddgHrefRe    = regexp.MustCompile(`uddg=([^&]+)`)
)

// Search 抓取 HTML 结果页并解析前 topK 条。
func (d *DuckDuckGo) Search(ctx context.Context, query string, topK int) ([]SearchHit, error) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, pkg.Wrap(4006, "build ddg request failed", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) WorkBaby/0.1")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, pkg.Wrap(4006, "ddg request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, pkg.Wrap(4006, "ddg http error", nil)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, pkg.Wrap(4006, "ddg read body failed", err)
	}
	return parseDuckDuckGo(body, topK), nil
}

// parseDuckDuckGo 解析结果页（标题链接与摘要两段分别匹配后按位置合并）。
func parseDuckDuckGo(html []byte, topK int) []SearchHit {
	s := string(html)
	links := resultLinkRe.FindAllStringSubmatch(s, -1)
	snips := snippetRe.FindAllStringSubmatch(s, -1)

	out := make([]SearchHit, 0, min(len(links), topK))
	for i, m := range links {
		if len(out) >= topK {
			break
		}
		href := m[1]
		if mm := ddgHrefRe.FindStringSubmatch(href); len(mm) == 2 {
			if dec, err := url.QueryUnescape(mm[1]); err == nil {
				href = dec
			}
		}
		title := cleanHTML(m[2])
		snippet := ""
		if i < len(snips) {
			snippet = cleanHTML(snips[i][1])
		}
		if title == "" && href == "" {
			continue
		}
		out = append(out, SearchHit{Title: title, URL: href, Snippet: snippet})
	}
	return out
}

func cleanHTML(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
