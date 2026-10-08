package service

import (
	"slices"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/factory"
	"WorkBaby/backend/pkg"
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
	// nil 表示「这次没提交模型列表」，沿用旧的；空切片表示「清空」，要真的写空。
	if req.Models != nil {
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
	} else if err == nil {
		// 非首个服务也要校正默认模型：新会话只继承服务时模型名会是空的。
		_ = p.refreshDefaultModel(d)
	}
	vo := providerVO(d, parseModels(d.Models))
	return &vo, nil
}

// Delete 删除模型服务，并级联清理它的模型级配置与默认指向。
func (p *ProviderService) Delete(id string) error {
	if err := p.env.Repo.DeleteModelConfigsByProvider(id); err != nil {
		return err
	}
	if err := p.env.Repo.DeleteProvider(id); err != nil {
		return err
	}
	return p.resetDefaultIfDeleted(id)
}

// resetDefaultIfDeleted 删掉的正是默认服务时，把默认转交给还活着的服务。
// 都不剩就清空设置：留一个指向已删 id 的默认比没有默认更糟。
func (p *ProviderService) resetDefaultIfDeleted(id string) error {
	cur, _ := p.env.Repo.GetSetting(domain.SettingDefaultProvider)
	if cur != id {
		return nil
	}
	all, err := p.env.Repo.ListProviders()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		_ = p.env.Repo.SetSetting(domain.SettingDefaultProvider, "")
		_ = p.env.Repo.SetSetting(domain.SettingDefaultModel, "")
		return nil
	}
	return p.SetDefault(all[0].ID)
}

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
	if err := p.env.Repo.SetSetting(domain.SettingDefaultProvider, id); err != nil {
		return err
	}
	// 顺带校正默认模型：新建会话要从这里继承模型名，指向不存在的模型
	// 会让每条新对话都报 model not found。
	return p.refreshDefaultModel(d)
}

// refreshDefaultModel 校正默认模型：没有就取列表第一个，指向不存在的模型时也跟着换。
func (p *ProviderService) refreshDefaultModel(d *domain.ProviderDO) error {
	cur, _ := p.env.Repo.GetSetting(domain.SettingDefaultModel)
	models := parseModels(d.Models)
	if len(models) == 0 {
		return nil
	}
	if cur != "" && slices.Contains(models, cur) {
		return nil
	}
	return p.env.Repo.SetSetting(domain.SettingDefaultModel, models[0])
}

// Reveal 返回解密后的 API Key：设置页「显示密钥」时才会调用。
func (p *ProviderService) Reveal(id string) (string, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return "", err
	}
	if d.APIKeyEnc == "" {
		return "", pkg.New(3114, "这个服务还没有保存 API Key", "")
	}
	return p.keyOf(d)
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
	return p.fetch(d, key)
}

// fetch 按服务的接口类型请求上游模型列表。
func (p *ProviderService) fetch(d *domain.ProviderDO, key string) ([]string, error) {
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
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
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

// ModelConfig 返回「内置目录 + 用户覆写」合并后的模型配置。
// 没有覆写时 WindowKnown 与目录一致；覆写了窗口就视为确切值。
func (p *ProviderService) ModelConfig(providerID, model string) (*domain.ModelConfigVO, error) {
	if providerID == "" {
		providerID = defaultProviderID(p.env)
	}
	cap := domain.ModelCapabilityOf(model)
	vo := &domain.ModelConfigVO{
		ProviderID: providerID, Model: model,
		ContextWindow: cap.ContextWindow, WindowKnown: cap.Known,
		MaxOutput: cap.MaxOutput, Thinking: cap.Thinking,
		Vision: cap.Vision, ToolCall: cap.ToolCall,
		Temperature: domain.DefaultTemperature, TopP: domain.DefaultTopP,
	}
	d, err := p.env.Repo.GetModelConfig(providerID, model)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return vo, nil
	}
	if d.ContextWindow > 0 {
		vo.ContextWindow = d.ContextWindow
		vo.WindowKnown = true
	}
	if d.MaxOutput > 0 {
		vo.MaxOutput = d.MaxOutput
	}
	vo.Temperature = d.Temperature
	vo.TopP = d.TopP
	vo.Vision = d.Vision
	vo.ToolCall = d.ToolCall
	return vo, nil
}

// ListModelConfigs 列出一个服务下的全部模型配置；不指定服务就取默认的。
// 出参形状与 ModelConfig 一致（配置与内置目录合并后的 VO）：
// 直接吐 DO 的话窗口与识图这些「目录认得出的值」全是零，界面显示成未配置。
func (p *ProviderService) ListModelConfigs(providerID string) ([]domain.ModelConfigVO, error) {
	if providerID == "" {
		providerID = defaultProviderID(p.env)
	}
	list, err := p.env.Repo.ListModelConfigs(providerID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ModelConfigVO, 0, len(list))
	for _, d := range list {
		vo, verr := p.ModelConfig(providerID, d.Model)
		if verr != nil {
			return nil, verr
		}
		out = append(out, *vo)
	}
	return out, nil
}

// UpsertModelConfig 保存模型级配置。
func (p *ProviderService) UpsertModelConfig(req domain.UpsertModelConfigREQ) (*domain.ModelConfigVO, error) {
	if req.ProviderID == "" {
		return nil, pkg.New(3111, "缺少模型服务", "")
	}
	if req.Model == "" {
		return nil, pkg.New(3111, "缺少模型名", "")
	}
	if _, err := p.env.Repo.GetProvider(req.ProviderID); err != nil {
		return nil, err
	}
	d := &domain.ModelConfigDO{
		ProviderID: req.ProviderID, Model: req.Model,
		ContextWindow: req.ContextWindow, MaxOutput: req.MaxOutput,
		Temperature: req.Temperature, TopP: req.TopP,
		Vision: req.Vision, ToolCall: req.ToolCall,
	}
	if req.Temperature <= 0 {
		d.Temperature = domain.DefaultTemperature
	}
	if req.TopP <= 0 {
		d.TopP = domain.DefaultTopP
	}
	if err := p.env.Repo.UpsertModelConfig(d); err != nil {
		return nil, err
	}
	return p.ModelConfig(req.ProviderID, req.Model)
}

// FetchModels 用未保存的连接信息拉模型列表：新增服务时不用先保存才能看列表。
func (p *ProviderService) FetchModels(api, baseURL, apiKey string) ([]string, error) {
	d := &domain.ProviderDO{API: api, BaseURL: baseURL}
	if apiKey != "" {
		enc, err := pkg.Encrypt(p.env.Cfg.MasterKey, apiKey)
		if err != nil {
			return nil, err
		}
		d.APIKeyEnc = enc
	}
	key, err := p.keyOf(d)
	if err != nil {
		return nil, err
	}
	return p.fetch(d, key)
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
// 会话指定的模型必须在该服务已配置的列表里，否则本地直接报错——
// 比把上游的 model not found 原始 JSON 转述给用户有用得多。
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
	if ms := parseModels(d.Models); len(ms) > 0 && !slices.Contains(ms, model) {
		return nil, "", pkg.New(3112,
			"模型「"+model+"」不在服务「"+d.Name+"」的模型列表里",
			"在对话底部换个模型，或到设置里检查这个服务的模型配置")
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

// defaultProviderID 取默认服务 id；没有默认就取第一个。
func defaultProviderID(env *Env) string {
	if id, err := env.Repo.GetSetting(domain.SettingDefaultProvider); err == nil && id != "" {
		return id
	}
	list, err := env.Repo.ListProviders()
	if err != nil || len(list) == 0 {
		return ""
	}
	return list[0].ID
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
