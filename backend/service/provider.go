package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/factory"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// modelListTimeout 是拉模型列表的整请求上限：列表接口不该慢到让用户等，
// 但它是一次性 GET，不像流式响应那样需要区别对待「等响应头」与「等正文」。
const modelListTimeout = 20 * time.Second

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
	// 「落服务」与「定默认」必须一起生效：中途失败会留下「配好了却没有默认服务」，
	// 用户看到的是保存成功的提示，下一次发送却报没配模型。
	vo := domain.ProviderVO{}
	if err := p.env.Repo.WithTx(func(tx *repo.Repo) error {
		if err := tx.UpsertProvider(d); err != nil {
			return err
		}
		// 第一个服务自动成为默认，避免用户配完还要再点一次。
		all, err := tx.ListProviders()
		if err != nil {
			return err
		}
		if len(all) == 1 {
			if err := p.setDefaultIn(tx, d.ID); err != nil {
				return err
			}
			d.IsDefault = true
		} else if err := p.refreshDefaultModelIn(tx, d); err != nil {
			// 非首个服务也要校正默认模型：新会话只继承服务时模型名会是空的。
			return err
		}
		vo = providerVO(d, parseModels(d.Models))
		return nil
	}); err != nil {
		return nil, err
	}
	return &vo, nil
}

// Delete 删除模型服务，并级联清理它的模型级配置与默认指向。
func (p *ProviderService) Delete(id string) error {
	return p.env.Repo.WithTx(func(tx *repo.Repo) error {
		if err := tx.DeleteModelConfigsByProvider(id); err != nil {
			return err
		}
		if err := tx.DeleteProvider(id); err != nil {
			return err
		}
		return p.resetDefaultIfDeletedIn(tx, id)
	})
}

// resetDefaultIfDeletedIn 删掉的正是默认服务时，把默认转交给还活着的服务。
// 都不剩就清空设置：留一个指向已删 id 的默认比没有默认更糟。
func (p *ProviderService) resetDefaultIfDeletedIn(tx *repo.Repo, id string) error {
	cur, _ := tx.GetSetting(domain.SettingDefaultProvider)
	if cur != id {
		return nil
	}
	all, err := tx.ListProviders()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		if err := tx.SetSetting(domain.SettingDefaultProvider, ""); err != nil {
			return err
		}
		return tx.SetSetting(domain.SettingDefaultModel, "")
	}
	return p.setDefaultIn(tx, all[0].ID)
}

// SetDefault 设为默认；同时写入设置便于会话缺省继承。
func (p *ProviderService) SetDefault(id string) error {
	return p.env.Repo.WithTx(func(tx *repo.Repo) error { return p.setDefaultIn(tx, id) })
}

func (p *ProviderService) setDefaultIn(tx *repo.Repo, id string) error {
	if err := tx.ClearDefault(); err != nil {
		return err
	}
	d, err := tx.GetProvider(id)
	if err != nil {
		return err
	}
	d.IsDefault = true
	if err := tx.UpsertProvider(d); err != nil {
		return err
	}
	if err := tx.SetSetting(domain.SettingDefaultProvider, id); err != nil {
		return err
	}
	// 顺带校正默认模型：新建会话要从这里继承模型名，指向不存在的模型
	// 会让每条新对话都报 model not found。
	return p.refreshDefaultModelIn(tx, d)
}

// refreshDefaultModelIn 校正默认模型：没有就取列表第一个，指向不存在的模型时也跟着换。
func (p *ProviderService) refreshDefaultModelIn(tx *repo.Repo, d *domain.ProviderDO) error {
	cur, _ := tx.GetSetting(domain.SettingDefaultModel)
	models := parseModels(d.Models)
	if len(models) == 0 {
		return nil
	}
	if cur != "" && slices.Contains(models, cur) {
		return nil
	}
	return tx.SetSetting(domain.SettingDefaultModel, models[0])
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

// modelsHTTP 复用同一个连接池：每次新建 Client 都等于新建一个池，旧池的连接还在 TIME_WAIT。
var modelsHTTP = &http.Client{
	Timeout: modelListTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     60 * time.Second,
	},
}

