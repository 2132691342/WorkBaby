package server

import (
	"time"

	"WorkBaby/internal/pkg"
	"github.com/gin-gonic/gin"
)

// traceIDKey 请求链路 ID 的 context key（注入 ctx 供日志）。
const traceIDKey = "wbTraceID"

// requestPath 取请求路径：命中路由用路由模板（低基数），未命中用真实 URL（定位 404 契约漂移）。
func requestPath(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return c.Request.URL.RequestURI()
}

// logger 访问日志中间件：slog 结构化（method/path/status/latency）。
// 非 2xx 一律提到 Warn/Error 等级，保证异常请求单独落文件。
func logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", requestPath(c),
			"status", status,
			"latencyMs", time.Since(start).Milliseconds(),
			"client", c.ClientIP(),
		}
		if pkg.L == nil {
			return
		}
		switch {
		case status >= 500:
			pkg.L.Error("http", attrs...)
		case status >= 400:
			pkg.L.Warn("http", attrs...)
		default:
			pkg.L.Info("http", attrs...)
		}
	}
}

// recoverer panic 恢复中间件：吞掉 panic，返回 500，避免进程崩溃。
func recoverer() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				if pkg.L != nil {
					pkg.L.Error("panic recovered", "path", requestPath(c), "panic", r)
				}
				Fail(c, pkg.New(5000, "internal panic", ""))
				c.Abort()
			}
		}()
		c.Next()
	}
}

// cors dev 场景下前端（vite 5173）跨端口访问后端 gin；生产同源（AssetServer），CORS 不生效。
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
