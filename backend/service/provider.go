// 模型服务编排：读写 PI 的 models.json 与 settings.json 里的默认选项。
// WorkBaby 不再自存一份服务商表——用户在别处（cc-switch、pi、手工编辑）配的服务商
// 打开就能看到，界面上改的也立刻是 pi 命令读到的那份。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// modelListTimeout 是拉模型列表的整请求上限。
const modelListTimeout = 20 * time.Second

// ProviderService 管理模型服务配置。
type ProviderService struct {
	env *Env
}

// NewProviderService 构造模型服务。
func NewProviderService(env *Env) *ProviderService { return &ProviderService{env: env} }

// List 列出 models.json 里的服务商 + settings.json 的默认模型。
func (p *ProviderService) List() (*domain.ProviderListRESP, error) {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return nil, err
	}
	settings, err := p.env.PiEnv.ReadSettings()
	if err != nil {
		settings = &pienv.SettingsFile{}
	}
	out := &domain.ProviderListRESP{
		Providers:    make([]domain.ProviderVO, 0, len(providers)),
		DefaultID:    settings.DefaultProvider,
		DefaultModel: settings.DefaultModel,
		Path:         p.env.PiEnv.ModelsPath(),
	}
	keys := make([]string, 0, len(providers))
	for key := range providers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var cfg pienv.ProviderConfig
		if err := json.Unmarshal(providers[key], &cfg); err != nil {
			continue
		}
		out.Providers = append(out.Providers, toProviderVO(key, cfg, settings.DefaultProvider == key))
	}
	return out, nil
}

// toProviderVO 把 models.json 的一条转成出参：密钥只给掩码。
func toProviderVO(id string, cfg pienv.ProviderConfig, isDefault bool) domain.ProviderVO {
	vo := domain.ProviderVO{
		ID: id, Name: cfg.Name, API: cfg.API, BaseURL: cfg.BaseURL,
		HasKey: strings.TrimSpace(cfg.APIKey) != "", KeyHint: maskKey(cfg.APIKey),
		IsDefault: isDefault, Models: make([]domain.ModelVO, 0, len(cfg.Models)),
	}
	if vo.Name == "" {
		vo.Name = id
	}
	for _, m := range cfg.Models {
		vo.Models = append(vo.Models, modelVO(m))
	}
	return vo
}

// modelVO 转一个模型条目；视觉能力由 input 里有没有 image 决定。
func modelVO(m pienv.ModelConfig) domain.ModelVO {
	v := domain.ModelVO{
		ID: m.ID, Name: m.Name, Reasoning: m.Reasoning,
		ContextWindow: m.ContextWindow, MaxTokens: m.MaxTokens,
	}
	if v.Name == "" {
		v.Name = m.ID
	}
	for _, in := range m.Input {
		if in == "image" {
			v.Vision = true
		}
	}
	levels := make([]string, 0, len(m.ThinkingMap))
	for level, mapped := range m.ThinkingMap {
		if mapped != "" {
			levels = append(levels, level)
		}
	}
	sort.Strings(levels)
	v.ThinkingLevel = levels
	return v
}

// maskKey 只暗示密钥的头尾，界面里永远看不到完整明文。
func maskKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	// $ENV / !command 是 PI 的取值表达式，不是密钥本身：原样显示更有信息量。
	if strings.HasPrefix(key, "$") || strings.HasPrefix(key, "!") {
		return key
	}
	if len(key) <= 10 {
		return "••••"
	}
	return key[:4] + "••••" + key[len(key)-4:]
}

// HasUsable 报告是否至少有一个服务配了密钥与模型：环境闸门的 model 判定。
func (p *ProviderService) HasUsable() bool {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return false
	}
	for _, raw := range providers {
		var cfg pienv.ProviderConfig
		if json.Unmarshal(raw, &cfg) != nil {
			continue
		}
		if len(cfg.Models) > 0 && (cfg.APIKey != "" || cfg.BaseURL != "") {
			return true
		}
	}
	return false
}

