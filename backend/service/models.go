package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Models 是模型与服务商编排：pi 的 models.json + auth.json。
type Models struct {
	// Layout 沙箱布局
	Layout pkg.Layout
}

// NewModels 造一个模型门面。
func NewModels(layout pkg.Layout) *Models {
	return &Models{Layout: layout}
}

// modelsFile 是 models.json 的形状。
type modelsFile struct {
	// Providers 服务商名 → 定义
	Providers map[string]providerFile `json:"providers"`
}

// providerFile 是一个服务商在 models.json 里的定义。
type providerFile struct {
	// BaseUrl 接口地址
	BaseUrl string `json:"baseUrl"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api"`
	// AuthKind 鉴权方式
	AuthKind string `json:"auth,omitempty"`
	// Models 模型清单
	Models []map[string]any `json:"models"`
}

// readModels 读 models.json；不存在或坏掉给空表。
func (m *Models) readModels() (modelsFile, error) {
	out := modelsFile{Providers: map[string]providerFile{}}
	if !pkg.Exists(m.Layout.ModelsFile()) {
		return out, nil
	}
	var value modelsFile
	pkg.ReadJSON(m.Layout.ModelsFile(), &value)
	if value.Providers == nil {
		value.Providers = map[string]providerFile{}
	}
	return value, nil
}

// authFile 是 auth.json 的形状：服务商名 → 凭据。其余键原样保留。
type authMap map[string]map[string]any

// readAuth 读 auth.json。
func (m *Models) readAuth() authMap {
	out := authMap{}
	pkg.ReadJSON(m.Layout.AuthFile(), &out)
	if out == nil {
		out = authMap{}
	}
	return out
}

// Providers 组装服务商清单。密钥恒为掩码：界面永远拿不到原文。
func (m *Models) Providers() domain.ProvidersRESP {
	config, _ := m.readModels()
	auth := m.readAuth()
	out := domain.ProvidersRESP{
		Providers:  make([]domain.ProviderVO, 0, len(config.Providers)),
		ConfigPath: m.Layout.ModelsFile(),
	}
	for name, entry := range config.Providers {
		models := make([]domain.ProviderModelEntryVO, 0, len(entry.Models))
		for _, raw := range entry.Models {
			models = append(models, modelEntryFromWire(raw))
		}
		key := ""
		if item, ok := auth[name]; ok {
			if value, ok := item["key"].(string); ok {
				key = value
			}
		}
		out.Providers = append(out.Providers, domain.ProviderVO{
			ProviderName: name,
			BaseUrl:      entry.BaseUrl,
			ApiProtocol:  entry.ApiProtocol,
			AuthKind:     defaultIfEmpty(entry.AuthKind, "api_key"),
			ApiKeyMasked: maskKey(key),
			HasKey:       key != "",
			Models:       models,
			ModelCount:   len(models),
		})
	}
	return out
}

// maskKey 把密钥变成可辨认但不可复原的掩码。
func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "••••"
	}
	return key[:4] + "••••" + key[len(key)-4:]
}

// modelEntryFromWire 把 models.json 里的一条模型转成界面结构。
func modelEntryFromWire(raw map[string]any) domain.ProviderModelEntryVO {
	entry := domain.ProviderModelEntryVO{ModelId: stringField(raw, "id")}
	entry.ModelName = stringField(raw, "name")
	entry.ApiProtocol = stringField(raw, "api")
	entry.InputKinds = asStringSlice(raw["input"])
	if flag, ok := raw["reasoning"].(bool); ok {
		entry.SupportsReasoning = flag
	}
	if n, ok := raw["contextWindow"].(float64); ok {
		entry.ContextWindow = int(n)
	}
	if n, ok := raw["maxTokens"].(float64); ok {
		entry.MaxTokens = int(n)
	}
	return entry
}

// Upsert 保存一个服务商：写 models.json，密钥非空时写 auth.json。
// ApiKey 空串语义是「保留原值」——界面拿到的是掩码，回传的必然不是原文。
func (m *Models) Upsert(req domain.ProviderUpsertREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	if name == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商名不能为空")
	}
	if strings.TrimSpace(req.BaseUrl) == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "接口地址不能为空")
	}
	config, _ := m.readModels()
	models := make([]map[string]any, 0, len(req.Models))
	for _, entry := range req.Models {
		models = append(models, modelEntryToWire(entry))
	}
	config.Providers[name] = providerFile{
		BaseUrl:     strings.TrimSpace(req.BaseUrl),
		ApiProtocol: defaultIfEmpty(strings.TrimSpace(req.ApiProtocol), "openai"),
		AuthKind:    defaultIfEmpty(strings.TrimSpace(req.AuthKind), "api_key"),
		Models:      models,
	}
	if err := pkg.WriteJSON(m.Layout.ModelsFile(), config); err != nil {
		return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "models.json 写入失败", err)
	}
	if key := strings.TrimSpace(req.ApiKey); key != "" {
		auth := m.readAuth()
		auth[name] = map[string]any{"type": "api_key", "key": key}
		if err := pkg.WriteJSON(m.Layout.AuthFile(), auth); err != nil {
			return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "auth.json 写入失败", err)
		}
	}
	return m.Providers(), nil
}

