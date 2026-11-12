// Package api 是 HTTP 薄壳：解析入参、调 service、回信封。它不写业务逻辑。
package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

// Server 是回环 HTTP 服务：业务 API 与 SSE 全走这里，前端不碰文件系统与子进程。
type Server struct {
	// svc 业务装配根
	svc *service.App
	// hub 事件广播
	hub *sse.Hub
	// token 鉴权令牌：每次启动随机
	token string
	// srv 底层 http 服务
	srv *http.Server
	// listener 实际监听器（端口由它给出）
	listener net.Listener
	// port 监听端口
	port int
}

// New 造一个服务；端口在 Start 时才确定。
func New(svc *service.App, hub *sse.Hub, token string) *Server {
	return &Server{svc: svc, hub: hub, token: token}
}

// Port 当前监听端口；未启动为 0。
func (s *Server) Port() int { return s.port }

// Start 监听 127.0.0.1 的随机端口并开始服务；返回实际端口。
func (s *Server) Start() (int, error) {
	if s.srv != nil {
		return s.port, nil
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// 不用 gin 的默认日志：它打到 stdout，而发布版没有控制台。
	// CORS 必须排在鉴权前面：浏览器对跨源请求先发 OPTIONS 预检，
	// 预检不携带令牌，先鉴权会把预检打成 401，之后所有真请求都被浏览器拦下。
	engine.Use(gin.Recovery(), s.corsMiddleWare(), s.authMiddleWare())
	s.mount(engine)
	// 未知接口回信封形式的 1404：前端拿它和业务错误走同一条显示路径。
	engine.NoRoute(s.unknown)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, pkg.WrapErr(pkg.CodeInternal, "本地端口监听失败", err)
	}
	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{
		Handler: engine,
		// 不设 WriteTimeout：SSE 是长连接，写超时会把对话流掐断。
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := s.srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			pkg.Warn("HTTP 服务退出：%v", err)
		}
	}()
	pkg.Info("HTTP 服务已监听 127.0.0.1:%d", s.port)
	return s.port, nil
}

// Handler 返回已装配的 gin 引擎；未启动时现场装一份。
// 测试用它绕过真实端口直接打路由——链路本身才是要断言的东西。
func (s *Server) Handler() http.Handler {
	if s.srv != nil {
		return s.srv.Handler
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), s.corsMiddleWare(), s.authMiddleWare())
	s.mount(engine)
	engine.NoRoute(s.unknown)
	return engine
}

