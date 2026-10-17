// 服务层内部共用件：时间戳、字符串数组编解码、工具名与角色规整。
package service

import (
	"encoding/json"
	"strings"
)

// decodeStringSlice 解析存成 JSON 串的字符串数组；坏 JSON 按空处理。
func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// encodeStringSlice 把字符串切片编成 JSON 列；空切片给空串（不占空间）。
func encodeStringSlice(list []string) string {
	if len(list) == 0 {
		return ""
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(raw)
}

// firstLine 取首行并截断，供日志记录失败原因。
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}
