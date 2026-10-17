// MCP 外部工具 HTTP handler：薄层，参数解析与响应组装，编排在 service。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// GetMCPConfig 读回配置原文与解析结果。
func (h *Handler) GetMCPConfig(c *gin.Context) {
	resp, err := h.Svc.MCP.Config()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// SaveMCPConfig 保存配置原文：先校验再落盘，成功即重载。
func (h *Handler) SaveMCPConfig(c *gin.Context) {
	var req domain.SaveMCPConfigREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.MCP.SaveConfig(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// MCPTemplate 内置模板，供「填入模板」按钮使用。
func (h *Handler) MCPTemplate(c *gin.Context) {
	ok(c, h.Svc.MCP.Templates())
}

// ReloadMCP 重启 PI 进程：mcp.json 是启动期读的，不重启等于没改。
func (h *Handler) ReloadMCP(c *gin.Context) {
	if err := h.Svc.MCP.Reload(); err != nil {
		fail(c, err)
		return
	}
	resp, err := h.Svc.MCP.Config()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// ListMCPServers 配置里的服务列表。
func (h *Handler) ListMCPServers(c *gin.Context) {
	ok(c, h.Svc.MCP.List())
}

// ProbeMCPServer 已删除：客户端是 PI，WorkBaby 不再自己握手。
// 保留路由但告诉调用方改用重载 + 工具清单：探测失败时原因也在 PI 的日志里。
func (h *Handler) ProbeMCPServer(c *gin.Context) {
	fail(c, pkg.New("9109", "现在由助手引擎自己连接 MCP 服务", "保存后点「重新加载」，再看工具清单里有没有这个服务的工具"))
}

// ListMCPTools 当前注册的全部 MCP 工具。
func (h *Handler) ListMCPTools(c *gin.Context) {
	ok(c, h.Svc.MCP.Tools())
}
