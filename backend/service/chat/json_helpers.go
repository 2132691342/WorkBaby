package chat

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/service/parser"
)

// asStrings 把 JSON 数组安全转成字符串切片。
func asStrings(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// stringOf 安全取字符串字段。
func stringOf(v any) string {
	if text, ok := v.(string); ok {
		return text
	}
	return ""
}

// defaultString 空串时用兜底值。
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// intOf 安全取整数字段；JSON 数字统一是 float64。
func intOf(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}

// floatOf 安全取浮点字段。
func floatOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

// contentText 把 `{content:[{type,text}]}` 形状拼成纯文本。
func contentText(v map[string]any) string {
	if v == nil {
		return ""
	}
	items, ok := v["content"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text := stringOf(block["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

// textOf 取消息正文的纯文本。
func textOf(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	items, ok := content.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if stringOf(block["type"]) == "text" {
			parts = append(parts, stringOf(block["text"]))
		}
	}
	return strings.Join(parts, "\n")
}

// parseDiffLines 复用会话解析里的 diff 切分：两处必须给界面同一种差异形状。
func parseDiffLines(text string) []domain.DiffLineVO {
	return parser.ParseDiff(text)
}

// filePathOf 从工具入参或结果详情里抠出被改动的文件路径。
func filePathOf(args any, result map[string]any) string {
	keys := []string{"file_path", "filePath", "path", "notebook_path"}
	if argMap, ok := args.(map[string]any); ok {
		for _, key := range keys {
			if value := stringOf(argMap[key]); value != "" {
				return value
			}
		}
	}
	if result != nil {
		if details, ok := result["details"].(map[string]any); ok {
			for _, key := range keys {
				if value := stringOf(details[key]); value != "" {
					return value
				}
			}
		}
	}
	return ""
}
