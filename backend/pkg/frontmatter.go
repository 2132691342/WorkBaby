package pkg

import "strings"

// Frontmatter 是技能 / 提示模板开头的 YAML 头里我们关心的两个字段。
type Frontmatter struct {
	// Name 技能的显示名；空串表示没写
	Name string
	// Description 一句话说明
	Description string
}

// ParseFrontmatter 读 `---` 包起来的头部。只认 name / description 两个键：
// 头部其余键属于引擎的扩展点，壳不解释也不改动原文。
func ParseFrontmatter(text string) Frontmatter {
	out := Frontmatter{}
	lines := splitLines(text)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			out.Name = value
		case "description":
			out.Description = value
		}
	}
	return out
}
