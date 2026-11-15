package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/sse"
)

// Models 是模型与服务商编排。真相在 SQLite 的 provider / provider_model 两张表：
// 服务商定义与模型清单要一起增删改，还要能按模型维度查统计——
// 摊成 JSON 文件后每次查询都得整读整解析，放进关系库更省事。
type Models struct {
	// Layout 目录布局
	Layout pkg.Layout
	// repo 服务商仓储
	repo *repo.ProviderRepo
	// Settings 设置（默认服务商 / 默认模型 / 默认思考档住在这里）
	Settings *Settings
	// Hub 事件广播：服务商与模型变了要让界面知道，否则对话页的模型下拉还是旧清单
	Hub *sse.Hub
}

// NewModels 造一个模型门面。
func NewModels(layout pkg.Layout, db *repo.DB, settings *Settings, hub *sse.Hub) *Models {
	return &Models{Layout: layout, repo: repo.NewProviderRepo(db), Settings: settings, Hub: hub}
}

// changed 播一次「模型目录动了」。对话页的模型下拉、模型页自己的清单都听这个
// 刷新——不播的话，用户在模型页加完服务商，输入区的下拉还是加之前那一份。
func (m *Models) changed() {
	if m.Hub == nil {
		return
	}
	m.Hub.Broadcast("models:changed", sse.EngineTag(nil))
}

// Providers 组装服务商清单。密钥恒为掩码：界面永远拿不到原文。
func (m *Models) Providers() domain.ProvidersRESP {
	rows, err := m.repo.Providers()
	if err != nil {
		return domain.ProvidersRESP{
			Providers:  []domain.ProviderVO{},
			ConfigPath: m.Layout.DatabasePath(),
			ReadError:  pkg.As(err).Message,
		}
	}
	out := domain.ProvidersRESP{
		Providers:  make([]domain.ProviderVO, 0, len(rows)),
		ConfigPath: m.Layout.DatabasePath(),
	}
	for _, row := range rows {
		models, _ := m.repo.Models(row.ProviderName)
		entries := make([]domain.ProviderModelEntryVO, 0, len(models))
		for _, item := range models {
			entries = append(entries, modelEntryFromDO(item, row.ApiProtocol))
		}
		out.Providers = append(out.Providers, domain.ProviderVO{
			ProviderName: row.ProviderName,
			BaseUrl:      row.BaseUrl,
			ApiProtocol:  defaultIfEmpty(row.ApiProtocol, "openai"),
			AuthKind:     defaultIfEmpty(row.AuthKind, "api_key"),
			ApiKeyMasked: maskKey(row.ApiKeySecret),
			HasKey:       row.ApiKeySecret != "",
			Models:       entries,
			ModelCount:   len(entries),
		})
	}
	return out
}

// Options 平铺全部模型，供对话页模型下拉使用。
func (m *Models) Options() domain.ModelListRESP {
	providers, _ := m.repo.Providers()
	out := domain.ModelListRESP{Models: make([]domain.ModelOptionVO, 0, 32)}
	for _, provider := range providers {
		models, _ := m.repo.Models(provider.ProviderName)
		for _, item := range models {
			entry := modelEntryFromDO(item, provider.ApiProtocol)
			out.Models = append(out.Models, domain.ModelOptionVO{
				ProviderName:      item.ProviderName,
				ModelId:           item.ModelId,
				ModelName:         entry.ModelName,
				ApiProtocol:       entry.ApiProtocol,
				ContextWindow:     entry.ContextWindow,
				MaxTokens:         entry.MaxTokens,
				SupportsReasoning: entry.SupportsReasoning,
				InputKinds:        entry.InputKinds,
			})
		}
	}
	return out
}

// Lookup 取一个模型的定义与密钥；模型必须存在。
func (m *Models) Lookup(providerName, modelId string) (domain.ProviderModelEntryVO, string, error) {
	provider, ok, err := m.repo.Provider(providerName)
	if err != nil {
		return domain.ProviderModelEntryVO{}, "", err
	}
	if !ok {
		return domain.ProviderModelEntryVO{}, "", pkg.Err(pkg.CodeProviderBad, "服务商不存在："+providerName)
	}
	model, ok, err := m.repo.FindModel(providerName, modelId)
	if err != nil {
		return domain.ProviderModelEntryVO{}, "", err
	}
	if !ok {
		return domain.ProviderModelEntryVO{}, "", pkg.Err(pkg.CodeNotFound, "模型不存在："+modelId)
	}
	return modelEntryFromDO(model, provider.ApiProtocol), provider.ApiKeySecret, nil
}

