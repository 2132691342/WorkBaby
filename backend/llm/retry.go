package llm

import (
	"context"
	"errors"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"WorkBaby/backend/pkg"
)

// 不可重试的错误特征：配额与鉴权类，重试只会让等待更久。
// context 超限也在这里：原地重试必然同样失败，压缩后重试由 agent 层负责。
var fatalPatterns = []string{
	"insufficient_quota", "out of budget", "quota exceeded", "billing",
	"invalid api key", "unauthorized", "permission denied", "forbidden",
	"context length", "request too large", "model_not_found",
}

// overflowPatterns 是各家「上下文超限」的文案特征。命中不代表对话完了——
// 本地 token 估算是粗估，偏乐观时就会走到这；agent 层据此压缩后重试。
var overflowPatterns = []string{
	"context length", "context_length_exceeded", "context window", "context_window",
	"maximum context", "max context", "too many tokens", "reduce the length",
	"prompt is too long", "prompt too long", "input is too long", "request too large",
	"exceeds the maximum", "maximum number of tokens", "exceed context",
}

// IsContextOverflow 判定错误是否是「上下文超限」。
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range overflowPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// outputPatterns 是各家「输出预算被拒」的文案特征：max_tokens / max_completion_tokens
// 超过该模型真实上限。窗口的 1/8 是本地算出来的经验值，只有上游知道真实上限，
// agent 层据此把预算减半重试一次。
var outputPatterns = []string{
	"max_tokens", "max_completion_tokens", "max output", "maximum output",
	"output token", "maxoutputtokens", "max_new_tokens",
}

// IsOutputLimit 判定错误是否是「输出预算超过上游上限」。
// 上下文超限的文案里也常出现 tokens / maximum，先排除它，避免压错了方向。
func IsOutputLimit(err error) bool {
	if err == nil || IsContextOverflow(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range outputPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// limitNumberRe 用来从错误文本里取上游声明的数字（输出上限、合法区间）。
var limitNumberRe = regexp.MustCompile(`\d+`)

// OutputLimitFrom 给出「输出预算被拒」之后的降级值：报错里写着上限就用它，没写就对折。
// 各家原话不同（`[1, 32768]` / `128000 > 32768` 都出现过），取比自己小的最大数字即可：
// 取大一点只是再被拒一次（调用方有次数上限），取小一点只是回答短些，都比整轮失败轻。
func OutputLimitFrom(err error, current int) int {
	if current <= 0 {
		return 0
	}
	if err != nil {
		if n := limitBelow(err.Error(), current); n > 0 {
			return n
		}
	}
	return current / 2
}

// limitBelow 扫描文本里的整数，返回小于 limit 的最大值；没有则返回 0。
func limitBelow(msg string, limit int) int {
	best := 0
	for _, raw := range limitNumberRe.FindAllString(msg, 8) {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= best || n >= limit {
			continue
		}
		best = n
	}
	return best
}

// 可重试的错误特征：限流、过载与传输中断。只有拿不到状态码时才用得上这些。
var retryPatterns = []string{
	"rate limit", "rate_limit", "429", "overloaded", "timeout",
	"connection reset", "connection refused", "socket hang up", "eai_again",
	"getaddrinfo", "other side closed", "resource exhausted", "520", "524",
	"temporarily unavailable", "try again", "stream ended",
	"unexpected eof", "broken pipe", "tls handshake", "no such host",
}

// StatusRetryable 按 HTTP 状态码分流：408/409/429 与 5xx 值得重试，
// 4xx 里除去这三类都属于「请求本身有问题」，重试一次也不会变对。
func StatusRetryable(code int) bool {
	switch code {
	case 408, 409, 429:
		return true
	case 400, 401, 403, 404, 413, 422:
		return false
	}
	return code >= 500
}

// Retryable 判定一个上游错误是否值得重试。
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 状态码优先：适配器拿到了就用它，拿不到（比如连接层直接报错）才回落到文本。
	var se *StatusError
	if errors.As(err, &se) {
		return StatusRetryable(se.Status)
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

// waitFor 取等待时长：服务端给了 Retry-After 就照它等，否则本地指数退避。
func waitFor(err error, base time.Duration, attempt int) time.Duration {
	var se *StatusError
	if err != nil && errors.As(err, &se) && se.RetryAfter > 0 {
		return se.RetryAfter
	}
	return backoff(base, attempt)
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
				if attempt < maxRetry && Retryable(err) && sleep(ctx, waitFor(err, base, attempt)) {
					continue
				}
				select {
				case out <- Event{Type: EventError, StopReason: StopError, Err: err}:
				case <-ctx.Done():
				}
				return
			}
			produced := false
			var failed error
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
					failed = ev.Err
				}
			}
			if failed == nil {
				return
			}
			if produced || attempt >= maxRetry {
				return
			}
			if !sleep(ctx, waitFor(failed, base, attempt)) {
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
