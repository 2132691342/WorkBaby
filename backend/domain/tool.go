// 工具聚合根：PI 当前声明给模型的工具清单（含内置、扩展与 MCP）。
package domain

// 工具来源。
const (
	ToolSourceBuiltin   = "builtin"
	ToolSourceExtension = "extension"
	ToolSourceMCP       = "mcp"
)

// ToolVO 一个工具；启用状态来自 PI settings.json 的 defaultTools。
type ToolVO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	// Server 只在 MCP 工具有值。
	Server  string `json:"server,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ToolListRESP 工具清单出参。
type ToolListRESP struct {
	Tools []ToolVO `json:"tools"`
	// Snapshot 说明这份清单是从哪来的：session（最近一次会话的工具声明）/
	// mcp（MCP 服务的在线工具）/ static（没有会话时按 PI 内置表推断）。
	Snapshot string `json:"snapshot"`
	// SnapshotAt 是那份会话的更新时间（毫秒），0 表示没有会话可用。
	SnapshotAt int64 `json:"snapshot_at"`
}

// ToggleToolREQ 启停一个工具。
type ToggleToolREQ struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}
