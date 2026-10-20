// 链路追踪 ID：一轮运行拿一个，日志与事件两边用它对齐同一次调用。
package pkg

import "github.com/google/uuid"

// NewTraceID 生成一次调用链用的 UUID v4。
func NewTraceID() string { return uuid.NewString() }
