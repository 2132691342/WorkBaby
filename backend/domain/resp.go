// 跨边界通用形态：统一响应外壳、健康探测与错误码常量。
package domain

// Resp 是 HTTP 统一响应外壳：code="0" 成功，非 "0" 为 AppError 错误码（数字字符串）。
type Resp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// HealthRESP 存活探测出参。
type HealthRESP struct {
	OK bool `json:"ok"`
}
