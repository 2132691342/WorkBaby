package parser

import "time"

// ParseTime 把引擎侧的 ISO 时间串换成毫秒。
// 读进来第一件事就换成毫秒：ISO 串散布在 DTO 里会让「比较大小」变成字符串比较，
// 时区一变就全错。解析不出来给 0，界面按「未知」显示。
func ParseTime(text string) int64 {
	if text == "" {
		return 0
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
	} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
