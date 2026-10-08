// Package pkg 提供无业务语义的叶子工具：错误、ID、日志、路径、加密、HTTP 规则与文件辅助。
package pkg

import (
	"errors"
	"fmt"
)

// AppError 是跨层传递的统一错误：code 供前端分流，message 面向用户，details 供排障。
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	if e.Details != "" {
		return fmt.Sprintf("[%d] %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 暴露底层错误，使 errors.Is / errors.As 可用。
func (e *AppError) Unwrap() error { return e.Err }

// New 构造一个不带底层原因的业务错误。
func New(code int, message, details string) *AppError {
	return &AppError{Code: code, Message: message, Details: details}
}

// Wrap 把底层错误包装为带 code 的业务错误。
func Wrap(code int, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// CodeOf 取错误的 code；非 AppError 返回通用内部错误码 9999。
func CodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 9999
}