// Upsert 新增或更新一个服务商：只改我们掌管的字段，其余键原样保留。
func (p *ProviderService) Upsert(req domain.UpsertProviderREQ) (*domain.ProviderVO, error) {
	if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.ID) == "" {
		return nil, domain.ErrProviderName
	}
	api := req.API
	if api == "" {
		api = domain.APIOpenAICompletions
	}
	if !isKnownAPI(api) {
		return nil, pkg.Wrap("3108", domain.ErrProviderAPI.Message, fmt.Errorf("%s", api))
	}
	extra, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = slugOf(req.Name)
	}
	raw := providers[id]
	fields := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, pkg.Wrap("3105", "这个服务商的配置读不出来", err)
		}
	}
	set := func(key string, v any) {
		encoded, err := json.Marshal(v)
		if err != nil {
			return
		}
		fields[key] = encoded
	}
	set("name", firstNonEmpty(req.Name, id))
	set("api", api)
	set("baseUrl", strings.TrimSpace(req.BaseURL))
	// 空密钥表示「不改」：界面上掩码是空的，用户没动它就不该把已存的密钥清掉。
	if strings.TrimSpace(req.APIKey) != "" {
		set("apiKey", strings.TrimSpace(req.APIKey))
	}
	if req.Models != nil {
		set("models", toModelConfigs(req.Models))
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, pkg.Wrap("3105", "这个服务商的配置写不出去", err)
	}
	providers[id] = encoded
	if err := p.env.PiEnv.WriteModelsRaw(extra, providers); err != nil {
		return nil, err
	}
	settings, _ := p.env.PiEnv.ReadSettings()
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	vo := toProviderVO(id, providerConfigOf(fields), settings.DefaultProvider == id)
	return &vo, nil
}

// toModelConfigs 把界面上的模型行转成 PI 的模型条目。
func toModelConfigs(list []domain.ModelREQ) []pienv.ModelConfig {
	out := make([]pienv.ModelConfig, 0, len(list))
	for _, m := range list {
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		cfg := pienv.ModelConfig{
			ID: m.ID, Name: firstNonEmpty(m.Name, m.ID),
			Reasoning: m.Reasoning, ContextWindow: m.ContextWindow, MaxTokens: m.MaxTokens,
		}
		if m.Vision {
			cfg.Input = []string{"text", "image"}
		}
		out = append(out, cfg)
	}
	return out
}

// providerConfigOf 把原始字段解成结构体（写回后给界面回显）。
func providerConfigOf(fields map[string]json.RawMessage) pienv.ProviderConfig {
	encoded, _ := json.Marshal(fields)
	var cfg pienv.ProviderConfig
	_ = json.Unmarshal(encoded, &cfg)
	return cfg
}

// Delete 删掉一个服务商。
func (p *ProviderService) Delete(id string) error {
	extra, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return err
	}
	if _, ok := providers[id]; !ok {
		return domain.ErrProviderNotFound
	}
	delete(providers, id)
	if err := p.env.PiEnv.WriteModelsRaw(extra, providers); err != nil {
		return err
	}
	// 默认模型指向它时一起清掉，否则 PI 启动会去找一个不存在的服务商。
	settings, _ := p.env.PiEnv.ReadSettings()
	if settings != nil && settings.DefaultProvider == id {
		settings.DefaultProvider = ""
		settings.DefaultModel = ""
		if err := p.env.PiEnv.WriteSettings(settings); err != nil {
			return err
		}
	}
	return p.restartIdle()
}

// SetDefault 设默认模型：写 PI settings.json 的 defaultProvider / defaultModel。
func (p *ProviderService) SetDefault(req domain.SetDefaultREQ) error {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return err
	}
	if _, ok := providers[req.Provider]; !ok {
		return domain.ErrProviderNotFound
	}
	settings, err := p.env.PiEnv.ReadSettings()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	settings.DefaultProvider = req.Provider
	settings.DefaultModel = req.Model
	return p.env.PiEnv.WriteSettings(settings)
}

