// Package skill 是 Skill 系统：SKILL.md 解析 + 注册中心 + 关键词路由。
//
// 边界：skill/ 不依赖 harness / agent / api / service / wails；
// skills 表是唯一真相源，SKILL.md 只是导入/导出格式。
package skill

import (
	"encoding/json"
	"fmt"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gopkg.in/yaml.v3"
)

// frontmatter SKILL.md 头部 YAML 结构。
type frontmatter struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	WhenToUse    whenToUseList `yaml:"when_to_use"`
	AllowedTools []string      `yaml:"allowed_tools"`
	Version      string        `yaml:"version"`
}

// whenToUseList 触发词列表；兼容 YAML 标量（when_to_use: pdf）与序列两种写法，
// 只认序列会让标量写法的技能解析失败被静默跳过。
type whenToUseList []string

// UnmarshalYAML 标量按单个触发词处理；序列按条目列表处理；其余形状按空处理（交由兜底）。
func (w *whenToUseList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if strings.TrimSpace(s) != "" {
			*w = []string{s}
		}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*w = list
		return nil
	default:
		return nil
	}
}

// ParseSKILLMD 解析 SKILL.md → SkillDO。
//
// 格式：首行 `---`，YAML frontmatter，闭合 `---`，之后为 Markdown body。
// 解析失败返回 8002。
func ParseSKILLMD(data []byte, source domain.SkillSourceKind, ref string) (*domain.SkillDO, error) {
	s := string(data)
	if !strings.HasPrefix(strings.TrimSpace(s), "---") {
		return nil, pkg.New(8002, "SKILL.md must start with ---", "")
	}
	// 去掉首行 ---
	rest := strings.TrimSpace(s)[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, pkg.New(8002, "SKILL.md missing closing ---", "")
	}
	head := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:])

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return nil, pkg.Wrap(8002, "parse frontmatter failed", err)
	}
	if fm.Name == "" {
		return nil, pkg.New(8002, "frontmatter name is required", "")
	}
	if fm.Description == "" {
		return nil, pkg.New(8002, "frontmatter description is required", "")
	}
	if body == "" {
		return nil, pkg.New(8002, "SKILL.md body is empty", "")
	}
	tools, err := json.Marshal(fm.AllowedTools)
	if err != nil {
		return nil, pkg.Wrap(8002, "marshal allowed_tools failed", err)
	}
	// 缺 when_to_use 时退回 skill 名兜底：触发词为空会让 Skill 永远匹配不上，
	// 表现为「装进去了但一次都触发不了」的静默死功能。
	whenToUse := strings.Join([]string(fm.WhenToUse), "\n")
	if strings.TrimSpace(whenToUse) == "" {
		whenToUse = fm.Name
	}
	return &domain.SkillDO{
		Name:         fm.Name,
		Description:  fm.Description,
		WhenToUse:    whenToUse,
		Body:         body,
		AllowedTools: string(tools),
		Frontmatter:  strings.TrimSpace(head),
		SourceKind:   source,
		SourceRef:    ref,
		Version:      fm.Version,
		Enabled:      true,
	}, nil
}

// allowedTools 解析 AllowedTools JSON → 切片。
func allowedTools(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// FormatError 供测试断言使用。
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
