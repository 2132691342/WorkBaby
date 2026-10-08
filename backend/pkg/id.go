package pkg

import (
	"crypto/rand"
	"sync"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// entropy 用单调源，保证同毫秒内生成的 ULID 仍按序。
var entropy = ulid.Monotonic(rand.Reader, 0)

// entropyMu 串行化熵源。Monotonic 不是并发安全的：并行工具 / 并行请求下
// 竞争内部状态会生成重复 ULID，落库时撞主键，整条消息直接丢。
var entropyMu sync.Mutex

// NewID 生成「前缀_ULID」形式的业务主键；前缀为空时只返回 ULID。
func NewID(prefix string) string {
	entropyMu.Lock()
	id := ulid.MustNew(ulid.Now(), entropy).String()
	entropyMu.Unlock()
	if prefix == "" {
		return id
	}
	return prefix + "_" + id
}

// NewTraceID 生成一次调用链用的 UUID v4。
func NewTraceID() string { return uuid.NewString() }
