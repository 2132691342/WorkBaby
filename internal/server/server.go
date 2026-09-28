// Package server 是 HTTP 层：gin 路由、统一响应与 SSE 推送。
package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	"github.com/gin-gonic/gin"
)

// Server 是内嵌 HTTP 服务：只绑回环 + 随机端口，本机单用户。
type Server struct {
	engine *gin.Engine
	hub    *Hub
	srv    *http.Server
	port   int
}

// New 构造 HTTP 服务；Hub 同时作为事件出口的 sink 装配到 Emitter。
func New(h *api.Handler) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// corsLocal 必须在 methodGuard 之前：打包版前端来自 wails.localhost，
	// 与 127.0.0.1 跨源，POST 的 OPTIONS 预检若先被 methodGuard 拦下，
	// 响应不带 CORS 头，浏览器直接拒绝后续 POST——前端只会看到 Network Error。
	engine.Use(gin.Recovery(), corsLocal(), methodGuard())

	hub := NewHub()
	if h.Emitter != nil {
		h.Emitter.SetSink(func(sessionID string, env domain.Envelope) { hub.Publish(sessionID, env) })
	}
	s := &Server{engine: engine, hub: hub}
	registerRoutes(engine, h, hub)
	return s
}

// Start 监听回环随机端口并返回实际端口。
func (s *Server) Start() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, pkg.Wrap(2202, "启动本地服务失败", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return 0, pkg.New(2202, "解析监听地址失败", "")
	}
	s.port = addr.Port
	s.srv = &http.Server{Handler: s.engine, ReadHeaderTimeout: 15 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s.port, nil
}

// Port 返回实际监听端口。
func (s *Server) Port() int { return s.port }

// Stop 关闭服务。
func (s *Server) Stop() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

// methodGuard 只放行 GET 与 POST，杜绝误用其它方法造成语义漂移。
func methodGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !pkg.AllowMethod(c.Request.Method) {
			c.JSON(200, gin.H{"code": 1002, "message": "只允许 GET 与 POST", "data": nil})
			c.Abort()
			return
		}
		c.Next()
	}
}

// corsLocal 允许前端在 vite dev server 下直连后端（来源仍是本机）。
func corsLocal() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
