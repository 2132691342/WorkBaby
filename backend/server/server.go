// Package server 是 HTTP 层：gin 路由、统一响应与 SSE 推送。
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

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
	// corsLocal 必须在 methodGuard 之前：预检若被 methodGuard 先拦下就不带 CORS 头，
	// 浏览器会拒绝后续 POST，前端只看到 Network Error。
	engine.Use(gin.Recovery(), corsLocal(), methodGuard())

	hub := NewHub()
	if h.Emitter != nil {
		h.Emitter.SetSink(func(sessionID string, env domain.Envelope) { hub.Publish(sessionID, env) })
		// 会话被删时把重放窗口与待合并 delta 一起清掉：Emitter 是 service 层的
		// 东西，反过来 import server 就破了依赖方向，只能从这里挂进去。
		h.Emitter.SetCutHook(hub.DropSession)
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
	go func() {
		// Serve 只在退出时返回，正常关闭是 ErrServerClosed；
		// 其余错误（监听器被抢、fd 耗尽）必须留痕，否则进程以为服务是健康的。
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			pkg.Errorf("server: 本地服务退出: %v", err)
		}
	}()
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

// corsLocal 只放行本机来源。回显任意 Origin 会让任何一个本地网页的脚本
// 都能读到本地接口的响应——包括密钥查看接口返回的明文 Key。
func corsLocal() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && localOrigin(origin) {
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

// localOrigin 判定来源是否来自本机：Wails 壳（wails:// 与 http://wails.localhost）、
// 本机回环页面与 vite dev server（任意端口的 localhost / 127.0.0.1）。
func localOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme == "wails" {
		return true
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "wails.localhost":
		return true
	}
	return false
}
