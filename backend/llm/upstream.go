package llm

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// maxRetryAfter 是能接受的服务端等待上限。对面要求等更久说明它短时间恢复不了，
// 让用户看见「上游正在限流」比让窗口空转一分钟更有用。
const maxRetryAfter = 60 * time.Second

// StatusError 由适配器从 HTTP 响应剥出：真实状态码，外加服务端建议的等待时长。
// 重试与等待本来就是按状态码分流的，认状态码比认零散的错误文本可靠得多。
type StatusError struct {
	Status     int
	RetryAfter time.Duration
	Body       string
}

func (e *StatusError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("HTTP %d（上游要求等待 %s）%s", e.Status, e.RetryAfter.Round(time.Second), e.Body)
	}
	return fmt.Sprintf("HTTP %d %s", e.Status, e.Body)
}

// MapStatus 把 HTTP 状态码映射到 AppError 码段 3103~3107。
// 三个适配器共用一份：写三遍的结果就是某一天只有两份在跟随改动。
func MapStatus(code int) int {
	switch {
	case code == 401 || code == 403:
		return 3104
	case code == 404:
		return 3105
	case code == 429:
		return 3106
	case code >= 500:
		return 3107
	default:
		return 3103
	}
}

// ParseRetryAfter 读服务端的等待指示：毫秒头优先，秒头兜底，也接受 HTTP-date。
// 拿不到返回 0，交由本地退避。上限由 maxRetryAfter 兜住。
func ParseRetryAfter(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	if ms := h.Get("Retry-After-Ms"); ms != "" {
		if n, err := strconv.Atoi(ms); err == nil && n > 0 {
			return capped(time.Duration(n) * time.Millisecond)
		}
	}
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return capped(time.Duration(n) * time.Second)
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return capped(d)
		}
	}
	return 0
}

func capped(d time.Duration) time.Duration {
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}
