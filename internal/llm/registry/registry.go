// Package registry 持有 Provider 注册中心。
//
// 边界：作为 llm 的子包，引用 llm 接口与 openai/anthropic/ollama 实现，
// 避免主 llm 包依赖具体实现导致的 import cycle。
package registry

import (
	"context"
	"sync"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/anthropic"
	"WorkBaby/internal/llm/ollama"
	"WorkBaby/internal/llm/openai"
	"WorkBaby/internal/pkg"
)

// Registry Provider 注册中心。
type Registry struct {
	mu        sync.RWMutex
	providers map[string]llm.Provider
	unready   map[string]*pkg.AppError
}

// New 空注册中心；Build 后填充。
func New() *Registry {
	return &Registry{providers: map[string]llm.Provider{}, unready: map[string]*pkg.AppError{}}
}

// Build 从 AiProviderDO 列表构建 Provider；加密后的 apiKey 通过 decAPIKey 还原。
func (r *Registry) Build(ps []domain.AiProviderDO, decAPIKey func(encrypted string) (string, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = map[string]llm.Provider{}
	r.unready = map[string]*pkg.AppError{}
	for _, p := range ps {
		if !p.Enabled {
			continue
		}
		key, err := decAPIKey(p.APIKey)
		if err != nil {
			r.unready[p.ID] = pkg.Wrap(2028, "decrypt api key failed", err)
			continue
		}
		prov, berr := buildOne(p, key)
		if berr != nil {
			r.unready[p.ID] = berr
			continue
		}
		r.providers[p.ID] = prov
	}
	return nil
}

func buildOne(p domain.AiProviderDO, key string) (llm.Provider, *pkg.AppError) {
	switch p.Kind {
	case domain.ProviderKindOpenAI:
		style := llm.ResolveThinkingStyle(p.ThinkingStyle, p.BaseURL, p.Model)
		return openai.New(p.Name, p.BaseURL, key).WithThinkingStyle(style), nil
	case domain.ProviderKindAnthropic:
		return anthropic.New(p.Name, p.BaseURL, key), nil
	case domain.ProviderKindOllama:
		return ollama.New(p.Name, p.BaseURL), nil
	default:
		return nil, pkg.New(3020, "unknown provider kind", string(p.Kind))
	}
}

// Reload 单实例重建。
func (r *Registry) Reload(p domain.AiProviderDO, decAPIKey func(encrypted string) (string, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !p.Enabled {
		delete(r.providers, p.ID)
		delete(r.unready, p.ID)
		return nil
	}
	key, err := decAPIKey(p.APIKey)
	if err != nil {
		r.unready[p.ID] = pkg.Wrap(2028, "decrypt api key failed", err)
		return err
	}
	prov, berr := buildOne(p, key)
	if berr != nil {
		r.unready[p.ID] = berr
		return berr
	}
	delete(r.unready, p.ID)
	r.providers[p.ID] = prov
	return nil
}

// Get 取一个 Provider；未就绪返回 3001。
func (r *Registry) Get(id string) (llm.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.unready[id]; ok {
		return nil, e
	}
	if p, ok := r.providers[id]; ok {
		return p, nil
	}
	return nil, pkg.Wrap(llm.ErrProviderUnready.Code, llm.ErrProviderUnready.Message+": provider not found or not built", llm.ErrProviderUnready)
}

// IsReady 用于 api 层快速判定。
func (r *Registry) IsReady(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[id]
	return ok
}

// Reason 返回 provider 未就绪的原因（ready 或未知 id 返回空串）。
func (r *Registry) Reason(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.providers[id]; ok {
		return ""
	}
	if e, ok := r.unready[id]; ok {
		return e.Error()
	}
	return ""
}

// StatusItem Provider 就绪状态。
type StatusItem struct {
	ProviderID string
	Name       string
	Ready      bool
	Reason     string
}

// Status 列出当前全部 provider 的就绪状态。
func (r *Registry) Status(ps []domain.AiProviderDO) []StatusItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]StatusItem, 0, len(ps))
	for _, p := range ps {
		_, ok := r.providers[p.ID]
		err := r.unready[p.ID]
		var reason string
		if err != nil {
			reason = err.Error()
		}
		out = append(out, StatusItem{ProviderID: p.ID, Name: p.Name, Ready: ok, Reason: reason})
	}
	return out
}

// PingAll 顺序对全部就绪 provider 调 Ping。
func (r *Registry) PingAll(ctx context.Context) map[string]error {
	r.mu.RLock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	out := map[string]error{}
	for _, id := range ids {
		p, err := r.Get(id)
		if err != nil {
			out[id] = err
			continue
		}
		if err := p.Ping(ctx); err != nil {
			out[id] = err
		}
	}
	return out
}