// Test 连通测试：发一条极短请求，只关心能不能通。
func (p *ProviderService) Test(ctx context.Context, id string) (*domain.TestProviderRESP, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return nil, err
	}
	streamer, model, err := p.build(d, "")
	if err != nil {
		return nil, err
	}
	// ctx 从请求链路传下来：用户关了设置页，这次连通性探测就该跟着停
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
func (p *ProviderService) Models(ctx context.Context, id string) ([]string, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return nil, err
	}
	key, err := p.keyOf(d)
	if err != nil {
		return nil, err
	}
	return p.fetch(ctx, d, key)
}

// fetch 按服务的接口类型请求上游模型列表。
func (p *ProviderService) fetch(ctx context.Context, d *domain.ProviderDO, key string) ([]string, error) {
	endpoint := modelsEndpoint(d)
	if endpoint == "" {
		return parseModels(d.Models), nil
	}
	ctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
	resp, err := modelsHTTP.Do(req)
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
		Temperature: domain.SamplingUnset, TopP: domain.SamplingUnset,
	}
	d, err := p.env.Repo.GetModelConfig(providerID, model)
	if err != nil {
		return nil, err
	}
	return applyOverride(cap, vo, d), nil
}

// applyOverride 把用户覆写叠到目录能力上；d 为 nil 表示没有覆写。
func applyOverride(cap domain.ModelCapability, vo *domain.ModelConfigVO, d *domain.ModelConfigDO) *domain.ModelConfigVO {
	if d == nil {
		return vo
	}
	if d.ContextWindow > 0 {
		cap = cap.WithWindow(d.ContextWindow)
		vo.ContextWindow = cap.ContextWindow
		vo.WindowKnown = true
	}
	vo.MaxOutput = cap.OutputBudget()
	vo.Temperature = d.Temperature
	vo.TopP = d.TopP
	vo.Vision = d.Vision
	vo.ToolCall = d.ToolCall
	return vo
}

// ListModelConfigs 列出一个服务下的全部模型配置；不指定服务就取默认的。
// 出 VO 而不是 DO：吐 DO 的话「目录认得出的值」全是零，界面显示成未配置。
func (p *ProviderService) ListModelConfigs(providerID string) ([]domain.ModelConfigVO, error) {
	if providerID == "" {
		providerID = defaultProviderID(p.env)
	}
	list, err := p.env.Repo.ListModelConfigs(providerID)
	if err != nil {
		return nil, err
	}
	// 一次取回全部覆写再在内存里合并：逐个 ModelConfig 会为每个模型多打一条
	// GetModelConfig，配置一多就成了 N+1。
	out := make([]domain.ModelConfigVO, 0, len(list))
	for _, d := range list {
		item := d
		cap := domain.ModelCapabilityOf(item.Model)
		vo := &domain.ModelConfigVO{
			ProviderID: providerID, Model: item.Model,
			ContextWindow: cap.ContextWindow, WindowKnown: cap.Known,
			MaxOutput: cap.MaxOutput, Thinking: cap.Thinking,
			Vision: cap.Vision, ToolCall: cap.ToolCall,
			Temperature: domain.SamplingUnset, TopP: domain.SamplingUnset,
		}
		out = append(out, *applyOverride(cap, vo, &item))
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
		ContextWindow: req.ContextWindow,
		Temperature:   domain.ClampSampling(req.Temperature),
		TopP:          domain.ClampSampling(req.TopP),
		Vision:        req.Vision, ToolCall: req.ToolCall,
	}
	if err := p.env.Repo.UpsertModelConfig(d); err != nil {
		return nil, err
	}
	return p.ModelConfig(req.ProviderID, req.Model)
}

// FetchModels 用未保存的连接信息拉模型列表：新增服务时不用先保存才能看列表。
func (p *ProviderService) FetchModels(ctx context.Context, api, baseURL, apiKey string) ([]string, error) {
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
	return p.fetch(ctx, d, key)
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
// 模型必须在该服务的列表里，本地直接报错比转述上游的 model not found 更有用。
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
		// 兜底不再猜模型名：猜出来的名字在用户机器上大概率不存在，
		// 报「模型不存在」比直接说清「还没配模型」更难排查。
		return nil, "", pkg.New(3112,
			"服务「"+d.Name+"」还没有可用的模型",
			"到设置里给它添加一个模型（可从上游拉取），或在对话底部选择模型")
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
