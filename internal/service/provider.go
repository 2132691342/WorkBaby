package service

import (
	"context"
	"slices"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// ProviderService LLM Provider 业务编排。
type ProviderService struct {
	r      *repo.AiProviderRepo
	cipher *pkg.Cipher
}

func NewProviderService(r *repo.AiProviderRepo, cipher *pkg.Cipher) *ProviderService {
	return &ProviderService{r: r, cipher: cipher}
}

func maskAPIKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if len([]byte(k)) <= 8 {
		return "****"
	}
	return k[:4] + "****" + k[len([]byte(k))-4:]
}

// encryptAPIKey 用于 Create/Update 写入前加密；cipher 为空时透传空字符串。
func (s *ProviderService) encryptAPIKey(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if s.cipher == nil {
		return plain, nil // 极端兜底：未初始化（不应发生，handler 已注）
	}
	return s.cipher.Encrypt(plain)
}

// decryptAPIKey 用于内部 Build Registry 时把已加密的 apiKey 还原出来。
func (s *ProviderService) decryptAPIKey(ct string) (string, error) {
	if ct == "" {
		return "", nil
	}
	if s.cipher == nil {
		// 兼容：未启用 cipher 时（如手工 log dump）明文回退
		return ct, nil
	}
	return s.cipher.Decrypt(ct)
}

// DecryptAPIKey 解密 API Key；导出给 LLM Registry 构造 Provider 时用。
func (s *ProviderService) DecryptAPIKey(ct string) (string, error) { return s.decryptAPIKey(ct) }

func (s *ProviderService) toRESP(p *domain.AiProviderDO) domain.AiProviderRESP {
	return domain.AiProviderRESP{
		ID:              p.ID,
		Name:            p.Name,
		Kind:            p.Kind,
		APIKeyMasked:    maskAPIKey(decryptForMask(p.APIKey, s.cipher)),
		BaseURL:         p.BaseURL,
		Model:           p.Model,
		Alias:           p.Alias,
		Tier:            domain.NormalizeTier(p.Tier),
		Enabled:         p.Enabled,
		ContextWindow:   p.ContextWindow,
		MaxOutputTokens: p.MaxOutputTokens,
		CompressRatio:   p.CompressRatio,
		Temperature:     p.Temperature,
		TopP:            p.TopP,
		ThinkingEffort:  p.ThinkingEffort,
		ThinkingStyle:   p.ThinkingStyle,
		// 解析后的方言：自动探测的结果要让用户看得见，否则「为什么没开思考」无从排查。
		ThinkingStyleResolved: string(llm.ResolveThinkingStyle(p.ThinkingStyle, p.BaseURL, p.Model)),
		SupportsToolCall:      p.SupportsToolCall,
		SupportsVision:        p.SupportsVision,
		SupportsReasoning:     p.SupportsReasoning,
		ToolCallEffective:     p.SupportsToolCallEffective(),
		VisionEffective:       p.SupportsVisionEffective(),
		ReasoningEffective:    p.SupportsReasoningEffective(),
		CapabilitiesJSON:      p.CapabilitiesJSON,
		PricingJSON:           p.PricingJSON,
		CreatedAt:             p.CreatedAt,
		UpdatedAt:             p.UpdatedAt,
	}
}

// decryptForMask 仅用于 RESP 的 apiKeyMasked 字段；解密失败回退原始字符串再 mask。
func decryptForMask(ct string, c *pkg.Cipher) string {
	if c == nil || ct == "" {
		return ct
	}
	pt, err := c.Decrypt(ct)
	if err != nil {
		return ct
	}
	return pt
}

func validateREQ(req *domain.AiProviderREQ) error {
	if req.Name == "" {
		return pkg.New(3010, "provider name is required", "")
	}
	if req.Kind == "" {
		req.Kind = domain.ProviderKindOpenAI
	}
	// 防御性校验：拒绝前端传入后端不支持的 kind（CLAUDE.md §8 反模式「臆造 API 特性」）。
	// kind 在 internal/domain/ai_provider.go AllProviderKinds 定义；registry.buildOne 失败会让 Provider 永久 unready。
	if !slices.Contains(domain.AllProviderKinds, req.Kind) {
		return pkg.New(3012, "unsupported provider kind", string(req.Kind))
	}
	if req.Model == "" {
		return pkg.New(3011, "provider model is required", "")
	}
	return nil
}

