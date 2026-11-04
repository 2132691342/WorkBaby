// Package pkg 是叶子工具层：错误、信封、ID、文件、日志、子进程与环境。
// 它只依赖标准库，任何上层都可以引用它，它不引用任何上层。
package pkg

// AppError 是唯一业务错误类型：Code 给界面分流，Message 直接给人看，Detail 给诊断面板。
type AppError struct {
	// Code 业务码（数字字符串），与 docs/ERROR-CODES.md 一一对应
	Code string
	// Message 人话描述
	Message string
	// Detail 补充上下文：stderr / 路径 / 上游原话
	Detail string
}

// Error 让 AppError 满足 error 接口。
func (e *AppError) Error() string {
	return e.Message
}

// Err 造一个业务错误；detail 可选，给了就带上。
func Err(code, message string, detail ...string) *AppError {
	e := &AppError{Code: code, Message: message}
	if len(detail) > 0 {
		e.Detail = detail[0]
	}
	return e
}

// WrapErr 把一个底层 error 归一到给定业务码下，原文进 Detail。
func WrapErr(code, message string, err error) *AppError {
	if err == nil {
		return Err(code, message)
	}
	return Err(code, message, err.Error())
}

// As 把任意 error 归一成 *AppError；已经是就原样返回，否则按通用错误包一层。
func As(err error) *AppError {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*AppError); ok {
		return ae
	}
	return &AppError{Code: CodeInternal, Message: err.Error()}
}
