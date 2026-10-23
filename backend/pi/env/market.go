// 插件市场：PI 官方的包发现机制是 npm 关键字 pi-package（pi.dev 的包画廊同一来源）。
// 这里只做发现与解析；装 / 卸继续走 PI 自己的 CLI，不重写包规则。
package env

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// marketTTL 是市场清单的缓存时长：面板来回切不必每次都问注册表。
const marketTTL = 5 * time.Minute

// MarketEntry 是注册表里搜到的一个可装包。
type MarketEntry struct {
	Name        string
	Description string
	Version     string
	Publisher   string
	Repo        string
	// PublishedAt 是最近发布时间（毫秒；0 表示注册表没给）。
	PublishedAt int64
}

var (
	marketMu    sync.Mutex
	marketCache []MarketEntry
	marketAt    time.Time
)

// SearchMarket 按关键字 pi-package 搜 npm，结果短缓存；refresh 跳过缓存。
// 与版本查询同一套双源策略：当前源搜不到就换另一个再试一趟。
func SearchMarket(refresh bool) ([]MarketEntry, error) {
	marketMu.Lock()
	cached, at := marketCache, marketAt
	marketMu.Unlock()
	if !refresh && cached != nil && time.Since(at) < marketTTL {
		return cached, nil
	}

	first := Registry()
	entries, err := searchMarketAt(first)
	if err != nil {
		if alt, altErr := searchMarketAt(AlternateRegistry(first)); altErr == nil {
			entries, err = alt, nil
		}
	}
	if err != nil {
		return nil, err
	}
	marketMu.Lock()
	marketCache, marketAt = entries, time.Now()
	marketMu.Unlock()
	return entries, nil
}

// searchMarketAt 从指定源按关键字搜包。npm 的 search 端点是标准接口，官方源与镜像同一形状。
func searchMarketAt(base string) ([]MarketEntry, error) {
	query := url.Values{}
	query.Set("text", "keywords:"+marketKeyword)
	query.Set("size", "250")
	client := &http.Client{Timeout: registryTimeout}
	target := normalizeRegistry(base) + "/-/v1/search?" + query.Encode()
	resp, err := client.Get(target)
	if err != nil {
		return nil, pkg.Wrap("9724", "连不上 npm 注册表，拉不到插件市场", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, pkg.New("9724", "npm 注册表返回异常", strings.TrimSpace(resp.Status))
	}
	var doc struct {
		Objects []struct {
			Package struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
				Date        string `json:"date"`
				Publisher   struct {
					Username string `json:"username"`
				} `json:"publisher"`
				Links struct {
					Repository string `json:"repository"`
				} `json:"links"`
			} `json:"package"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, pkg.Wrap("9724", "插件市场的响应读不出来", err)
	}
	out := make([]MarketEntry, 0, len(doc.Objects))
	for _, obj := range doc.Objects {
		p := obj.Package
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		out = append(out, MarketEntry{
			Name:        p.Name,
			Description: p.Description,
			Version:     p.Version,
			Publisher:   p.Publisher.Username,
			Repo:        p.Links.Repository,
			PublishedAt: isoMillis(p.Date),
		})
	}
	return out, nil
}

// isoMillis 把注册表给的 ISO 时间解成毫秒；解不出返回 0。
func isoMillis(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.UnixMilli()
	}
	return 0
}

// marketKeyword 是 PI 官方的包发现关键字：带上它的 npm 包才进 pi.dev 的包画廊。
const marketKeyword = "pi-package"
