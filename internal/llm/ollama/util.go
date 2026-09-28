package ollama

import (
	"strconv"

	"WorkBaby/internal/pkg"
)

// errString 把上游错误文本包成带 code 的 AppError，便于统一走 Err 字段。
func errString(s string) error { return pkg.New(3106, s, "") }

func itoa(n int) string { return strconv.Itoa(n) }
