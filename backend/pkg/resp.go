package pkg

import "github.com/gin-gonic/gin"

// Envelope 是 HTTP 出口的唯一形状：恒回 200，业务失败看 Code。
type Envelope struct {
	// Code 业务码（数字字符串），"0" 即成功
	Code string `json:"code"`
	// Message 人话描述，界面直接显示
	Message string `json:"message"`
	// Detail 补充上下文，只进诊断面板
	Detail string `json:"detail,omitempty"`
	// Data 载荷；失败时为 nil
	Data any `json:"data"`
}

// OK 回一帧成功信封。
func OK(c *gin.Context, data any) {
	c.JSON(200, Envelope{Code: CodeOK, Data: data})
}

// Fail 回一帧失败信封；err 不是 *AppError 时按内部错误处理。
func Fail(c *gin.Context, err error) {
	ae := As(err)
	c.JSON(200, Envelope{Code: ae.Code, Message: ae.Message, Detail: ae.Detail})
}
