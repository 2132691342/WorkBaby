package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/factory"
	"WorkBaby/internal/pkg"
)

// ProviderService 管理模型服务配置并负责构造上游适配器。
type ProviderService struct {
	env *Env
}

// NewProviderService 构造模型服务。
func NewProviderService(env *Env) *ProviderService { return &ProviderService{env: env} }

// List 列出模型服务，不返回密钥。
func (p *ProviderService) List() ([]domain.ProviderVO, error) {
	list, err := p.env.Repo.ListProviders()
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProviderVO, 0, len(list))
	for i := range list {
		out = append(out, providerVO(&list[i], parseModels(list[i].Models)))
	}
	return out, nil
}

// Upsert 新增或更新；密钥用 AES-GCM 加密后落库。
func (p *ProviderService) Upsert(req domain.UpsertProviderREQ) (*domain.ProviderVO, error) {
	if req.Name == "" {
		return nil, pkg.New(3109, "请给这个服务起个名字", "")
	}
	switch req.API {
	case domain.APIOpenAI, domain.APIAnthropic, domain.APIOllama:
	default:
		// 测试注入的假实现也放行，方便集成测试不联网跑通全链路。
		if !factory.HasOverride(req.API) {
			return nil, pkg.New(3108, "不支持的模型服务类型", req.API)
		}
	}

	d := &domain.ProviderDO{
		ID:      req.ID,
		Name:    req.Name,
		API:     req.API,
		BaseURL: req.BaseURL,
		Enabled: true,
	}
	if req.ID == "" {
		d.ID = pkg.NewID(domain.PrefixProvider)
	} else {
		old, err := p.env.Repo.GetProvider(req.ID)
		if err != nil {
			return nil, err
		}
		d.Enabled = old.Enabled
		d.IsDefault = old.IsDefault
		d.Models = old.Models
		if req.APIKey == "" {
			d.APIKeyEnc = old.APIKeyEnc
		}
	}
	if req.APIKey != "" {
		enc, err := pkg.Encrypt(p.env.Cfg.MasterKey, req.APIKey)
		if err != nil {
			return nil, err
		}
		d.APIKeyEnc = enc
	}
	if len(req.Models) > 0 {
		raw, _ := json.Marshal(req.Models)
		d.Models = string(raw)
	}
	if err := p.env.Repo.UpsertProvider(d); err != nil {
		return nil, err
	}

	// 第一个服务自动成为默认，避免用户配完还要再点一次。
	all, err := p.env.Repo.ListProviders()
	if err == nil && len(all) == 1 {
		_ = p.SetDefault(d.ID)
		d.IsDefault = true
	}
	vo := providerVO(d, parseModels(d.Models))
	return &vo, nil
}

// Delete 删除模型服务。
func (p *ProviderService) Delete(id string) error { return p.env.Repo.DeleteProvider(id) }

// SetDefault 设为默认；同时写入设置便于会话缺省继承。
func (p *ProviderService) SetDefault(id string) error {
	if err := p.env.Repo.ClearDefault(); err != nil {
		return err
	}
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return err
	}
	d.IsDefault = true
	if err := p.env.Repo.UpsertProvider(d); err != nil {
		return err
	}
	return p.env.Repo.SetSetting(domain.SettingDefaultProvider, id)
}

// Test 连通测试：发一条极短请求，只关心能不能通。
func (p *ProviderService) Test(id string) (*domain.TestProviderRESP, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return nil, err
	}
	streamer, model, err := p.build(d, "")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	events, err := streamer.Stream(ctx, llm.Request{
		Model:    model,
		System:   "你是一个连通性测试助手。",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "回复 OK"}},
	})
	if err != nil {
		return &domain.TestProviderRESP{OK: false, Model: model, Detail: err.Error()}, nil
	}
	ok := false
	for ev := range events {
		if ev.Type == llm.EventDelta || ev.Type == llm.EventDone {
			ok = true
		}
		if ev.Type == llm.EventError && ev.Err != nil {
			return &domain.TestProviderRESP{OK: false, Model: model, Detail: ev.Err.Error()}, nil
		}
	}
	return &domain.TestProviderRESP{OK: ok, Model: model, Detail: "连通正常"}, nil
}

