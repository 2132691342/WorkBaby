package api

import (
	"path/filepath"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ListMcpServers 返回 MCP server 配置 + 运行时状态（env 值掩码）。
func (h *Handler) ListMcpServers() ([]domain.McpServerRESP, error) {
	return h.mcpSvc.List(h.ctx)
}

// AddMcpServer 新增或更新 MCP server 配置，并立即起停子进程。
func (h *Handler) AddMcpServer(req domain.McpServerREQ) (domain.McpServerRESP, error) {
	p, err := h.mcpSvc.Add(h.ctx, &req)
	if err != nil {
		return domain.McpServerRESP{}, err
	}
	return *p, nil
}

// SetMcpServerEnabled 启停 MCP server。
func (h *Handler) SetMcpServerEnabled(name string, enabled bool) error {
	return h.mcpSvc.SetEnabled(h.ctx, name, enabled)
}

// RemoveMcpServer 删除配置并停掉子进程。
func (h *Handler) RemoveMcpServer(name string) error {
	return h.mcpSvc.Remove(h.ctx, name)
}

// GetMcpRaw 返回 mcp.json 兼容原始内容（JSON 编辑器用）。
func (h *Handler) GetMcpRaw() (domain.McpRawRESP, error) {
	content, err := h.mcpSvc.Raw(h.ctx)
	if err != nil {
		return domain.McpRawRESP{}, err
	}
	return domain.McpRawRESP{Content: content}, nil
}

// SaveMcpRaw 解析 mcp.json 原始内容并全量对齐 + 热重载。
func (h *Handler) SaveMcpRaw(req domain.McpRawREQ) (domain.McpReloadRESP, error) {
	active, err := h.mcpSvc.SaveRaw(h.ctx, req.Content)
	if err != nil {
		return domain.McpReloadRESP{}, err
	}
	return domain.McpReloadRESP{Active: active, Saved: true}, nil
}

// ReloadMcpServers 热重载全部 MCP server。
func (h *Handler) ReloadMcpServers() (domain.McpReloadRESP, error) {
	active, err := h.mcpSvc.Reload(h.ctx)
	if err != nil {
		return domain.McpReloadRESP{}, err
	}
	return domain.McpReloadRESP{Active: active}, nil
}

// RevealMcpFile 打开数据目录（Go 版 MCP 配置存于数据库，无 mcp.json 文件）。
func (h *Handler) RevealMcpFile() error {
	if h.paths == nil || h.paths.Home == "" {
		return pkg.New(8000, "home path not ready", "")
	}
	wruntime.BrowserOpenURL(h.ctx, "file:///"+filepath.ToSlash(h.paths.Home))
	return nil
}
