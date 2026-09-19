package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"WorkBaby/internal/api"
	"WorkBaby/internal/event"
	"WorkBaby/internal/pkg"
	"github.com/gin-gonic/gin"
)

// Server 双主机 HTTP 服务：gin.Engine + SSE hub + api.Handler。
// 业务路由全部注册在 router.go；这里负责装配与生命周期。
type Server struct {
	engine  *gin.Engine
	handler *api.Handler
	hub     *SSEHub
	srv     *http.Server
	port    int
}

// New 构造 Server；handler 为已 Startup 的 api.Handler（含全部 service）。
// log 为 run 事件日志，供 SSE 断线重放；可为 nil（退化为无重放）。
func New(handler *api.Handler, bus *event.Bus, log *event.RunEventLog) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(logger(), recoverer(), cors())
	s := &Server{
		engine:  engine,
		handler: handler,
		hub:     NewSSEHub(bus, log),
	}
	s.registerRoutes()
	// 未命中路由：返回统一 JSON 而非 gin 默认 HTML，前端可直接读到 code=1404
	s.engine.NoRoute(func(c *gin.Context) {
		Fail(c, pkg.New(1404, "接口不存在: "+c.Request.Method+" "+c.Request.URL.Path, ""))
	})
	return s
}

// Port 返回实际监听端口（Start 后有效）。
func (s *Server) Port() int { return s.port }

// Start 监听 127.0.0.1:0（随机端口）并后台 serve；返回绑定地址供注入前端。
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen http failed: %w", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{Handler: s.engine}
	go func() {
		if serr := s.srv.Serve(ln); serr != nil && serr != http.ErrServerClosed {
			// 启动失败记日志；进程主循环继续（GUI 仍可用，API 不可用）
			fmt.Printf("[server] serve failed: %v\n", serr)
		}
	}()
	return fmt.Sprintf("http://127.0.0.1:%d", s.port), nil
}

// Shutdown 优雅关闭 HTTP 服务与 SSE 连接。
func (s *Server) Shutdown(ctx context.Context) {
	s.hub.Close()
	if s.srv != nil {
		shCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shCtx)
	}
}
