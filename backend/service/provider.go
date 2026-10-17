// 模型服务编排：凭据加密、模型的增删改，并把结果写进 PI 的 models.json。
// WorkBaby 是 models.json 的唯一写入者，PI 只读——两边都能写会让「改了没生效」无法归因。
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
	envfile "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// modelListTimeout 是拉模型列表的整请求上限。
const modelListTimeout = 20 * time.Second

// ProviderService 管理模型服务配置。
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

// HasUsable 报告是否至少有一个可用服务配了模型：环境闸门的 model 判定。
func (p *ProviderService) HasUsable() bool {
	list, err := p.env.Repo.ListProviders()
	if err != nil {
		return false
	}
	for i := range list {
		if list[i].Enabled && len(parseModels(list[i].Models)) > 0 {
			return true
		}
	}
	return false
}

// Upsert 新增或更新；密钥用 AES-GCM 加密后落库，随后同步进 models.json。
func (p *ProviderService) Upsert(req domain.UpsertProviderREQ) (*domain.ProviderVO, error) {
	if req.Name == "" {
		return nil, pkg.New("3109", "请给这个服务起个名字", "")
	}
	if !slices.Contains(domain.APIAll, req.API) {
		return nil, pkg.New("3108", "不支持的模型服务类型", req.API)
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
	// 「落服务」与「定默认」必须一起生效：中途失败会留下「配好了却没有默认服务」。
	vo := domain.ProviderVO{}
	if err := p.env.Repo.WithTx(func(tx *repo.Repo) error {
		if err := tx.UpsertProvider(d); err != nil {
			return err
		}
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
			return err
		}
		vo = providerVO(d, parseModels(d.Models))
		return nil
	}); err != nil {
		return nil, err
	}
	// 落库成功后再同步到 PI 的配置：同步失败不该让保存看起来失败，
	// 但必须让用户看到原因，否则下一次对话会莫名其妙报模型不可用。
	if err := p.SyncToPi(); err != nil {
		pkg.Warnf("provider: 同步 models.json 失败: %v", err)
	}
	return &vo, nil
}

// Delete 删除模型服务，并级联清理它的模型级配置与默认指向。
func (p *ProviderService) Delete(id string) error {
	if err := p.env.Repo.WithTx(func(tx *repo.Repo) error {
		if err := tx.DeleteModelConfigsByProvider(id); err != nil {
			return err
		}
		if err := tx.DeleteProvider(id); err != nil {
			return err
		}
		return p.resetDefaultIfDeletedIn(tx, id)
	}); err != nil {
		return err
	}
	if err := p.SyncToPi(); err != nil {
		pkg.Warnf("provider: 同步 models.json 失败: %v", err)
	}
	return nil
}

// resetDefaultIfDeletedIn 删掉的正是默认服务时，把默认转交给还活着的服务。
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
	if err := p.env.Repo.WithTx(func(tx *repo.Repo) error { return p.setDefaultIn(tx, id) }); err != nil {
		return err
	}
	if err := p.SyncToPi(); err != nil {
		pkg.Warnf("provider: 同步 models.json 失败: %v", err)
	}
	return nil
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
		return "", pkg.New("3114", "这个服务还没有保存 API Key", "")
	}
	return p.keyOf(d)
}

// modelsHTTP 复用同一个连接池。
var modelsHTTP = &http.Client{
	Timeout: modelListTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     60 * time.Second,
	},
}

