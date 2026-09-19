// Package server 是双主机 HTTP 层：gin 装配、统一响应、SSE hub、中间件。
// 依赖方向：server → api → service（server 不直接调 service/repo/能力域）。
package server

import (
	"net/http"

	"WorkBaby/internal/pkg"
	"github.com/gin-gonic/gin"
)

// Response 统一响应体：code=0 成功，非 0 为 AppError 错误码。
// 前端 api/http.ts 解包 body.data；code != 0 抛 Error(body.message)。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Details string `json:"details,omitempty"`
}

// OK 成功响应（HTTP 200 + code=0）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok", Data: data})
}

// OKNoData 成功但无 data（删/停等空操作）。
func OKNoData(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok"})
}

// Fail 业务失败：AppError → {code,message}；普通 error → 500。
// HTTP 恒 200（前端按 body.code 分流，桌面场景不依赖 HTTP 状态码）。
func Fail(c *gin.Context, err error) {
	if err == nil {
		OKNoData(c)
		return
	}
	if ae, ok := pkg.As(err); ok {
		c.JSON(http.StatusOK, Response{Code: ae.Code, Message: ae.Message, Details: ae.Details})
		return
	}
	c.JSON(http.StatusOK, Response{Code: 5000, Message: "internal error", Details: err.Error()})
}

// BindJSON 绑定 body 到目标结构；失败返回 4001。
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return pkg.New(4001, "invalid request body", err.Error())
	}
	return nil
}

// unwrap 处理 api.Handler 方法的 (T, error) 返回：err 非 nil → Fail，否则 OK。
// 用法：unwrap(c, h.ListSessions(...))（多返回值自动展开）。
// 注意：必须是非泛型签名——Go 的多值展开不适用于泛型函数参数。
func unwrap(c *gin.Context, v any, err error) {
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, v)
}
