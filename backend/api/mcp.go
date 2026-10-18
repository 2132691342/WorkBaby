// MCP handler：服务清单（含在线状态）、配置原文读写、登录登出与重载。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListMCPServers 返回配置里的服务 + 在线状态 + 两份配置原文。
func (h *Handler) ListMCPServers(c *gin.Context) {
	vo, err := h.Svc.MCP.List(c.Query("workspace"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// SaveMCPConfig 保存配置原文（scope 决定写全局还是项目）。
func (h *Handler) SaveMCPConfig(c *gin.Context) {
	var req domain.SaveMCPREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.MCP.Save(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// RemoveMCPServer 删掉一个服务。
func (h *Handler) RemoveMCPServer(c *gin.Context) {
	var req domain.RemoveMCPServerREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.MCP.Remove(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// LoginMCPServer 给 OAuth 服务发起登录。
func (h *Handler) LoginMCPServer(c *gin.Context) {
	var req domain.MCPServerNameREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	go func() {
		if err := h.Svc.MCP.Login(req.Name, func(line string) {
			h.emitSetup("mcp-login", 0, line)
		}); err != nil {
			h.emitSetup("mcp-login", 0, "登录失败："+err.Error())
		}
	}()
	ok(c, true)
}

// LogoutMCPServer 清掉某个服务的 OAuth 凭据。
func (h *Handler) LogoutMCPServer(c *gin.Context) {
	var req domain.MCPServerNameREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.MCP.Logout(req.Name); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ReloadMCP 重启 PI 进程：mcp.json 是启动期读的，不重启等于没改。
func (h *Handler) ReloadMCP(c *gin.Context) {
	if err := h.Svc.MCP.Reload(); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}
