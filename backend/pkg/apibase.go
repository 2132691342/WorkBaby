// OpenAI 兼容端点的地址拼接：探测与聊天共用同一套约定，带不带 /v1 都能用。
package pkg

import "strings"

// NormalizeApiBase 归一接口根地址：去首尾空白与末尾斜杠，其余原样保留。
// 用户可能填到 /v1 这一级，也可能只填域名——两种都合法，差异在拼路径时收敛。
func NormalizeApiBase(baseUrl string) string {
	return strings.TrimRight(strings.TrimSpace(baseUrl), "/")
}

// OpenAIEndpoint 拼一个 OpenAI 兼容端点（如 chat/completions、models）。
// base 已带 /v1 尾巴就不再补：否则按界面占位符填写会得到 /v1/v1/... 的必死地址。
func OpenAIEndpoint(baseUrl, path string) string {
	base := NormalizeApiBase(baseUrl)
	path = strings.TrimLeft(path, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/" + path
	}
	return base + "/v1/" + path
}