// Upsert 保存一个服务商：写服务商定义与模型清单。
// ApiKey 空串语义是「保留原值」——界面拿到的是掩码，回传的必然不是原文。
func (m *Models) Upsert(req domain.ProviderUpsertREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	if name == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商名不能为空")
	}
	if strings.TrimSpace(req.BaseUrl) == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "接口地址不能为空")
	}
	existing, found, err := m.repo.Provider(name)
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
	now := pkg.NowMs()
	row := domain.ProviderDO{
		ProviderName: name,
		BaseUrl:      strings.TrimSpace(req.BaseUrl),
		ApiProtocol:  defaultIfEmpty(strings.TrimSpace(req.ApiProtocol), "openai"),
		AuthKind:     defaultIfEmpty(strings.TrimSpace(req.AuthKind), "api_key"),
		ProviderSort: existing.ProviderSort,
		CreatedAtMs:  existing.CreatedAtMs,
		UpdatedAtMs:  now,
	}
	if row.CreatedAtMs == 0 {
		row.CreatedAtMs = now
	}
	// 密钥空串 = 保留原值：界面回传的是掩码，当成新值存会把密钥写坏。
	if key := strings.TrimSpace(req.ApiKey); key != "" {
		row.ApiKeySecret = key
	} else if found {
		row.ApiKeySecret = existing.ApiKeySecret
	}
	if err := m.repo.Upsert(row); err != nil {
		return domain.ProvidersRESP{}, err
	}
	// 请求带了模型清单就按「整表替换」对账：前端弹窗回传的是编辑后的全量清单，
	// 用户删掉一行再保存时，库里旧行不删的话模型下拉永远显示已删的模型。
	// 请求没带清单则保留原有清单：改地址不该把模型一起清空。
	if req.Models != nil {
		keep := make(map[string]bool, len(req.Models))
		for _, entry := range req.Models {
			id := strings.TrimSpace(entry.ModelId)
			if id == "" || keep[id] {
				continue
			}
			keep[id] = true
			model := modelDOFromVO(name, entry, row.ApiProtocol, len(keep)-1, now)
			if err := m.repo.UpsertModel(model); err != nil {
				return domain.ProvidersRESP{}, err
			}
		}
		stored, err := m.repo.Models(name)
		if err != nil {
			return domain.ProvidersRESP{}, err
		}
		for _, old := range stored {
			if keep[old.ModelId] {
				continue
			}
			if _, err := m.repo.DeleteModel(name, old.ModelId); err != nil {
				return domain.ProvidersRESP{}, err
			}
			// 删掉的正是活跃 / 默认指向的那条时把指针清掉：
			// 留着它，下一句话会撞「模型不存在」。
			m.DropActiveIf(name, old.ModelId)
		}
	}
	m.changed()
	return m.Providers(), nil
}

// SaveModel 单条模型增改：按 ModelId 定位，存在即改，不存在即增。
func (m *Models) SaveModel(req domain.ProviderModelSaveREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	provider, found, err := m.repo.Provider(name)
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
	if !found {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商不存在："+name)
	}
	entry := req.Model
	if strings.TrimSpace(entry.ModelId) == "" {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "模型 ID 不能为空")
	}
	existing, found, err := m.repo.FindModel(name, entry.ModelId)
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
	sortIndex := 0
	createdAt := nowOr(existing.CreatedAtMs)
	if found {
		sortIndex = existing.ModelSort
		createdAt = existing.CreatedAtMs
	} else {
		models, _ := m.repo.Models(name)
		sortIndex = len(models)
	}
	model := modelDOFromVO(name, entry, provider.ApiProtocol, sortIndex, createdAt)
	if err := m.repo.UpsertModel(model); err != nil {
		return domain.ProvidersRESP{}, err
	}
	m.changed()
	return m.Providers(), nil
}

