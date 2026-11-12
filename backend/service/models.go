package service

import (
	"sort"
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

// readModels 读 models.json；文件不存在等同于「还没配过」，返回空表且不报错。
// 文件存在却解析失败才是真错：那种情况下界面必须说出来，否则用户会以为配置被吞了。
func (m *Models) readModels() (modelsFile, string) {
	out := modelsFile{Providers: map[string]providerFile{}}
	var value modelsFile
	if err := pkg.ReadJSON(m.Layout.ModelsFile(), &value); err != nil {
		if !pkg.Exists(m.Layout.ModelsFile()) {
			return out, ""
		}
		return out, "models.json 解析失败：" + err.Error()
	}
	if value.Providers == nil {
		value.Providers = map[string]providerFile{}
	}
	return value, ""
}

// authFile 是 auth.json 的形状：服务商名 → 凭据。其余键原样保留。
type authMap map[string]map[string]any

// readAuth 读 auth.json；读不出来给空表（凭据缺失由服务商行上的 has_key 表达）。
func (m *Models) readAuth() authMap {
	out := authMap{}
	if pkg.ReadJSON(m.Layout.AuthFile(), &out) != nil || out == nil {
		return authMap{}
	}
	return out
}

// editableModels 读 models.json 并拒掉「文件坏了」的写入：坏文件里可能还有
// 用户手写的服务商，整表覆盖等于把它们一次清空——那是不可恢复的数据丢失。
func (m *Models) editableModels() (modelsFile, error) {
	config, readError := m.readModels()
	if readError != "" {
		return config, pkg.Err(pkg.CodeConfigRead, readError, m.Layout.ModelsFile())
	}
	return config, nil
}

// Providers 组装服务商清单。密钥恒为掩码：界面永远拿不到原文。
func (m *Models) Providers() domain.ProvidersRESP {
	config, readError := m.readModels()
	auth := m.readAuth()
	out := domain.ProvidersRESP{
		Providers:  make([]domain.ProviderVO, 0, len(config.Providers)),
		ConfigPath: m.Layout.ModelsFile(),
		ReadError:  readError,
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

// Options 平铺全部自定义服务商的模型，供对话页模型下拉使用。
// 按服务商名排序、服务商内保持声明顺序，两次刷新的清单顺序才不会跳。
// 读不到配置就回空清单：下拉仍可用，只是只剩「当前模型」一项。
func (m *Models) Options() domain.ModelListRESP {
	config, _ := m.readModels()
	names := make([]string, 0, len(config.Providers))
	for name := range config.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := domain.ModelListRESP{Models: make([]domain.ModelOptionVO, 0), ModelSource: "config"}
	for _, name := range names {
		for _, raw := range config.Providers[name].Models {
			item := modelEntryFromWire(raw)
			out.Models = append(out.Models, domain.ModelOptionVO{
				ProviderName:      name,
				ModelId:           item.ModelId,
				ModelName:         item.ModelName,
				ApiProtocol:       item.ApiProtocol,
				ContextWindow:     item.ContextWindow,
				MaxTokens:         item.MaxTokens,
				SupportsReasoning: item.SupportsReasoning,
				InputKinds:        item.InputKinds,
			})
		}
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
	config, err := m.editableModels()
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
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
	config, err := m.editableModels()
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
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
	config, err := m.editableModels()
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
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
	config, err := m.editableModels()
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
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

// Defaults 读 pi 的默认模型与默认思考档。
// 三个键住在 settings.json 里，与插件声明、引擎自己写的键同处一文件——
// 所以读走 map 而不是结构体，写的时候才好把别人的键原样带回去。
func (m *Models) Defaults() domain.ModelDefaultVO {
	settings := readPiSettings(m.Layout)
	return domain.ModelDefaultVO{
		DefaultProvider:      stringOf(settings["defaultProvider"]),
		DefaultModel:         stringOf(settings["defaultModel"]),
		DefaultThinkingLevel: stringOf(settings["defaultThinkingLevel"]),
		SettingsFile:         m.Layout.SettingsFile(),
	}
}

// SaveDefaults 写默认模型与默认思考档；空串表示把该键删掉（回到引擎默认）。
// 值不合法就整条拒绝：settings.json 里的非法枚举会让引擎启动时校验失败，
// 而那时用户已经在界面上看到「保存成功」了。
func (m *Models) SaveDefaults(req domain.ModelDefaultSaveREQ) (domain.ModelDefaultVO, error) {
	provider := strings.TrimSpace(req.DefaultProvider)
	modelId := strings.TrimSpace(req.DefaultModel)
	level := strings.TrimSpace(req.DefaultThinkingLevel)
	if (provider == "") != (modelId == "") {
		return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "默认服务商与默认模型要一起填或一起留空")
	}
	if level != "" && !isThinkingLevel(level) {
		return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "未知的思考档："+level)
	}
	if provider != "" {
		config, err := m.editableModels()
		if err != nil {
			return domain.ModelDefaultVO{}, err
		}
		providerDef, ok := config.Providers[provider]
		if !ok {
			return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "服务商不存在："+provider)
		}
		if !providerHasModel(providerDef, modelId) {
			return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "该服务商下没有这个模型："+modelId)
		}
	}

	piSettingsMu.Lock()
	defer piSettingsMu.Unlock()
	settings := readPiSettings(m.Layout)
	putOrClear(settings, "defaultProvider", provider)
	putOrClear(settings, "defaultModel", modelId)
	putOrClear(settings, "defaultThinkingLevel", level)
	if err := writePiSettings(m.Layout, settings); err != nil {
		return domain.ModelDefaultVO{}, err
	}
	return m.Defaults(), nil
}

// thinkingLevels 是 pi 认的思考档，与引擎 get_available_thinking_levels 的口径一致。
var thinkingLevels = []string{"off", "minimal", "low", "medium", "high"}

// isThinkingLevel 判断一个档位是否合法。
func isThinkingLevel(level string) bool {
	for _, item := range thinkingLevels {
		if item == level {
			return true
		}
	}
	return false
}

// putOrClear 空串删键，非空写值。留空串与「没设过」在引擎眼里不同：
// 前者会当成一个名叫空的模型去查，结果是启动即失败。
func putOrClear(settings map[string]any, key, value string) {
	if value == "" {
		delete(settings, key)
		return
	}
	settings[key] = value
}

// providerHasModel 判断服务商下是否声明了某个模型。
func providerHasModel(provider providerFile, modelId string) bool {
	for _, item := range provider.Models {
		if stringField(item, "id") == modelId {
			return true
		}
	}
	return false
}
