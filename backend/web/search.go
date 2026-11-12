package web

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"WorkBaby/backend/pkg"
)

// SearchResult 是一条搜索结果。
type SearchResult struct {
	// ResultTitle 标题
	ResultTitle string `json:"result_title"`
	// ResultURL 地址
	ResultURL string `json:"result_url"`
	// ResultSnippet 摘要
	ResultSnippet string `json:"result_snippet"`
	// ResultSource 来源站点名
	ResultSource string `json:"result_source,omitempty"`
}

// SearchProvider 是一个搜索服务的适配器。
// 各家搜索的请求形状与返回形状都不同，统一在这里收口：
// 上层只认 SearchResult，不认各家字段。
type SearchProvider interface {
	// ProviderName 提供商标识
	ProviderName() string
	// Search 执行一次搜索
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
}

// ProviderKind 是搜索服务的种类。
type ProviderKind string

const (
	// ProviderDuckDuckGo 免密钥的默认搜索
	ProviderDuckDuckGo ProviderKind = "duckduckgo"
	// ProviderBrave Brave 搜索（需密钥）
	ProviderBrave ProviderKind = "brave"
	// ProviderExa Exa 搜索（需密钥，偏语义检索）
	ProviderExa ProviderKind = "exa"
	// ProviderTavily Tavily 搜索（需密钥，专为 AI 检索设计）
	ProviderTavily ProviderKind = "tavily"
)

// SearchConfig 是搜索服务的配置。
type SearchConfig struct {
	// Kind 提供商种类
	Kind ProviderKind `json:"kind"`
	// ApiKey 密钥；免密钥的提供商会忽略它
	ApiKey string `json:"api_key"`
	// MaxResults 单次最多返回几条
	MaxResults int `json:"max_results"`
}

// NewSearchProvider 按配置造一个搜索适配器。
func NewSearchProvider(config SearchConfig, client *Client) SearchProvider {
	switch config.Kind {
	case ProviderBrave:
		return &braveProvider{config: config, client: client}
	case ProviderExa:
		return &exaProvider{config: config, client: client}
	case ProviderTavily:
		return &tavilyProvider{config: config, client: client}
	default:
		return &duckduckgoProvider{config: config, client: client}
	}
}

// limitOf 取单次结果条数上限。
func limitOf(config SearchConfig) int {
	if config.MaxResults > 0 && config.MaxResults <= 20 {
		return config.MaxResults
	}
	return 8
}

// duckduckgoProvider 是免密钥的默认搜索：走 DuckDuckGo 的 HTML 端点。
// 免密钥的意义是「开箱能用」——用户配第一个模型就能联网搜索，
// 不必先去申请一个搜索服务的账号。
type duckduckgoProvider struct {
	// config 配置
	config SearchConfig
	// client 抓取客户端
	client *Client
}

// ProviderName 提供商标识。
func (p *duckduckgoProvider) ProviderName() string { return string(ProviderDuckDuckGo) }

// duckduckgoEndpoint 是搜索端点。
const duckduckgoEndpoint = "https://html.duckduckgo.com/html/"

// Search 执行一次搜索。
func (p *duckduckgoProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, pkg.Err(pkg.CodeBadRequest, "搜索词不能为空")
	}
	url := duckduckgoEndpoint + "?q=" + queryEscape(trimmed)
	response, err := p.client.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, pkg.Err(pkg.CodeProvision, "搜索失败", "上游返回 "+itoa(response.StatusCode))
	}
	results := parseDuckDuckGo(response.Body, limitOf(p.config))
	if len(results) == 0 {
		return nil, pkg.Err(pkg.CodeNotFound, "没有搜到结果", "换个说法再试一次")
	}
	return results, nil
}

// braveProvider 是 Brave 搜索。
type braveProvider struct {
	// config 配置
	config SearchConfig
	// client 抓取客户端
	client *Client
}

// ProviderName 提供商标识。
func (p *braveProvider) ProviderName() string { return string(ProviderBrave) }

