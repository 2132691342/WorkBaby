package pkg

import "strings"

// AllowMethod 是业务 API 的方法白名单：只放行 GET 与 POST，杜绝误用 DELETE/PUT 造成语义漂移。
func AllowMethod(m string) bool {
	switch strings.ToUpper(m) {
	case "GET", "POST":
		return true
	default:
		return false
	}
}

// NormalizeMethod 把方法归一为大写，供日志与前端提示复用。
func NormalizeMethod(m string) string { return strings.ToUpper(m) }
