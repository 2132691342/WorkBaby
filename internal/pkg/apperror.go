// Package pkg 聚合 WorkBaby 的叶子工具能力：统一错误 AppError、ID、日志、路径、AES-GCM 加解密、行级 diff、HTTP method 白名单。
// 各 internal 层可依赖它；它不依赖任何其他 internal 业务包。
package pkg

import (
	"errors"
	"fmt"
)

// AppError 是跨绑定边界的统一错误形态：error → JS Promise reject 时序列化为 {code,message,details}。
// Code 取 1000-9999 按域分段（段位表见 CLAUDE.md §2.4），前后端共同遵守。
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Error 实现 error 接口；Format 给出可读文本，便于日志。
func (e *AppError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// New 构造一个无底层 cause 的 AppError；details 一般用于附加上下文字段。
func New(code int, message, details string) *AppError {
	return &AppError{Code: code, Message: message, Details: details}
}

// Wrap 把底层 err 装进 AppError；底层信息写入 Details；err 为 nil 时返回 nil。
func Wrap(code int, message string, err error) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{Code: code, Message: message, Details: err.Error()}
}

// As 辅助 errors.As 的简写。
func As(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