// Models 拉取上游模型列表。
func (p *ProviderService) Models(id string) ([]string, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return nil, err
	}
	key, err := p.keyOf(d)
	if err != nil {
		return nil, err
	}
	endpoint := modelsEndpoint(d)
	if endpoint == "" {
		return parseModels(d.Models), nil
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, pkg.Wrap(3103, "构造模型列表请求失败", err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if d.API == domain.APIAnthropic {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, pkg.Wrap(3103, "拉取模型列表失败", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, pkg.New(3103, "拉取模型列表失败", strings.TrimSpace(string(raw)))
	}
	var payload struct {
		Data  []struct{ ID string `json:"id"` }     `json:"data"`
		Models []struct{ Name string `json:"name"` } `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return parseModels(d.Models), nil
	}
	out := []string{}
	for _, m := range payload.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	for _, m := range payload.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	if len(out) == 0 {
		return parseModels(d.Models), nil
	}
	return out, nil
}

// Streamer 按会话配置构造适配器；缺省回落到默认服务。
func (p *ProviderService) Streamer(providerID, model string) (llm.Streamer, string, error) {
	d, err := p.pick(providerID)
	if err != nil {
		return nil, "", err
	}
	return p.build(d, model)
}

// pick 取指定服务，或默认的那个。
func (p *ProviderService) pick(providerID string) (*domain.ProviderDO, error) {
	if providerID != "" {
		return p.env.Repo.GetProvider(providerID)
	}
	list, err := p.env.Repo.ListProviders()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].IsDefault && list[i].Enabled {
			return &list[i], nil
		}
	}
	for i := range list {
		if list[i].Enabled {
			return &list[i], nil
		}
	}
	return nil, domain.ErrNoProvider
}

// build 解密密钥并构造适配器；模型缺省取该服务已知的第一个。
func (p *ProviderService) build(d *domain.ProviderDO, model string) (llm.Streamer, string, error) {
	key, err := p.keyOf(d)
	if err != nil {
		return nil, "", err
	}
	if model == "" {
		if ms := parseModels(d.Models); len(ms) > 0 {
			model = ms[0]
		}
	}
	if model == "" {
		model = defaultModelOf(d.API)
	}
	streamer, err := factory.NewStreamer(llm.ClientConfig{
		API: d.API, BaseURL: d.BaseURL, APIKey: key, Model: model,
	})
	if err != nil {
		return nil, "", err
	}
	return streamer, model, nil
}

// keyOf 解密密钥；Ollama 通常不需要密钥。
func (p *ProviderService) keyOf(d *domain.ProviderDO) (string, error) {
	if d.APIKeyEnc == "" {
		return "", nil
	}
	return pkg.Decrypt(p.env.Cfg.MasterKey, d.APIKeyEnc)
}

func modelsEndpoint(d *domain.ProviderDO) string {
	base := strings.TrimRight(d.BaseURL, "/")
	switch d.API {
	case domain.APIOllama:
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return base + "/api/tags"
	case domain.APIAnthropic:
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return base + "/v1/models"
	default:
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return base + "/models"
	}
}

func defaultModelOf(api string) string {
	switch api {
	case domain.APIAnthropic:
		return "claude-sonnet-4-20250514"
	case domain.APIOllama:
		return "qwen2.5:7b"
	default:
		return "gpt-4o-mini"
	}
}

func providerVO(d *domain.ProviderDO, models []string) domain.ProviderVO {
	return domain.ProviderVO{
		ID: d.ID, Name: d.Name, API: d.API, BaseURL: d.BaseURL,
		HasKey: d.APIKeyEnc != "", Models: models, IsDefault: d.IsDefault,
		Enabled: d.Enabled, CreatedAt: d.CreatedAt,
	}
}

func parseModels(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