// Create 新建 Provider：APIKey 入库前加密。
func (s *ProviderService) Create(ctx context.Context, req *domain.AiProviderREQ) (*domain.AiProviderRESP, error) {
	if err := validateREQ(req); err != nil {
		return nil, err
	}
	ct, err := s.encryptAPIKey(req.APIKey)
	if err != nil {
		return nil, pkg.Wrap(2028, "encrypt api key failed", err)
	}
	p := &domain.AiProviderDO{
		ID:                pkg.NewID(domain.IDProvider),
		Name:              req.Name,
		Kind:              req.Kind,
		APIKey:            ct,
		BaseURL:           req.BaseURL,
		Model:             req.Model,
		Alias:             req.Alias,
		Tier:              domain.NormalizeTier(req.Tier),
		Enabled:           req.Enabled == nil || *req.Enabled,
		ContextWindow:     req.ContextWindow,
		MaxOutputTokens:   req.MaxOutputTokens,
		CompressRatio:     orDefault(req.CompressRatio, defaultCompressionRatio),
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		ThinkingEffort:    req.ThinkingEffort,
		ThinkingStyle:     string(llm.ParseThinkingStyle(req.ThinkingStyle)),
		SupportsToolCall:  req.SupportsToolCall,
		SupportsVision:    req.SupportsVision,
		SupportsReasoning: req.SupportsReasoning,
		CapabilitiesJSON:  req.CapabilitiesJSON,
		PricingJSON:       req.PricingJSON,
	}
	if err := s.r.Create(ctx, p); err != nil {
		return nil, err
	}
	r := s.toRESP(p)
	return &r, nil
}

// Update 更新 Provider：APIKey 若非空则重新加密后覆盖；空则保留原值。
func (s *ProviderService) Update(ctx context.Context, id string, req *domain.AiProviderREQ) (*domain.AiProviderRESP, error) {
	p, err := s.r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.Kind != "" {
		p.Kind = req.Kind
	}
	if req.APIKey != "" {
		ct, eerr := s.encryptAPIKey(req.APIKey)
		if eerr != nil {
			return nil, pkg.Wrap(2028, "encrypt api key failed", eerr)
		}
		p.APIKey = ct
	}
	p.BaseURL = req.BaseURL
	if req.Model != "" {
		p.Model = req.Model
	}
	p.Alias = req.Alias
	if req.Tier != "" {
		p.Tier = domain.NormalizeTier(req.Tier)
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.ContextWindow > 0 {
		p.ContextWindow = req.ContextWindow
	}
	if req.CompressRatio > 0 {
		p.CompressRatio = req.CompressRatio
	}
	p.Temperature = req.Temperature
	p.TopP = req.TopP
	p.ThinkingEffort = req.ThinkingEffort
	// 思维方言与能力三态允许显式清空回落到自动判定，故不做非零判断
	p.ThinkingStyle = string(llm.ParseThinkingStyle(req.ThinkingStyle))
	p.SupportsToolCall = req.SupportsToolCall
	p.SupportsVision = req.SupportsVision
	p.SupportsReasoning = req.SupportsReasoning
	if req.CapabilitiesJSON != "" {
		p.CapabilitiesJSON = req.CapabilitiesJSON
	}
	if req.PricingJSON != "" {
		p.PricingJSON = req.PricingJSON
	}
	if err := s.r.Update(ctx, p); err != nil {
		return nil, err
	}
	r := s.toRESP(p)
	return &r, nil
}

func (s *ProviderService) Delete(ctx context.Context, id string) error {
	return s.r.Delete(ctx, id)
}

func (s *ProviderService) Get(ctx context.Context, id string) (*domain.AiProviderRESP, error) {
	p, err := s.r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	r := s.toRESP(p)
	return &r, nil
}

func (s *ProviderService) List(ctx context.Context) ([]domain.AiProviderRESP, error) {
	ps, err := s.r.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AiProviderRESP, 0, len(ps))
	for i := range ps {
		out = append(out, s.toRESP(&ps[i]))
	}
	return out, nil
}

// ListAvailable 聊天模型选择器（只列 enabled）。
func (s *ProviderService) ListAvailable(ctx context.Context) ([]domain.AvailableModelRESP, error) {
	ps, err := s.r.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AvailableModelRESP, 0, len(ps))
	for _, p := range ps {
		out = append(out, domain.AvailableModelRESP{
			ID:              p.ID,
			Name:            p.Name,
			Kind:            p.Kind,
			Model:           p.Model,
			Alias:           p.Alias,
			Tier:            p.Tier,
			Enabled:         p.Enabled,
			ContextWindow:   p.ContextWindow,
			MaxOutputTokens: p.MaxOutputTokens,
			CompressRatio:   p.CompressRatio,
			Temperature:     p.Temperature,
			ThinkingEffort:  p.ThinkingEffort,
			ThinkingStyle:   p.ThinkingStyle,
			ToolCall:        p.SupportsToolCallEffective(),
			Vision:          p.SupportsVisionEffective(),
			Reasoning:       p.SupportsReasoningEffective(),
		})
	}
	return out, nil
}

// TestConnect 占位：当前只校验字段完整性。
func (s *ProviderService) TestConnect(ctx context.Context, id string) error {
	p, err := s.r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.BaseURL == "" || p.Model == "" {
		return domain.ErrProviderInvalid
	}
	if p.Kind != domain.ProviderKindOllama && p.APIKey == "" {
		return domain.ErrProviderInvalid
	}
	return nil
}

func orDefault(v, d float64) float64 {
	if v <= 0 {
		return d
	}
	return v
}
