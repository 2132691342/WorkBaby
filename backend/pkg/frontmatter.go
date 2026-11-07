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
// 块标量（`|` 与 `>`）按 YAML 规则接上缩进行，只切当前行的话 description 只剩一个「|」。
func ParseFrontmatter(text string) Frontmatter {
	out := Frontmatter{}
	lines := splitLines(text)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" {
			break
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			out.Name = unquoteYAML(value)
		case "description":
			if folded, ok := readBlockScalar(lines, i+1, value); ok {
				out.Description = folded
				continue
			}
			out.Description = unquoteYAML(value)
		}
	}
	return out
}

// readBlockScalar 读块标量：从 start 行起收集缩进更深的行。
// `|` 保留换行，`>` 把换行折成空格——两者都要把内容接成一段。
func readBlockScalar(lines []string, start int, marker string) (string, bool) {
	if marker != "|" && marker != ">" && marker != "|-" && marker != ">-" {
		return "", false
	}
	folded := marker == ">" || marker == ">-"
	var parts []string
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		// 空行属于块内容；遇到新的顶层键或头部结束才算块结束。
		if trimmed == "" {
			parts = append(parts, "")
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		if strings.HasPrefix(trimmed, "---") {
			break
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return "", true
	}
	if folded {
		// 折行标量：空行折成空格，其余行用空格相连。
		return strings.TrimSpace(strings.Join(parts, " ")), true
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), true
}

// unquoteYAML 去掉首尾引号；内容里还有引号时按原样保留。
func unquoteYAML(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		if (first == '"' || first == '\'') && value[len(value)-1] == first {
			return value[1 : len(value)-1]
		}
	}
	return value
}