// Defaults 返回 PI 当前的默认模型。
func (p *ProviderService) Defaults() (string, string) {
	settings, err := p.env.PiEnv.ReadSettings()
	if err != nil || settings == nil {
		return "", ""
	}
	return settings.DefaultProvider, settings.DefaultModel
}

// EffectiveDefaults 返回界面要显示的默认模型：settings.json 里没设时，
// 取第一个有模型的服务——PI 自己也是这么自动选的，界面不能显示「没配模型」。
func (p *ProviderService) EffectiveDefaults() (string, string) {
	provider, model := p.Defaults()
	if provider != "" && model != "" {
		return provider, model
	}
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return provider, model
	}
	keys := make([]string, 0, len(providers))
	for key := range providers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cfg := providerConfigOf(rawToFields(providers[key]))
		if len(cfg.Models) == 0 {
			continue
		}
		return key, cfg.Models[0].ID
	}
	return provider, model
}

// Reveal 显式查看某个服务商的密钥明文（界面上点眼睛才走这一步）。
func (p *ProviderService) Reveal(id string) (string, error) {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return "", err
	}
	raw, ok := providers[id]
	if !ok {
		return "", domain.ErrProviderNotFound
	}
	cfg := providerConfigOf(rawToFields(raw))
	return cfg.APIKey, nil
}

// rawToFields 把一条原始 JSON 解成字段表。
func rawToFields(raw json.RawMessage) map[string]json.RawMessage {
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &fields)
	return fields
}

// Test 连通测试：用第一个模型向上游要一次模型列表。
func (p *ProviderService) Test(ctx context.Context, id string) (*domain.TestProviderRESP, error) {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return nil, err
	}
	raw, ok := providers[id]
	if !ok {
		return nil, domain.ErrProviderNotFound
	}
	cfg := providerConfigOf(rawToFields(raw))
	if len(cfg.Models) == 0 {
		return nil, domain.ErrProviderModel
	}
	names, err := listRemoteModels(ctx, cfg.API, cfg.BaseURL, cfg.APIKey)
	if err != nil {
		return &domain.TestProviderRESP{OK: false, Model: cfg.Models[0].ID, Message: err.Error()}, nil
	}
	return &domain.TestProviderRESP{
		OK: true, Model: cfg.Models[0].ID,
		Message: fmt.Sprintf("连上了，上游报告 %d 个模型", len(names)),
	}, nil
}

// FetchModels 用给定连接信息向上游要模型列表；ID 非空时可复用已存密钥。
func (p *ProviderService) FetchModels(ctx context.Context, req domain.FetchModelsREQ) (*domain.FetchModelsRESP, error) {
	api, base, key := req.API, req.BaseURL, req.APIKey
	if req.ID != "" {
		_, providers, err := p.env.PiEnv.ReadModelsRaw()
		if err != nil {
			return nil, err
		}
		if raw, ok := providers[req.ID]; ok {
			cfg := providerConfigOf(rawToFields(raw))
			if api == "" {
				api = cfg.API
			}
			if base == "" {
				base = cfg.BaseURL
			}
			if key == "" {
				key = cfg.APIKey
			}
		}
	}
	names, err := listRemoteModels(ctx, api, base, key)
	if err != nil {
		return nil, err
	}
	return &domain.FetchModelsRESP{Models: names}, nil
}

