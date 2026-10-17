// 用量与运行记录的共同形态：token 口径、运行状态与检查点位点。
package domain

// UsageVO 是一次模型调用的用量读数。
// 字段直接采用 PI 的口径：它已按协议归一，WorkBaby 不再二次换算。
type UsageVO struct {
	// Input 是这次请求发出去的全部输入（含命中缓存的部分）。
	Input  int `json:"input"`
	Output int `json:"output"`
	// Cached 是 Input 里命中上游缓存的部分，恒 ≤ Input；命中率由它除以 Input 得出。
	Cached int `json:"cached"`
	Total  int `json:"total"`
	// Cost 是上游按模型单价算出的美元成本；模型未声明价格时为 0（界面据此隐藏钱卡片）。
	Cost float64 `json:"cost"`
	// Context 是这一轮发出去时上下文占了多少窗口。
	Context   int   `json:"context"`
	LatencyMs int64 `json:"latency_ms"`
}

// CacheHitRate 返回 0-100 的缓存命中率；分母为 0 时返回 0。
func (u UsageVO) CacheHitRate() int {
	if u.Input <= 0 {
		return 0
	}
	r := u.Cached * 100 / u.Input
	if r > 100 {
		return 100
	}
	return r
}
