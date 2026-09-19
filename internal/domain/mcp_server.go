// Package domain 是业务聚合根：每个聚合根一个文件，含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量。
//
// 本文件：MCP Server 聚合根：外部 MCP 工具源的接入配置。
package domain

import (
	"gorm.io/gorm"

	"WorkBaby/internal/pkg"
)

// McpTransport MCP 传输方式。
type McpTransport string

const (
	McpTransportStdio McpTransport = "stdio" // v1 唯一支持：子进程 stdin/stdout
	McpTransportHTTP  McpTransport = "http"  // v2
)

// McpConfigFile 是 mcp.json 的配置容器：servers 为数组，mcpServers 为生态兼容对象格式。
type McpConfigFile struct {
	Version    int                       `json:"version"`
	Servers    []McpConfigEntry          `json:"servers"`
	McpServers map[string]McpConfigEntry `json:"mcpServers"` // MCP 生态标准字段名，保持兼容
}

// McpConfigEntry 是 mcp.json 中的单个 server 配置。
type McpConfigEntry struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport McpTransport      `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	BaseURL   string            `json:"base_url"`
	Enabled   *bool             `json:"enabled"`
}

// MCP server 运行状态（运行时态，不落库）。
const (
	McpStatusReady   = "ready"
	McpStatusUnready = "unready"
)

// McpServerDO MCP server 持久化实体（mcp_servers 表）。
type McpServerDO struct {
	ID        string         `gorm:"primaryKey;size:64" json:"id"`
	Name      string         `gorm:"size:64;uniqueIndex" json:"name"`
	Transport McpTransport   `gorm:"size:16" json:"transport"`
	Command   string         `gorm:"size:512" json:"command"`
	Args      string         `gorm:"type:text" json:"-"` // JSON 数组
	Env       string         `gorm:"type:text" json:"-"` // JSON map
	BaseURL   string         `gorm:"size:512" json:"base_url"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
	ToolCount int            `gorm:"default:0" json:"tool_count"`
	CreatedAt int64          `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64          `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 固定表名。
func (McpServerDO) TableName() string { return "mcp_servers" }

// McpServerREQ 新增/更新入参（前端 JSON 友好：args/env 用原生类型，落库前序列化）。
type McpServerREQ struct {
	Name      string            `json:"name"`
	Transport McpTransport      `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	Enabled   *bool             `json:"enabled"`
}

// McpServerRESP 出参：落库配置 + 运行时状态（Ready/Error 来自 Manager）。
type McpServerRESP struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport McpTransport      `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	Enabled   bool              `json:"enabled"`
	ToolCount int               `json:"tool_count"`
	Ready     bool              `json:"ready"`
	Error     string            `json:"error,omitempty"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

// McpRawREQ mcp.json 原始内容保存入参（前端 JSON 编辑器）。
type McpRawREQ struct {
	Content string `json:"content"`
}

// McpRawRESP mcp.json 原始内容出参。
type McpRawRESP struct {
	Content string `json:"content"`
}

// McpReloadRESP 热重载 / 原始保存结果。
type McpReloadRESP struct {
	Active int  `json:"active"` // 就绪 server 数
	Saved  bool `json:"saved"`
}

// 包级错误变量；错误码段位 8000。
var (
	ErrMcpServerNotFound = pkg.New(8009, "mcp server not found", "")
	ErrMcpStartFailed    = pkg.New(8003, "mcp server start failed", "")
	ErrMcpInitialize     = pkg.New(8004, "mcp initialize failed", "")
	ErrMcpToolCall       = pkg.New(8005, "mcp tool call failed", "")
	ErrMcpListTools      = pkg.New(8006, "mcp tools/list failed", "")
	ErrMcpNameConflict   = pkg.New(8007, "mcp tool name conflict", "")
	ErrMcpTransport      = pkg.New(8008, "mcp transport unsupported", "")
)