// Available 向 PI 进程要它眼下的可用模型（含内置服务商与登录态）。
func (p *ProviderService) Available(ctx context.Context, workspace string) ([]domain.AvailableModelVO, error) {
	if workspace == "" {
		workspace = p.env.Workspace()
	}
	sup := p.env.PiSupervisor()
	if sup == nil || workspace == "" {
		return nil, pkg.New("3112", "还没有可用的工作目录", "")
	}
	client, err := sup.Ensure(background(60*time.Second), workspace)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	list, err := client.AvailableModels(cctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AvailableModelVO, 0, len(list))
	for _, m := range list {
		vo := domain.AvailableModelVO{
			ID: m.ID, Name: m.Name, Provider: m.Provider,
			ContextWindow: m.ContextWindow, MaxTokens: m.MaxTokens, Reasoning: m.Reasoning,
		}
		for _, in := range m.Input {
			if in == "image" {
				vo.Vision = true
			}
		}
		out = append(out, vo)
	}
	return out, nil
}

// ModelSpec 取一个模型的能力（上下文窗口 / 输出上限），供水位环与预算用。
// 查不到返回 0，前端按「约」显示。
func (p *ProviderService) ModelSpec(provider, model string) (int, int) {
	_, providers, err := p.env.PiEnv.ReadModelsRaw()
	if err != nil {
		return 0, 0
	}
	if raw, ok := providers[provider]; ok {
		cfg := providerConfigOf(rawToFields(raw))
		for _, m := range cfg.Models {
			if m.ID == model {
				return m.ContextWindow, m.MaxTokens
			}
		}
	}
	// 服务商没写模型条目时，翻一遍全部服务商：同名模型的能力是一样的。
	for _, raw := range providers {
		cfg := providerConfigOf(rawToFields(raw))
		for _, m := range cfg.Models {
			if m.ID == model {
				return m.ContextWindow, m.MaxTokens
			}
		}
	}
	return 0, 0
}

// listRemoteModels 按接口类型向上游要模型清单。
func listRemoteModels(ctx context.Context, api, baseURL, apiKey string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, pkg.New("3106", "先填接口地址", "")
	}
	url := base + "/models"
	switch api {
	case domain.APIAnthropicMessages:
		if !strings.Contains(base, "/v1") {
			url = base + "/v1/models"
		}
	case domain.APIGoogleGenerative:
		url = base + "/models"
		if apiKey != "" {
			url += "?key=" + apiKey
		}
	}

	cctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, pkg.Wrap("3106", "接口地址不正确", err)
	}
	if apiKey != "" && !strings.HasPrefix(apiKey, "!") && !strings.HasPrefix(apiKey, "$") {
		switch api {
		case domain.APIAnthropicMessages:
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		case domain.APIGoogleGenerative:
			req.Header.Set("x-goog-api-key", apiKey)
		default:
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, pkg.Wrap("3106", "连不上这个接口，检查地址与网络", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, pkg.New("3107", fmt.Sprintf("上游返回 %d", resp.StatusCode), clipText(string(body), 240))
	}
	return parseModelNames(body), nil
}

// parseModelNames 兼容三种上游形状：OpenAI 的 data[]、Anthropic 的 data[]、
// Google 的 models[]。认不出就返回空表——报错比编一个模型名更有用。
func parseModelNames(body []byte) []string {
	var doc struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, m := range doc.Data {
		add(firstNonEmpty(m.ID, m.Name))
	}
	for _, m := range doc.Models {
		// Google 的 name 形如 models/gemini-2.0，界面只留最后一段。
		add(strings.TrimPrefix(firstNonEmpty(m.ID, m.Name), "models/"))
	}
	sort.Strings(out)
	return out
}

// restartIdle 重启空闲进程，让新的配置生效。
func (p *ProviderService) restartIdle() error {
	sup := p.env.PiSupervisor()
	if sup == nil {
		return nil
	}
	return sup.RestartIdle(context.Background())
}

// isKnownAPI 校验接口类型在我们认识的范围内。
func isKnownAPI(api string) bool {
	for _, v := range domain.APIAll {
		if v == api {
			return true
		}
	}
	return false
}

// slugOf 从显示名生成一个可用的服务商键。
var nonSlug = regexp.MustCompile(`[^a-z0-9._-]+`)

func slugOf(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonSlug.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "provider"
	}
	return slug
}

// firstNonEmpty 返回第一个非空值。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
