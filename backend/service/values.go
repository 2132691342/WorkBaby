package service

import (
	"encoding/json"
	"os"
	"sort"

	"WorkBaby/backend/domain"
)

// stringOf 安全取字符串；非字符串给空串。
func stringOf(v any) string {
	text, _ := v.(string)
	return text
}

// asStringSlice 把任意形状的数组解成字符串数组。
// 上游与本应用的落盘值都可能是 nil / []any / []string，统一收口成一个形状。
func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

// toAnySlice 把字符串数组转成 []any（写 JSON 时用）。
func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// removeAllQuiet 删除目录或文件；不存在不算错误。
func removeAllQuiet(path string) error {
	if path == "" || !existsQuiet(path) {
		return nil
	}
	return os.RemoveAll(path)
}

// existsQuiet 判断路径存在。
func existsQuiet(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ParseDiffLines 把统一差异文本拆成差异行。
// 与会话回放共用同一套形状：两处给界面的差异必须是同一种。
func ParseDiffLines(text string) []domain.DiffLineVO {
	return parseDiffShared(text)
}

// decodeJSON 把一个 JSON 串解成通用形状；解不开给空 map。
func decodeJSON(raw string) map[string]any {
	out := map[string]any{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// sortedKeys 取 map 的有序键：清单类输出要稳定，否则界面每刷新一次顺序就跳。
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
