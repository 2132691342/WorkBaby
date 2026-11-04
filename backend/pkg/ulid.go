package pkg

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// 现役 ID 前缀白名单。加一个前缀等于加一类可从字符串认出宿主对象的标识，
// 所以它必须集中在这里，而不是散在各处的字符串拼接。
const (
	PrefixRun     = "RUN" // 一次对话运行
	PrefixAsk     = "ASK" // 一次结构化提问
	PrefixEvent   = "EVT" // 一条 SSE 帧
	PrefixJob     = "JOB" // 一次后台任务
	PrefixSession = "SES" // 一次壳内会话登记
)

var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

// NewID 生成 `{PREFIX}_{26位ULID}`；同毫秒内多次调用保持字典序递增。
func NewID(prefix string) string {
	entropyMu.Lock()
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	entropyMu.Unlock()
	if err != nil {
		// 熵源不可用时退化成时间戳：宁可 ID 弱一点，也不能让业务路径炸掉。
		return prefix + "_" + ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	}
	return prefix + "_" + id.String()
}