// modelEntryToWire 把界面结构转成 models.json 的条目。
func modelEntryToWire(entry domain.ProviderModelEntryVO) map[string]any {
	out := map[string]any{"id": entry.ModelId}
	if entry.ModelName != "" {
		out["name"] = entry.ModelName
	}
	if entry.ApiProtocol != "" {
		out["api"] = entry.ApiProtocol
	}
	if entry.SupportsReasoning {
		out["reasoning"] = true
	}
	if entry.ContextWindow > 0 {
		out["contextWindow"] = entry.ContextWindow
	}
	if entry.MaxTokens > 0 {
		out["maxTokens"] = entry.MaxTokens
	}
	if len(entry.InputKinds) > 0 {
		out["input"] = toAnySlice(entry.InputKinds)
	}
	return out
}

// SaveModel 单条模型增改：按 ModelId 定位，存在即改，不存在即增。
// 服务商必须已存在——模型挂在服务商下面，不能凭空造出半个服务商。
func (m *Models) SaveModel(req domain.ProviderModelSaveREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	config, _ := m.readModels()
	provider, ok := config.Providers[name]
	if !ok {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商不存在："+name)
	}
	entry := req.Model
	if strings.TrimSpace(entry.ModelId) == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "模型 ID 不能为空")
	}
	wire := modelEntryToWire(entry)
	replaced := false
	for i := range provider.Models {
		if stringField(provider.Models[i], "id") == entry.ModelId {
			provider.Models[i] = wire
			replaced = true
			break
		}
	}
	if !replaced {
		provider.Models = append(provider.Models, wire)
	}
	config.Providers[name] = provider
	if err := pkg.WriteJSON(m.Layout.ModelsFile(), config); err != nil {
		return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "models.json 写入失败", err)
	}
	return m.Providers(), nil
}

// DeleteModel 删除服务商下的一条模型。
func (m *Models) DeleteModel(req domain.ProviderModelDeleteREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	config, _ := m.readModels()
	provider, ok := config.Providers[name]
	if !ok {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商不存在："+name)
	}
	kept := make([]map[string]any, 0, len(provider.Models))
	removed := false
	for _, item := range provider.Models {
		if stringField(item, "id") == strings.TrimSpace(req.ModelId) {
			removed = true
			continue
		}
		kept = append(kept, item)
	}
	if !removed {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeNotFound, "未找到模型："+req.ModelId)
	}
	provider.Models = kept
	config.Providers[name] = provider
	if err := pkg.WriteJSON(m.Layout.ModelsFile(), config); err != nil {
		return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "models.json 写入失败", err)
	}
	return m.Providers(), nil
}

// Delete 删除服务商：models.json 与 auth.json 一起清。
func (m *Models) Delete(name string) (domain.ProvidersRESP, error) {
	config, _ := m.readModels()
	delete(config.Providers, name)
	if err := pkg.WriteJSON(m.Layout.ModelsFile(), config); err != nil {
		return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "models.json 写入失败", err)
	}
	auth := m.readAuth()
	if _, ok := auth[name]; ok {
		delete(auth, name)
		if err := pkg.WriteJSON(m.Layout.AuthFile(), auth); err != nil {
			return domain.ProvidersRESP{}, pkg.WrapErr(pkg.CodeConfigWrite, "auth.json 写入失败", err)
		}
	}
	return m.Providers(), nil
}

// resolveProbe 把探测入参补齐：没给的项从已保存的配置里取。
func (m *Models) resolveProbe(req domain.ProviderProbeREQ) (baseUrl, apiProtocol, apiKey string) {
	config, _ := m.readModels()
	baseUrl = strings.TrimSpace(req.BaseUrl)
	apiProtocol = strings.TrimSpace(req.ApiProtocol)
	apiKey = strings.TrimSpace(req.ApiKey)
	if entry, ok := config.Providers[req.ProviderName]; ok {
		if baseUrl == "" {
			baseUrl = entry.BaseUrl
		}
		if apiProtocol == "" {
			apiProtocol = entry.ApiProtocol
		}
	}
	if apiKey == "" {
		if item, ok := m.readAuth()[req.ProviderName]; ok {
			if value, ok := item["key"].(string); ok {
				apiKey = value
			}
		}
	}
	return baseUrl, defaultIfEmpty(apiProtocol, "openai"), apiKey
}
