package pkg

import (
	"crypto/rand"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// entropy 用单调源，保证同毫秒内生成的 ULID 仍按序。
var entropy = ulid.Monotonic(rand.Reader, 0)

// NewID 生成「前缀_ULID」形式的业务主键；前缀为空时只返回 ULID。
func NewID(prefix string) string {
	id := ulid.MustNew(ulid.Now(), entropy).String()
	if prefix == "" {
		return id
	}
	return prefix + "_" + id
}

// NewTraceID 生成一次调用链用的 UUID v4。
func NewTraceID() string { return uuid.NewString() }