// Search 执行一次搜索。
func (p *braveProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(p.config.ApiKey) == "" {
		return nil, pkg.Err(pkg.CodeProviderBad, "Brave 搜索需要密钥")
	}
	url := "https://api.search.brave.com/res/v1/web/search?q=" + queryEscape(strings.TrimSpace(query))
	response, err := p.client.GetJSON(ctx, url, map[string]string{
		"X-Subscription-Token": p.config.ApiKey,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, pkg.Err(pkg.CodeProvision, "搜索失败", "上游返回 "+itoa(response.StatusCode))
	}
	var envelope struct {
		Web struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Desc    string `json:"description"`
				Profile string `json:"profile"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal([]byte(response.Body), &envelope); err != nil {
		return nil, pkg.Err(pkg.CodeProvision, "搜索结果解析失败", err.Error())
	}
	out := make([]SearchResult, 0, len(envelope.Web.Results))
	for _, item := range envelope.Web.Results {
		out = append(out, SearchResult{
			ResultTitle:   item.Title,
			ResultURL:     item.URL,
			ResultSnippet: item.Desc,
			ResultSource:  item.Profile,
		})
		if len(out) >= limitOf(p.config) {
			break
		}
	}
	if len(out) == 0 {
		return nil, pkg.Err(pkg.CodeNotFound, "没有搜到结果", "")
	}
	return out, nil
}

// exaProvider 是 Exa 搜索：偏语义检索，适合找技术资料。
type exaProvider struct {
	// config 配置
	config SearchConfig
	// client 抓取客户端
	client *Client
}

// ProviderName 提供商标识。
func (p *exaProvider) ProviderName() string { return string(ProviderExa) }

// Search 执行一次搜索。
func (p *exaProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(p.config.ApiKey) == "" {
		return nil, pkg.Err(pkg.CodeProviderBad, "Exa 搜索需要密钥")
	}
	payload := map[string]any{
		"query":      strings.TrimSpace(query),
		"numResults": limitOf(p.config),
		"contents":   map[string]any{"text": map[string]any{"maxCharacters": 1200}},
	}
	response, err := p.client.PostJSON(ctx, "https://api.exa.ai/search", payload, map[string]string{
		"x-api-key": p.config.ApiKey,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, pkg.Err(pkg.CodeProvision, "搜索失败", "上游返回 "+itoa(response.StatusCode))
	}
	var envelope struct {
		Results []struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Text   string `json:"text"`
			Author string `json:"author"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(response.Body), &envelope); err != nil {
		return nil, pkg.Err(pkg.CodeProvision, "搜索结果解析失败", err.Error())
	}
	out := make([]SearchResult, 0, len(envelope.Results))
	for _, item := range envelope.Results {
		out = append(out, SearchResult{
			ResultTitle:   item.Title,
			ResultURL:     item.URL,
			ResultSnippet: item.Text,
			ResultSource:  item.Author,
		})
	}
	if len(out) == 0 {
		return nil, pkg.Err(pkg.CodeNotFound, "没有搜到结果", "")
	}
	return out, nil
}

// tavilyProvider 是 Tavily 搜索：返回带答案的检索结果。
type tavilyProvider struct {
	// config 配置
	config SearchConfig
	// client 抓取客户端
	client *Client
}

// ProviderName 提供商标识。
func (p *tavilyProvider) ProviderName() string { return string(ProviderTavily) }

// Search 执行一次搜索。
func (p *tavilyProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(p.config.ApiKey) == "" {
		return nil, pkg.Err(pkg.CodeProviderBad, "Tavily 搜索需要密钥")
	}
	payload := map[string]any{
		"query":       strings.TrimSpace(query),
		"max_results": limitOf(p.config),
	}
	response, err := p.client.PostJSON(ctx, "https://api.tavily.com/search", payload, map[string]string{
		"Authorization": "Bearer " + p.config.ApiKey,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, pkg.Err(pkg.CodeProvision, "搜索失败", "上游返回 "+itoa(response.StatusCode))
	}
	var envelope struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(response.Body), &envelope); err != nil {
		return nil, pkg.Err(pkg.CodeProvision, "搜索结果解析失败", err.Error())
	}
	out := make([]SearchResult, 0, len(envelope.Results))
	for _, item := range envelope.Results {
		out = append(out, SearchResult{
			ResultTitle:   item.Title,
			ResultURL:     item.URL,
			ResultSnippet: item.Content,
		})
	}
	if len(out) == 0 {
		return nil, pkg.Err(pkg.CodeNotFound, "没有搜到结果", "")
	}
	return out, nil
}

// queryEscape 做查询串转义。
func queryEscape(text string) string {
	return url.QueryEscape(text)
}

// itoa 十进制转换。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
