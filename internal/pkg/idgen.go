// 文件：ID 生成——业务 ULID 带域前缀（约束见 domain/id.go），trace 用 UUID。

package pkg

import (
	"strings"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

// NewID 返回 "<PREFIX>_<ULID>"；前缀必须大写，符合 internal/domain/id.go 的 SCREAMING_SNAKE 常量风格。
// 单调排序：同毫秒内自增熵，保证日志排序与文件落盘顺序一致。
func NewID(prefix string) string {
	u := ulid.Make()
	if prefix == "" {
		return u.String()
	}
	return strings.ToUpper(prefix) + "_" + u.String()
}

// NewTraceID 返回 UUID v4（用于跨边界 trace ID，非业务主键）。
func NewTraceID() string {
	return uuid.NewString()
}
