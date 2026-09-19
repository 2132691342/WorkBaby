// 文件：业务对外 HTTP method 白名单——工具调用与工作流节点仅允许 GET / POST，防 PUT/DELETE/PATCH 误用；
// Provider 适配层调上游 API 按各家协议放行，不受此约束。

package pkg

import "strings"

// AllowedMethods 业务对外 / 工具 / 工作流允许的 HTTP method 白名单（唯一真理源，禁止在 internal 内重复定义）。
var AllowedMethods = map[string]bool{
	"GET":  true,
	"POST": true,
}

// AllowedMethodList 用于 schema / 错误消息拼接（保持稳定顺序）。
var AllowedMethodList = []string{"GET", "POST"}

// AllowMethod 报告 method 是否在白名单内（大小写不敏感，去首尾空白）。
func AllowMethod(method string) bool {
	_, ok := NormalizeMethod(method)
	return ok
}

// NormalizeMethod trim + ToUpper；若在白名单返回 (规范 method, true)，否则 ("", false)。
func NormalizeMethod(method string) (string, bool) {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		return "", false
	}
	if AllowedMethods[m] {
		return m, true
	}
	return "", false
}

// AllowedMethodCSV 返回 "GET, POST"，用于错误消息拼接。
func AllowedMethodCSV() string {
	out := make([]string, 0, len(AllowedMethodList))
	for _, m := range AllowedMethodList {
		out = append(out, m)
	}
	return strings.Join(out, ", ")
}
