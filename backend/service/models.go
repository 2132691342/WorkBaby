package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// Models 是模型与服务商编排。真相在 SQLite 的 provider / provider_model 两张表：
// 服务商定义与模型清单要一起增删改，还要能按模型维度查统计——
// 摊成 JSON 文件后每次查询都得整读整解析，放进关系库更省事。
type Models struct {
	// Layout 目录布局
	Layout pkg.Layout
	// repo 服务商仓储
	repo *repo.ProviderRepo
}

// NewModels 造一个模型门面。
func NewModels(layout pkg.Layout, db *repo.DB) *Models {
	return &Models{Layout: layout, repo: repo.NewProviderRepo(db)}
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
	// 请求没带模型时保留原有清单：改地址不该把模型一起清空。
	if len(req.Models) > 0 {
		for index, entry := range req.Models {
			model := modelDOFromVO(name, entry, row.ApiProtocol, index, now)
			if err := m.repo.UpsertModel(model); err != nil {
				return domain.ProvidersRESP{}, err
			}
		}
	}
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
	return m.Providers(), nil
}
func (m *Models) Delete(name string) (domain.ProvidersRESP, error) {
	if err := m.repo.Delete(strings.TrimSpace(name)); err != nil {
		return domain.ProvidersRESP{}, err
	}
	return m.Providers(), nil
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
