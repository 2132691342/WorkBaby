// Package pkg 提供无业务语义的叶子工具：错误、ID、日志、路径、加密、HTTP 规则与文件辅助。
package pkg

import (
	"errors"
	"fmt"
)

// AppError 是跨层传递的统一错误：code 供前端分流，message 面向用户，details 供排障。
// code 是数字字符串（"1101"），与 HTTP 状态码的 int 从类型上分开。
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 暴露底层错误，使 errors.Is / errors.As 可用。
func (e *AppError) Unwrap() error { return e.Err }

// New 构造一个不带底层原因的业务错误。
func New(code string, message, details string) *AppError {
	return &AppError{Code: code, Message: message, Details: details}
}

// Wrap 把底层错误包装为带 code 的业务错误。
func Wrap(code string, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// CodeOf 取错误的 code；无错误返回成功码 "0"，非 AppError 返回通用内部错误码 "9999"。
func CodeOf(err error) string {
	if err == nil {
		return "0"
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "9999"
}