// DeleteModel 删除服务商下的一条模型。
func (m *Models) DeleteModel(req domain.ProviderModelDeleteREQ) (domain.ProvidersRESP, error) {
	name := strings.TrimSpace(req.ProviderName)
	if _, found, err := m.repo.Provider(name); err != nil {
		return domain.ProvidersRESP{}, err
	} else if !found {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeProviderBad, "服务商不存在："+name)
	}
	removed, err := m.repo.DeleteModel(name, strings.TrimSpace(req.ModelId))
	if err != nil {
		return domain.ProvidersRESP{}, err
	}
	if !removed {
		return domain.ProvidersRESP{}, pkg.Err(pkg.CodeNotFound, "未找到模型："+req.ModelId)
	}
	// 删的正是当前选中的那条：活跃设置跟着清掉。
	// 不清的后果是下一句话报「模型不存在」，而用户早就不记得删过它。
	m.DropActiveIf(name, req.ModelId)
	m.changed()
	return m.Providers(), nil
}

// Delete 删除一个服务商及其全部模型。
func (m *Models) Delete(name string) (domain.ProvidersRESP, error) {
	name = strings.TrimSpace(name)
	if err := m.repo.Delete(name); err != nil {
		return domain.ProvidersRESP{}, err
	}
	// 删的正是当前选中的服务商：活跃设置一起清，别留一个指向空服务的指针。
	m.DropActiveIf(name, "")
	m.changed()
	return m.Providers(), nil
}

// DropActiveIf 在活跃设置指向被删掉的服务商 / 模型时清掉它们。
// 判定按名字而不是按「还剩几个」：另一个服务商下同样 ID 的模型不该被牵连。
func (m *Models) DropActiveIf(providerName, modelId string) {
	if m.Settings == nil {
		return
	}
	clear := map[string]string{}
	if m.Settings.Get(KeyActiveProvider) == providerName {
		clear[KeyActiveProvider] = ""
		// 给了模型 ID 才按模型判：删整个服务商时，只要服务商对上就全清。
		if modelId == "" || m.Settings.Get(KeyActiveModel) == modelId {
			clear[KeyActiveModel] = ""
		}
	}
	// 默认模型同理：留着它，下一次开新会话仍会落到已不存在的组合上。
	if m.Settings.Get(KeyDefaultProvider) == providerName {
		clear[KeyDefaultProvider] = ""
		if modelId == "" || m.Settings.Get(KeyDefaultModel) == modelId {
			clear[KeyDefaultModel] = ""
		}
	}
	if len(clear) == 0 {
		return
	}
	if err := m.Settings.SaveSettings(clear); err != nil {
		pkg.Warn("清理已删服务商的活跃设置失败：%v", err)
	}
}

// ProviderOf 取一个服务商的定义；不存在给零值。
// 与 Lookup 分开：模型级调用要的是「这个模型长什么样」，
// 而连上游要的是「这个服务商在哪」，两个问题错一层。
func (m *Models) ProviderOf(providerName string) domain.ProviderDO {
	row, found, err := m.repo.Provider(providerName)
	if err != nil || !found {
		return domain.ProviderDO{}
	}
	return row
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

// Defaults 读新会话的起步设置：默认服务商、默认模型与默认思考档。
func (m *Models) Defaults() domain.ModelDefaultVO {
	if m.Settings == nil {
		return domain.ModelDefaultVO{}
	}
	return domain.ModelDefaultVO{
		DefaultProvider:      m.Settings.Get(KeyDefaultProvider),
		DefaultModel:         m.Settings.Get(KeyDefaultModel),
		DefaultThinkingLevel: m.Settings.Get(KeyDefaultThinkingLevel),
		SettingsFile:         m.Settings.Layout.StorePath,
	}
}

// SaveDefaults 写默认模型与默认思考档；空串表示把该键删掉（回到引擎默认）。
// 值不合法就整条拒绝：先写一半再报错，用户会看到一个没保存上的半成品。
func (m *Models) SaveDefaults(req domain.ModelDefaultSaveREQ) (domain.ModelDefaultVO, error) {
	provider := strings.TrimSpace(req.DefaultProvider)
	modelId := strings.TrimSpace(req.DefaultModel)
	level := strings.TrimSpace(req.DefaultThinkingLevel)
	// 服务商与模型要一起填或一起留空：只填一半的话，新会话会卡在“找不到模型”。
	if (provider == "") != (modelId == "") {
		return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "默认服务商与默认模型要一起填或一起留空")
	}
	if level != "" && !isThinkingLevel(level) {
		return domain.ModelDefaultVO{}, pkg.Err(pkg.CodeProviderBad, "未知的思考档："+level)
	}
	if provider != "" {
		if _, _, err := m.Lookup(provider, modelId); err != nil {
			return domain.ModelDefaultVO{}, err
		}
	}
	// 空串就是「清掉该键」：读回来是空串，与没设过等价。
	if err := m.Settings.SaveSettings(map[string]string{
		KeyDefaultProvider:      provider,
		KeyDefaultModel:         modelId,
		KeyDefaultThinkingLevel: level,
	}); err != nil {
		return domain.ModelDefaultVO{}, err
	}
	m.changed()
	return m.Defaults(), nil
}

