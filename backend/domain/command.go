// 斜杠命令聚合根：`/` 菜单里一条命令的来源与说明。
package domain

// 命令来源：扩展注册 / 提示模板 / 技能。
const (
	CommandSourceExtension = "extension"
	CommandSourcePrompt    = "prompt"
	CommandSourceSkill     = "skill"
)

// CommandVO 是 PI 报上来的一个斜杠命令。
// 真相在 PI 进程里（扩展命令只有它知道），本层不缓存、不补全。
type CommandVO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// ExportSessionREQ 导出会话入参：Path 由用户在保存对话框里选。
type ExportSessionREQ struct {
	Path string `json:"path"`
}

// ExportSessionRESP 导出出参：真写出去的那个路径。
type ExportSessionRESP struct {
	Path string `json:"path"`
}
