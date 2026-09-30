package llm

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
)

// 不可重试的错误特征：配额与鉴权类，重试只会让等待更久。
var fatalPatterns = []string{
	"insufficient_quota", "out of budget", "quota exceeded", "billing",
	"invalid api key", "unauthorized", "permission denied",
}

// 可重试的错误特征：限流、过载与传输中断。
var retryPatterns = []string{
	"rate limit", "rate_limit", "429", "overloaded", "timeout",
	"connection reset", "connection refused", "socket hang up",
	"temporarily unavailable", "try again", "stream ended",
}

// Retryable 判定一个上游错误是否值得重试。
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range fatalPatterns {
		if strings.Contains(msg, p) {
			return false
		}
	}
	for _, p := range retryPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// RetryingStreamer 在流「尚未产出任何内容就失败」时重试，已经出过内容就不再重试——
// 否则用户会看到重复的前半段正文。
type RetryingStreamer struct {
	Inner     Streamer
	MaxRetry  int
	BaseDelay time.Duration
}

// Stream 重试直到成功或耗尽次数；ctx 取消立即放弃。
func (r RetryingStreamer) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	if r.Inner == nil {
		return nil, pkg.New(3002, "未配置模型服务", "")
	}
	maxRetry := r.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 2
	}
	base := r.BaseDelay
	if base <= 0 {
		base = 500 * time.Millisecond
	}

	out := make(chan Event, 64)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		defer close(out)
		for attempt := 0; ; attempt++ {
			src, err := r.Inner.Stream(ctx, req)
			if err != nil {
				if attempt < maxRetry && Retryable(err) && sleep(ctx, backoff(base, attempt)) {
					continue
				}
				select {
				case out <- Event{Type: EventError, StopReason: StopError, Err: err}:
				case <-ctx.Done():
				}
				return
			}
			produced := false
			failed := false
			for ev := range src {
				if ev.Type == EventDelta || ev.Type == EventThinking || ev.Type == EventToolCall {
					produced = true
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
				if ev.Type == EventError {
					failed = true
				}
			}
			if !failed {
				return
			}
			if produced || attempt >= maxRetry {
				return
			}
			if !sleep(ctx, backoff(base, attempt)) {
				return
			}
		}
	}()
	return out, nil
}

// backoff 指数退避，带 ≤25% 抖动，上限 8 秒。
func backoff(base time.Duration, attempt int) time.Duration {
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	jitter := 1.0 - rand.Float64()*0.25
	return time.Duration(float64(d) * jitter)
}

// sleep 可中断等待；被取消时返回 false。
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