// isThinkingLevel 判断思考档是否在可用集合内。
func isThinkingLevel(level string) bool { return thinkingLevels[level] }

// resolveProbe 把探测入参补齐：没给的项从已保存的配置里取。
func (m *Models) resolveProbe(req domain.ProviderProbeREQ) (baseUrl, apiProtocol, apiKey string) {
	baseUrl = strings.TrimSpace(req.BaseUrl)
	apiProtocol = strings.TrimSpace(req.ApiProtocol)
	apiKey = strings.TrimSpace(req.ApiKey)
	if provider, found, err := m.repo.Provider(req.ProviderName); err == nil && found {
		if baseUrl == "" {
			baseUrl = provider.BaseUrl
		}
		if apiProtocol == "" {
			apiProtocol = provider.ApiProtocol
		}
		if apiKey == "" {
			apiKey = provider.ApiKeySecret
		}
	}
	return baseUrl, defaultIfEmpty(apiProtocol, "openai"), apiKey
}

// modelEntryFromDO 把持久化行转成界面条目。
func modelEntryFromDO(row domain.ProviderModelDO, providerProtocol string) domain.ProviderModelEntryVO {
	return domain.ProviderModelEntryVO{
		ModelId:           row.ModelId,
		ModelName:         defaultIfEmpty(row.ModelName, row.ModelId),
		ApiProtocol:       defaultIfEmpty(row.ApiProtocol, defaultIfEmpty(providerProtocol, "openai")),
		ContextWindow:     row.ContextWindow,
		MaxTokens:         row.MaxTokens,
		SupportsReasoning: row.SupportsReasoning,
		InputKinds:        splitKinds(row.InputKinds),
	}
}

// modelDOFromVO 把界面条目转成持久化行。
func modelDOFromVO(providerName string, entry domain.ProviderModelEntryVO, providerProtocol string, sortIndex int, createdAt int64) domain.ProviderModelDO {
	return domain.ProviderModelDO{
		ProviderName:      providerName,
		ModelId:           strings.TrimSpace(entry.ModelId),
		ModelName:         strings.TrimSpace(entry.ModelName),
		ApiProtocol:       defaultIfEmpty(strings.TrimSpace(entry.ApiProtocol), defaultIfEmpty(providerProtocol, "openai")),
		ContextWindow:     entry.ContextWindow,
		MaxTokens:         entry.MaxTokens,
		SupportsReasoning: entry.SupportsReasoning,
		InputKinds:        joinKinds(entry.InputKinds),
		ModelSort:         sortIndex,
		CreatedAtMs:       createdAt,
	}
}

// splitKinds 把逗号分隔的输入种类拆回数组。
func splitKinds(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// joinKinds 把输入种类数组拼成逗号分隔的字符串。
func joinKinds(kinds []string) string {
	if len(kinds) == 0 {
		return ""
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if trimmed := strings.TrimSpace(kind); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, ",")
}

// nowOr 取时间戳；为 0 时给当前时刻。
func nowOr(value int64) int64 {
	if value != 0 {
		return value
	}
	return pkg.NowMs()
}
