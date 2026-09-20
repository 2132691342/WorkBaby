package llm

// 跨 Provider 归一化契约：错误分类 / 重试退避。

import (
	"context"
	"errors"
	"testing"
	"time"

	"WorkBaby/internal/pkg"

	"github.com/stretchr/testify/assert"
)

// TestErrorClassConformance 错误分类：决定重试/降级/提示的唯一直相源，三家 Provider 共用。
func TestErrorClassConformance(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{"rate_limit", pkg.New(3003, "429 rate limited", ""), ClassTransient},
		{"upstream_5xx", pkg.New(3004, "502 bad gateway", ""), ClassTransient},
		{"timeout", pkg.New(3005, "upstream timeout", ""), ClassTransient},
		{"invalid_key", pkg.New(3001, "401 unauthorized", ""), ClassAuth},
		{"context_too_long", pkg.New(3008, "context length exceeded", ""), ClassContext},
		{"bad_request", pkg.New(3007, "400 invalid message", ""), ClassRequest},
		{"cancelled", context.Canceled, ClassCancelled},
		// 内层段位码被 Wrap 字符串化进 Details 后仍要能分类（provider 层不保留 cause 链）
		{"inner_marker", pkg.Wrap(3099, "stream failed", pkg.New(3003, "429", "")), ClassTransient},
		{"unknown", errors.New("boom"), ClassUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, ClassifyError(c.err))
			assert.Equal(t, c.want == ClassTransient, IsTransient(c.err), "IsTransient 必须与 ClassTransient 一致")
		})
	}
}

// TestRetryPolicyConformance 退避：尊重服务端 Retry-After，且任何 attempt 都不越界。
func TestRetryPolicyConformance(t *testing.T) {
	p := DefaultRetryPolicy()
	// Retry-After 优先于指数退避：上游说等多久就等多久
	assert.Equal(t, 3*time.Second, p.Backoff(0, 3*time.Second))
	for attempt := 0; attempt < 6; attempt++ {
		d := p.Backoff(attempt, 0)
		assert.GreaterOrEqual(t, d, p.BaseDelay, "attempt %d 退避不得短于基准", attempt)
		assert.LessOrEqual(t, d, p.MaxDelay, "attempt %d 退避不得超封顶", attempt)
	}
	// 提取：结构化挂载的合法值取到，越界（>120s）与非挂载错误一律忽略
	assert.Equal(t, 5*time.Second, RetryAfter(WithRetryAfter(pkg.New(3003, "429", ""), 5*time.Second)))
	assert.Zero(t, RetryAfter(WithRetryAfter(pkg.New(3003, "429", ""), 600*time.Second)))
	assert.Zero(t, RetryAfter(errors.New("boom")))
}