// Stop 关闭服务；给一个短超时，别让 SSE 长连接拖住退出。
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// corsMiddleWare 补齐跨源访问：WebView2 里页面源是 wails.localhost 虚拟域名，
// 对回环上的本服务就是跨源；开发态前端又被 vite 端口伺服，同样跨源。
// 浏览器对带自定义头的请求先发不带令牌的 OPTIONS 预检，这里直接放行并回响应头。
func (s *Server) corsMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" && originAllowed(origin) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-Desktop-Token")
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// originAllowed 判断来源是否自家界面：WebView2 的虚拟域与开发态本地端口。
// 服务只绑回环地址、令牌每次启动随机，真正的门禁是令牌；这里只是让浏览器放行。
func originAllowed(origin string) bool {
	if origin == "http://wails.localhost" || origin == "https://wails.localhost" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

// authMiddleWare 校验令牌：HTTP 走请求头，SSE 走查询参数（EventSource 带不了自定义头）。
func (s *Server) authMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		given := c.GetHeader("X-Desktop-Token")
		if given == "" {
			given = c.Query("token")
		}
		if given != s.token {
			// 鉴权失败回 401 而不是信封：这不是业务失败，是根本没进门。
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// base 是挂载点。
const base = "/api/v1"

// mount 挂上全部路由。只允许 GET / POST：动词靠路径段表达，写操作不出现在 GET 上。
func (s *Server) mount(engine *gin.Engine) {
	g := engine.Group(base)

	// 引导 / 设置 / 偏好
	g.GET("/bootstrap", s.bootstrap)
	g.GET("/settings", s.settingsGet)
	g.POST("/settings/save", s.settingsSave)
	g.POST("/settings/workspace", s.settingsWorkspace)
	g.GET("/workspaces", s.workspacesGet)
	g.POST("/settings/workspace/forget", s.workspaceForget)
	g.GET("/prefs", s.prefsGet)
	g.POST("/prefs/autostart", s.prefsAutoStart)
	g.POST("/prefs/restore", s.prefsRestore)
	g.POST("/prefs/close-action", s.prefsCloseAction)
	g.POST("/prefs/data-dir", s.prefsOpenDataDir)
	g.GET("/logs", s.logs)

	// 引擎 / 应用 / 健康
	g.GET("/engines", s.enginesGet)
	g.POST("/engines/detect", s.enginesDetect)
	g.POST("/engines/restart", s.enginesRestart)
	g.POST("/engines/repair", s.enginesRepair)
	g.GET("/health/engines", s.healthGet)
	g.GET("/apps", s.appsGet)

	// 会话
	g.GET("/sessions", s.sessionsList)
	g.GET("/sessions/detail", s.sessionDetail)
	g.POST("/sessions/delete", s.sessionDelete)
	g.POST("/sessions/rename", s.sessionRename)

	// 对话
	g.GET("/chat/state", s.chatState)
	g.POST("/chat/send", s.chatSend)
	g.POST("/chat/stop", s.chatStop)
	g.POST("/chat/steer", s.chatSteer)
	g.POST("/chat/followup", s.chatFollowUp)
	g.POST("/chat/clear-queue", s.chatClearQueue)
	g.POST("/chat/remove-queued", s.chatRemoveQueued)
	g.POST("/chat/compact", s.chatCompact)
	g.POST("/chat/answer", s.chatAnswer)
	g.POST("/chat/new", s.chatNew)
	g.POST("/chat/clone", s.chatClone)
	g.POST("/chat/export-html", s.chatExport)
	g.POST("/chat/model", s.chatModel)
	g.POST("/chat/thinking", s.chatThinking)
	g.POST("/chat/autocompact", s.chatAutocompact)
	g.POST("/chat/plan", s.chatPlan)
	g.POST("/messages/hide", s.messageHide)
	g.POST("/messages/unhide", s.messageUnhide)

	// 模型与服务商
	g.GET("/models", s.modelsList)
	g.GET("/providers", s.providersGet)
	g.POST("/providers/upsert", s.providerUpsert)
	g.POST("/providers/delete", s.providerDelete)
	g.POST("/providers/model/save", s.providerModelSave)
	g.POST("/providers/model/delete", s.providerModelDelete)
	g.POST("/providers/models", s.providerProbe)
	g.POST("/providers/test", s.providerProbe)

	// 技能 / MCP
	g.GET("/skills", s.skillsList)
	g.POST("/skills/install", s.skillInstall)
	g.GET("/skills/content", s.skillContent)
	g.POST("/skills/save", s.skillSave)
	g.POST("/skills/toggle", s.skillToggle)
	g.POST("/skills/create", s.skillCreate)
	g.POST("/skills/import", s.skillImport)
	g.POST("/skills/delete", s.skillDelete)
	g.POST("/skills/reset", s.skillReset)
	g.GET("/mcp", s.mcpList)
	g.POST("/mcp/save", s.mcpSave)
	g.POST("/mcp/toggle", s.mcpToggle)
	g.POST("/mcp/probe", s.mcpProbe)
	g.POST("/mcp/remove", s.mcpRemove)

	// 联网
	g.GET("/web/status", s.webStatus)
	g.POST("/web/settings", s.webSettings)
	g.POST("/web/search", s.webSearch)
	g.POST("/web/fetch", s.webFetch)

	// 工作区预索引
	g.GET("/index/status", s.indexStatus)
	g.POST("/index/search", s.indexSearch)
	g.POST("/index/rebuild", s.indexRebuild)

	// 文件 / git / 系统
	g.GET("/files", s.filesList)
	g.GET("/files/search", s.filesSearch)
	g.GET("/files/read", s.fileRead)
	g.POST("/files/save", s.fileSave)
	g.POST("/files/mkdir", s.fileMkdir)
	g.POST("/files/delete", s.fileDelete)
	g.POST("/files/rename", s.fileRename)
	mountGit(g, s)
	g.POST("/system/reveal", s.systemReveal)
	g.POST("/system/open", s.systemOpen)
	g.GET("/config/files", s.configFiles)
	g.GET("/config/text", s.configText)

	// SSE
	g.GET("/events", s.events)
}

// unknown 回一个信封形式的 1404：前端拿它和业务错误走同一条显示路径。
func (s *Server) unknown(c *gin.Context) {
	pkg.Fail(c, pkg.Err(pkg.CodeUnknownRoute, "未知接口", c.Request.Method+" "+c.Request.URL.Path))
}

// bind 解析 JSON body；失败直接回 1000 并返回 false。
func bind[T any](c *gin.Context, out *T) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		pkg.Fail(c, pkg.Err(pkg.CodeBadRequest, "请求体不合法", err.Error()))
		return false
	}
	return true
}

// queryInt 读一个整数查询参数。
func queryInt(c *gin.Context, key string, fallback int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

// ok 回一个「只带成功标志」的信封。
func ok(c *gin.Context) { pkg.OK(c, domain.OkRESP{Ok: true}) }
