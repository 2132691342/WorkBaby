package service

import (
	"context"
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/web"
)

// Web 是联网能力的业务门面。
// 它把「搜索服务配置」变成引擎能用的工具，并把抓取结果整理成模型能读的文本。
type Web struct {
	// Settings 设置（搜索服务配置与密钥）
	Settings *Settings
	// client 抓取客户端
	client *web.Client
	// provider 当前的搜索适配器
	provider web.SearchProvider
	// providerKind 当前适配器的种类
	providerKind web.ProviderKind
}

// NewWeb 造一个联网门面。
func NewWeb(settings *Settings) *Web {
	w := &Web{
		Settings: settings,
		client:   web.NewClient(),
	}
	w.reload()
	return w
}

// reload 按设置里的配置重建搜索适配器。
// 搜索服务可以在设置页改，改完要立刻生效——所以每次都从设置读。
func (w *Web) reload() {
	config := w.config()
	w.providerKind = config.Kind
	w.provider = web.NewSearchProvider(config, w.client)
}

// config 读出搜索服务配置。
func (w *Web) config() web.SearchConfig {
	kind := web.ProviderKind(w.Settings.Get(KeyWebSearchKind))
	if kind == "" {
		kind = web.ProviderDuckDuckGo
	}
	return web.SearchConfig{
		Kind:       kind,
		ApiKey:     w.Settings.Get(KeyWebSearchKey),
		MaxResults: 8,
	}
}

// Kit 造给引擎的联网出口。
// 每次调用都重建适配器：设置改了之后不必重启应用才生效。
func (w *Web) Kit() agent.WebKit {
	return agent.WebKitFunc{
		SearchFunc: w.Search,
		FetchFunc:  w.Fetch,
	}
}

// Search 联网搜索。
func (w *Web) Search(ctx context.Context, query string, limit int) ([]web.SearchResult, error) {
	config := w.config()
	provider := web.NewSearchProvider(config, w.client)
	return provider.Search(ctx, query, limit)
}

// Fetch 抓取一个地址并抽出可读正文。
// 抽不出正文就把原文给模型：有些站点结构古怪，抽取规则会漏内容，
// 那时原文比空内容有用。
func (w *Web) Fetch(ctx context.Context, rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if err := web.ValidateURL(trimmed); err != nil {
		return "", err
	}
	response, err := w.client.Get(ctx, trimmed)
	if err != nil {
		return "", err
	}
	if response.StatusCode != 200 {
		return "", pkg.Err(pkg.CodeProvision, "抓取失败", "上游返回 "+itoa(response.StatusCode))
	}
	contentType := strings.ToLower(response.ContentType)
	switch {
	case strings.Contains(contentType, "pdf"):
		text := web.ExtractPDFText([]byte(response.Body))
		if strings.TrimSpace(text) == "" {
			return "", pkg.Err(pkg.CodeNotFound, "这个 PDF 没有抽出文字（可能是扫描件）")
		}
		return text, nil
	case strings.Contains(contentType, "json"):
		return response.Body, nil
	case strings.Contains(contentType, "html"), strings.Contains(contentType, "text/"):
		text := web.ExtractReadable(response.Body)
		if strings.TrimSpace(text) == "" {
			return response.Body, nil
		}
		return text, nil
	default:
		return response.Body, nil
	}
}

// Status 返回联网能力的当前状态；界面据此显示「已就绪 / 没配搜索服务」。
func (w *Web) Status() domain.WebStatusVO {
	config := w.config()
	needsKey := config.Kind != web.ProviderDuckDuckGo
	ready := !needsKey || strings.TrimSpace(config.ApiKey) != ""
	message := "联网搜索已就绪（" + string(config.Kind) + "）"
	if !ready {
		message = "搜索服务「" + string(config.Kind) + "」还没填密钥，去设置页补上"
	}
	return domain.WebStatusVO{
		WebKind:    string(config.Kind),
		WebReady:   ready,
		WebMessage: message,
		WebHasKey:  strings.TrimSpace(config.ApiKey) != "",
	}
}

// SearchRESP 执行一次搜索并组装成界面要的形状。
func (w *Web) SearchRESP(ctx context.Context, query string, limit int) (domain.WebSearchRESP, error) {
	results, err := w.Search(ctx, query, limit)
	if err != nil {
		return domain.WebSearchRESP{}, err
	}
	items := make([]domain.WebSearchItemVO, 0, len(results))
	for _, item := range results {
		items = append(items, domain.WebSearchItemVO{
			ResultTitle:   item.ResultTitle,
			ResultURL:     item.ResultURL,
			ResultSnippet: item.ResultSnippet,
			ResultSource:  item.ResultSource,
		})
	}
	return domain.WebSearchRESP{
		SearchResults:  items,
		SearchProvider: string(w.config().Kind),
	}, nil
}

// FetchRESP 抓取一个地址并组装成界面要的形状。
func (w *Web) FetchRESP(ctx context.Context, rawURL string) (domain.WebFetchRESP, error) {
	text, err := w.Fetch(ctx, rawURL)
	if err != nil {
		return domain.WebFetchRESP{}, err
	}
	return domain.WebFetchRESP{
		FetchUrl:       strings.TrimSpace(rawURL),
		FetchText:      text,
		FetchCharCount: len([]rune(text)),
	}, nil
}

// SaveSettings 保存联网设置并返回新的状态。
// 密钥空串语义是「保留原值」：界面拿到的是掩码，回传的必然不是原文。
func (w *Web) SaveSettings(req domain.WebSettingsREQ) (domain.WebStatusRESP, error) {
	values := map[string]string{}
	if kind := strings.TrimSpace(req.WebKind); kind != "" {
		switch web.ProviderKind(kind) {
		case web.ProviderDuckDuckGo, web.ProviderBrave, web.ProviderExa, web.ProviderTavily:
			values[KeyWebSearchKind] = kind
		default:
			return domain.WebStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "不认识的搜索服务："+kind)
		}
	}
	if key := strings.TrimSpace(req.WebApiKey); key != "" {
		values[KeyWebSearchKey] = key
	}
	if len(values) > 0 {
		if err := w.Settings.SaveSettings(values); err != nil {
			return domain.WebStatusRESP{}, err
		}
	}
	return w.SettingsRESP(), nil
}

// SettingsRESP 组装联网设置与状态。
func (w *Web) SettingsRESP() domain.WebStatusRESP {
	status := w.Status()
	return domain.WebStatusRESP{
		WebKind:    status.WebKind,
		WebHasKey:  status.WebHasKey,
		WebMessage: status.WebMessage,
		WebKinds: []string{
			string(web.ProviderDuckDuckGo),
			string(web.ProviderBrave),
			string(web.ProviderExa),
			string(web.ProviderTavily),
		},
	}
}
