// Package resource 统一加载 AGENTS.md / 斜杠命令两类基于目录的资源。
//
// 拆出来是 §9.7 提到的「PI 形态 resource 边界」最小落地：
//   - 原 service/agents_md.go（56 LOC）+ service/command_file.go（112 LOC）合并到本包
//   - frontmatter 解析器一并迁入（service/frontmatter.go 也将迁入）
//   - skill 包（internal/skill/）保留不动：已是 well-shaped 的独立包
//
// 单一职责：目录 → 解析 → 切片。失败条目降级跳过，不阻断整体装载。
package resource

import (
	"regexp"
	"strconv"
	"strings"
)

// CommandNamePattern kebab-case 命令名（与 / 面板输入习惯一致）。
// 长度 1~64：与 PI SkillSpec 同档边界，避免超长异常名。
var CommandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// FrontMatter 极简 frontmatter：只支持顶层 `key: value`（值可为逗号/空格分隔的列表）。
// 命令与 Agent 定义文件只需 6~8 个平铺字段；为读这些字段引入完整 YAML 解析器代价不成比例。
type FrontMatter struct {
	fields map[string]string
	body   string
}

// SplitFrontMatter 拆分 frontmatter 与正文；无 frontmatter 时 fields 为空、body 为原文。
func SplitFrontMatter(s string) FrontMatter {
	out := FrontMatter{fields: map[string]string{}, body: strings.TrimSpace(s)}
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "---") {
		return out
	}
	rest := t[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return out
	}
	out.body = strings.TrimSpace(rest[idx+4:])
	for _, line := range strings.Split(rest[:idx], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		out.fields[k] = trimQuotes(strings.TrimSpace(val))
	}
	return out
}

// trimQuotes 去掉值的成对引号（写 frontmatter 时带引号是常见习惯）。
func trimQuotes(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// get 读取字符串字段（键名大小写不敏感，兼容 camelCase 写法）。
func (f FrontMatter) Get(key string) string {
	if v := f.fields[strings.ToLower(key)]; v != "" {
		return v
	}
	// camelCase → kebab-case 回退：disallowedTools / disallowed-tools 两种写法都认
	kebab := camelToKebab(key)
	return f.fields[kebab]
}

// list 读取列表字段：逗号或空白分隔，兼容 `[a, b]` 写法。
func (f FrontMatter) List(key string) []string {
	raw := strings.Trim(f.Get(key), "[]")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := trimQuotes(strings.Trim(strings.TrimSpace(p), `"'`)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// boolean 布尔字段：true/false/yes/no/1/0/on/off；缺失或不可解析返回 def。
func (f FrontMatter) Boolean(key string, def bool) bool {
	switch strings.ToLower(f.Get(key)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	}
	return def
}

// intValue 整数字段：缺失或不可解析返回 def。
func (f FrontMatter) IntValue(key string, def int) int {
	v := f.Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// Body 取正文。
func (f FrontMatter) Body() string { return f.body }

// HasAnyField 任一字段存在即真。
func (f FrontMatter) HasAnyField(keys []string) bool { return len(unsupportedFields(f, keys)) > 0 }

// UnsupportedFields 返回实际出现但当前不生效的字段名。
func unsupportedFields(f FrontMatter, keys []string) []string {
	var out []string
	for _, k := range keys {
		if f.Get(k) != "" {
			out = append(out, k)
		}
	}
	return out
}

// camelToKebab 把 camelCase 键名转 kebab-case（disallowedTools → disallowed-tools）。
func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}