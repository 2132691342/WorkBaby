// 跨边界通用形态：统一响应外壳与错误码常量。
package domain

// Resp 是 HTTP 统一响应外壳：code=0 成功，非 0 为 AppError 错误码。
type Resp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}
