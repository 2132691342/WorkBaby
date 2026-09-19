package service

import (
	"strconv"
	"strings"
)

// frontMatter 极简 frontmatter：只支持顶层 `key: value`（值可为逗号 / 空格分隔的列表）。
//
// 命令文件与 Agent 定义文件只需要 6~8 个平铺字段，不支持嵌套 / 多行块 / YAML 锚点；
// 为读这几个字段引入完整 YAML 解析器代价不成比例，且会让「写坏的字段」静默变成零值。
type frontMatter struct {
	fields map[string]string
	body   string
}

// splitFrontMatter 拆分 frontmatter 与正文；无 frontmatter 时 fields 为空、body 为原文。
func splitFrontMatter(s string) frontMatter {
	out := frontMatter{fields: map[string]string{}, body: strings.TrimSpace(s)}
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
func (f frontMatter) get(key string) string {
	if v := f.fields[strings.ToLower(key)]; v != "" {
		return v
	}
	// camelCase → kebab-case 回退：disallowedTools / disallowed-tools 两种写法都认
	kebab := camelToKebab(key)
	return f.fields[kebab]
}

// list 读取列表字段：逗号或空白分隔，兼容 `[a, b]` 写法。
func (f frontMatter) list(key string) []string {
	raw := strings.Trim(f.get(key), "[]")
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
func (f frontMatter) boolean(key string, def bool) bool {
	switch strings.ToLower(f.get(key)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	}
	return def
}

// intValue 整数字段：缺失或不可解析返回 def。
func (f frontMatter) intValue(key string, def int) int {
	v := f.get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
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