// Test 连通测试：发一条极短请求，只关心能不能通。
// 它不经过 PI（那要先进一次会话才知道成不成），直接打上游的 /models，
// 用 HTTP 状态码回答「地址与密钥对不对」——这是用户在这一步唯一想知道的事。
func (p *ProviderService) Test(ctx context.Context, id string) (*domain.TestProviderRESP, error) {
	d, err := p.env.Repo.GetProvider(id)
	if err != nil {
		return nil, err
	}
	models := parseModels(d.Models)
	if len(models) == 0 {
		return &domain.TestProviderRESP{OK: false, Model: "", Detail: "这个服务还没有添加模型"}, nil
	}
	model := models[0]
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key, err := p.keyOf(d)
	if err != nil {
		return nil, err
	}
	_, err = p.fetch(ctx, d, key)
	if err != nil {
		return &domain.TestProviderRESP{OK: false, Model: model, Detail: err.Error()}, nil
	}
	return &domain.TestProviderRESP{OK: true, Model: model, Detail: "连通正常"}, nil
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
		return nil, pkg.Wrap("3103", "构造模型列表请求失败", err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if d.API == domain.APIAnthropicMessages {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := modelsHTTP.Do(req)
	if err != nil {
		return nil, pkg.Wrap("3103", "拉取模型列表失败", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, pkg.New("3103", "拉取模型列表失败", strings.TrimSpace(string(raw)))
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

// SyncToPi 把全部服务与模型写进 PI 的 models.json。
// 每次启动与每次改动后都跑一遍：内容由当前状态决定，幂等。
func (p *ProviderService) SyncToPi() error {
	list, err := p.env.Repo.ListProviders()
	if err != nil {
		return err
	}
	file, err := p.env.PiEnv.ReadModels()
	if err != nil {
		// 用户把 models.json 改坏了：覆盖它（它本来就归 WorkBaby 管），
		// 但只警告不阻断——界面上看到的就是 WorkBaby 里的配置。
		pkg.Warnf("provider: models.json 无法解析，将覆盖: %v", err)
		file = nil
	}
	if file == nil {
		file = &envfile.ModelsFile{}
	}
	file.Providers = map[string]envfile.ProviderConfig{}
	for i := range list {
		d := &list[i]
		if !d.Enabled {
			continue
		}
		key, err := p.keyOf(d)
		if err != nil {
			pkg.Warnf("provider: 解不开 %s 的密钥，跳过同步: %v", d.ID, err)
			continue
		}
		cfg := envfile.ProviderConfig{
			Name:       d.Name,
			BaseURL:    d.BaseURL,
			APIKey:     key,
			API:        d.API,
			AuthHeader: boolPtr(true),
		}
		for _, model := range parseModels(d.Models) {
			cfg.Models = append(cfg.Models, p.modelConfigOf(d, model))
		}
		file.Providers[d.ID] = cfg
	}
	return p.env.PiEnv.WriteModels(file)
}

// modelConfigOf 把一个模型连同它的覆写配置翻译成 PI 的模型条目。
func (p *ProviderService) modelConfigOf(d *domain.ProviderDO, model string) envfile.ModelConfig {
	cap := domain.ModelCapabilityOf(model)
	out := envfile.ModelConfig{
		ID:            model,
		Name:          model,
		ContextWindow: cap.ContextWindow,
		MaxTokens:     cap.MaxOutput,
		Reasoning:     cap.Thinking,
		Input:         []string{"text"},
	}
	if cap.Vision {
		out.Input = append(out.Input, "image")
	}
	// 用户覆写优先于内置目录。
	if cfg, err := p.env.Repo.GetModelConfig(d.ID, model); err == nil && cfg != nil {
		merged := cap.WithConfig(cfg)
		if merged.ContextWindow > 0 {
			out.ContextWindow = merged.ContextWindow
		}
		if merged.MaxOutput > 0 {
			out.MaxTokens = merged.MaxOutput
		}
		out.Reasoning = merged.Thinking
		if merged.Vision {
			out.Input = []string{"text", "image"}
		} else {
			out.Input = []string{"text"}
		}
	}
	return out
}

// ModelConfig 返回「内置目录 + 用户覆写」合并后的模型配置。
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
	merged := cap.WithConfig(d)
	vo.ContextWindow = merged.ContextWindow
	if d.ContextWindow > 0 {
		vo.WindowKnown = true
	}
	vo.MaxOutput = merged.MaxOutput
	vo.MaxOutputSet = d.MaxOutput > 0
	vo.Thinking = merged.Thinking
	vo.Vision = merged.Vision
	vo.ToolCall = merged.ToolCall
	vo.Temperature = d.Temperature
	vo.TopP = d.TopP
	return vo
}

// ListModelConfigs 列出一个服务下的全部模型配置。
func (p *ProviderService) ListModelConfigs(providerID string) ([]domain.ModelConfigVO, error) {
	if providerID == "" {
		providerID = defaultProviderID(p.env)
	}
	list, err := p.env.Repo.ListModelConfigs(providerID)
	if err != nil {
		return nil, err
	}
	// 一次取回全部覆写再在内存里合并：逐个 ModelConfig 会为每个模型多打一条查询。
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

// UpsertModelConfig 保存模型级配置并同步进 models.json。
func (p *ProviderService) UpsertModelConfig(req domain.UpsertModelConfigREQ) (*domain.ModelConfigVO, error) {
	if req.ProviderID == "" {
		return nil, pkg.New("3111", "缺少模型服务", "")
	}
	if req.Model == "" {
		return nil, pkg.New("3111", "缺少模型名", "")
	}
	if _, err := p.env.Repo.GetProvider(req.ProviderID); err != nil {
		return nil, err
	}
	d := &domain.ModelConfigDO{
		ProviderID: req.ProviderID, Model: req.Model,
		ContextWindow: req.ContextWindow,
		MaxOutput:     req.MaxOutput,
		Temperature:   domain.ClampSampling(req.Temperature),
		TopP:          domain.ClampSampling(req.TopP),
		Vision:        req.Vision, ToolCall: req.ToolCall, Thinking: req.Thinking,
	}
	if err := p.env.Repo.UpsertModelConfig(d); err != nil {
		return nil, err
	}
	if err := p.SyncToPi(); err != nil {
		pkg.Warnf("provider: 同步 models.json 失败: %v", err)
	}
	return p.ModelConfig(req.ProviderID, req.Model)
}

// FetchModels 用未保存的连接信息拉模型列表。
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

// keyOf 解密密钥；本地部署常在无需鉴权的端点后面。
func (p *ProviderService) keyOf(d *domain.ProviderDO) (string, error) {
	if d.APIKeyEnc == "" {
		return "", nil
	}
	return pkg.Decrypt(p.env.Cfg.MasterKey, d.APIKeyEnc)
}

func modelsEndpoint(d *domain.ProviderDO) string {
	base := strings.TrimRight(d.BaseURL, "/")
	switch d.API {
	case domain.APIAnthropicMessages:
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return base + "/v1/models"
	case domain.APIGoogle:
		if base == "" {
			base = "https://generativelanguage.googleapis.com/v1beta"
		}
		return base + "/models"
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

func boolPtr(b bool) *bool { return &b }
