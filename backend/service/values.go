package service

// stringOf 安全取字符串；非字符串给空串。
func stringOf(v any) string {
	text, _ := v.(string)
	return text
}
