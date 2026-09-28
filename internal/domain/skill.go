package domain

import "WorkBaby/internal/pkg"

var (
	ErrSkillNotFound = pkg.New(6103, "技能不存在", "")
	ErrSkillName     = pkg.New(6101, "技能名不合法", "只能用小写字母、数字和连字符")
	ErrSkillFront    = pkg.New(6102, "技能文件缺少正确的开头说明", "")
)

// Skill 是磁盘上一个 SKILL.md 的解析结果，不落库——文件即真相源。
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
}

// SkillVO 出参与 Skill 同形，单独声明避免内部字段外泄。
type SkillVO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
}

// ToggleSkillREQ 启停入参。
type ToggleSkillREQ struct {
	Enabled bool `json:"enabled"`
}

// CreateSkillREQ 新建技能入参。Name 必须是合法技能名，Body 是方法论正文。
type CreateSkillREQ struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// ImportSkillsREQ 导入技能入参：可以是 SKILL.md，也可以是包含它的文件夹。
type ImportSkillsREQ struct {
	Paths []string `json:"paths"`
}

// ImportSkillsRESP 导入出参。
type ImportSkillsRESP struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// SkillContentRESP 技能正文出参。
type SkillContentRESP struct {
	Content string `json:"content"`
}

// ToVO 转成出参。
func (s *Skill) ToVO() SkillVO {
	return SkillVO{
		ID: s.ID, Name: s.Name, Description: s.Description,
		Location: s.Location, Source: s.Source, Enabled: s.Enabled,
	}
}

// 技能来源。
const (
	SkillSourceBuiltin   = "builtin"
	SkillSourceGlobal    = "global"
	SkillSourceWorkspace = "workspace"
)
