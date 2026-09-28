// Package factory 按配置构造上游适配器。独立成包是为了避免适配器反向依赖 llm 形成环。
package factory

import (
	"sync"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/anthropic"
	"WorkBaby/internal/llm/ollama"
	"WorkBaby/internal/llm/openai"
	"WorkBaby/internal/pkg"
)

var (
	mu        sync.Mutex
	overrides = map[string]func(llm.ClientConfig) llm.Streamer{}
)

// SetOverride 测试注入口：按 api 名替换实现，让内核集成测试不联网。
func SetOverride(api string, f func(llm.ClientConfig) llm.Streamer) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		delete(overrides, api)
		return
	}
	overrides[api] = f
}

// HasOverride 报告某个 api 名是否已被测试注解替换。
func HasOverride(api string) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := overrides[api]
	return ok
}

// NewStreamer 按 api 类型构造适配器并套上重试。Provider 只是配置行，新增一家服务通常不需要新适配器。
func NewStreamer(cfg llm.ClientConfig) (llm.Streamer, error) {
	mu.Lock()
	f, ok := overrides[cfg.API]
	mu.Unlock()
	var inner llm.Streamer
	if ok {
		inner = f(cfg)
	} else {
		switch cfg.API {
		case "openai":
			inner = openai.New(cfg)
		case "anthropic":
			inner = anthropic.New(cfg)
		case "ollama":
			inner = ollama.New(cfg)
		default:
			return nil, pkg.New(3108, "不支持的模型服务类型", cfg.API)
		}
	}
	return llm.RetryingStreamer{Inner: inner, MaxRetry: 2, BaseDelay: 0}, nil
}
