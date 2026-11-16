// 上游失败的有限重试：只重发「还没产出任何内容」的失败，退避可被用户中止打断。
package agent

import (
	"context"
	"strings"
	"time"
)

// 重试参数：三次退避（2s/4s/8s），只救瞬时故障；额度与鉴权类错误重发只会白烧。
const (
	maxUpstreamRetries = 3
	retryBaseDelay     = 2 * time.Second
)

// retryableMarkers 是可重试失败的原话特征：限流 / 过载 / 网关 / 连接类。
// 匹配的是已经归一成用户话术的错误串，所以中英文都列。
var retryableMarkers = []string{
	"429", "500", "502", "503", "504",
	"rate limit", "ratelimit", "too many requests", "overloaded", "timeout", "timed out",
	"connection reset", "connection refused", "eof", "流中断", "限流", "超时", "过载",
	"temporarily", "try again", "稍后",
}

// nonRetryableMarkers 是一票否决的特征：重发改变不了结果。
var nonRetryableMarkers = []string{
	"401", "403", "404", "400", "invalid", "unauthorized", "forbidden",
	"quota", "billing", "insufficient", "余额", "额度", "密钥", "不存在",
}

// retryableUpstreamError 判断一次失败值不值得重发。
func retryableUpstreamError(message string) bool {
	lowered := strings.ToLower(message)
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(lowered, marker) {
			return false
		}
	}
	for _, marker := range retryableMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// streamWithRetry 发起模型调用；建流失败且可重试时退避重发。
// 已产出分片的失败不在这层重发：半截内容已经给用户看过了。
func (r *Runner) streamWithRetry(ctx context.Context, h Handler) (Message, error) {
	for attempt := 0; ; attempt++ {
		message, err := r.streamOnce(ctx, h)
		if err == nil {
			return message, nil
		}
		if ctx.Err() != nil || attempt >= maxUpstreamRetries || !retryableUpstreamError(err.Error()) {
			return Message{}, err
		}
		if !r.waitRetry(ctx, h, attempt+1) {
			return Message{}, err
		}
	}
}

// retryUpstreamError 流内失败（错误结束事件）的同一套退避；返回 true 表示该重发这一步。
func (r *Runner) retryUpstreamError(ctx context.Context, h Handler, message string) bool {
	if ctx.Err() != nil || !retryableUpstreamError(message) {
		return false
	}
	r.mu.Lock()
	attempt := r.retries
	r.retries++
	r.mu.Unlock()
	if attempt >= maxUpstreamRetries {
		return false
	}
	return r.waitRetry(ctx, h, attempt+1)
}

// waitRetry 退避等待并广播进度；用户中止立刻返回 false。
func (r *Runner) waitRetry(ctx context.Context, h Handler, attempt int) bool {
	base := retryBaseDelay
	if r.RetryBaseDelay > 0 {
		base = r.RetryBaseDelay
	}
	delay := base << (attempt - 1)
	if h.OnRetry != nil {
		h.OnRetry(attempt, maxUpstreamRetries, delay.Milliseconds())
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	if h.OnRetryEnd != nil {
		h.OnRetryEnd()
	}
	return ctx.Err() == nil
}
