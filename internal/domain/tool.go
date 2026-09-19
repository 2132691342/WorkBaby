package domain

// ToolParamVO 工具参数视图（由 ToolSchema.Parameters 解析，供前端工具页展示）。
type ToolParamVO struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// ToolMeta 工具元信息（工具页展示；RiskLevel 取 tool 包枚举的字符串值）。
// Category / ActivityDesc 供前端按动作类别呈现（图标、动词），避免前端硬编码工具名映射。
type ToolMeta struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	RiskLevel    string        `json:"risk_level"`
	Group        string        `json:"group"`
	Category     string        `json:"category"`
	ActivityDesc string        `json:"activity_desc"`
	ReadOnly     bool          `json:"read_only"`
	Destructive  bool          `json:"destructive"`
	Enabled      bool          `json:"enabled"`
	Params       []ToolParamVO `json:"params"`
	SchemaJSON   string        `json:"schema_json"`
}
