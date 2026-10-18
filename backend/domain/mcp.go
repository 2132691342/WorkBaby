// MCP 聚合根：配置来自 mcp.json，在线状态与工具清单来自 `pi mcp list --json`。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrMCPConfig = pkg.New("9101", "MCP 配置格式不正确", "")
	ErrMCPSave   = pkg.New("9102", "保存 MCP 配置失败", "")
	ErrMCPServer = pkg.New("9103", "MCP 服务不存在", "")
)

// MCPServerVO 一个 MCP 服务：配置 + 在线状态。
type MCPServerVO struct {
	Name string `json:"name"`
	// Scope 是 global（~/.pi/agent/mcp.json）或 project（<workspace>/.pi/mcp.json）。
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Enabled bool   `json:"enabled"`
	// Exposure 是 PI 的工具暴露档：direct / codemode / deferred。
	Exposure  string   `json:"exposure"`
	Transport string   `json:"transport"`
	State     string   `json:"state"`
	Error     string   `json:"error"`
	Tools     []string `json:"tools"`
}

// MCPListRESP MCP 面板的完整出参：列表 + 两份配置原文（高级编辑用）。
type MCPListRESP struct {
	Servers     []MCPServerVO `json:"servers"`
	GlobalPath  string        `json:"global_path"`
	ProjectPath string        `json:"project_path"`
	GlobalRaw   string        `json:"global_raw"`
	ProjectRaw  string        `json:"project_raw"`
}

// SaveMCPREQ 保存配置原文：scope 决定写哪一份。
type SaveMCPREQ struct {
	Scope string `json:"scope"`
	// Raw 是 JSON 原文；保存前会做语法与 mcpServers 结构校验。
	Raw string `json:"raw"`
}

// RemoveMCPServerREQ 删掉一个服务（走 `pi mcp remove`）。
type RemoveMCPServerREQ struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

// MCPServerNameREQ 只带名字的入参（登录 / 登出）。
type MCPServerNameREQ struct {
	Name string `json:"name"`
}
