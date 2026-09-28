package pkg

import (
	"fmt"
	"strconv"
)

// fmtSprintf 是 fmt.Sprintf 的内部别名，供日志模块统一格式化入口。
func fmtSprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func itoa(n int) string { return strconv.Itoa(n) }
