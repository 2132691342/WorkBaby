// Package modelmeta 内置模型元数据目录：provider 行未声明上下文窗口 / 单价时的兜底缺省。
// 数据为嵌入的 catalog.json（近似公开缺省值，设置页可按模型覆盖）；lookup 按模型名
// 不区分大小写的子串匹配，多条命中取最长 match（更具体者优先），未命中返回零值。
package modelmeta

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Pricing 模型单价（USD / 百万 token）；零值 = 未配单价（上层不计费）。
type Pricing struct {
	InputPerM     float64
	OutputPerM    float64
	CacheReadPerM float64
}

//go:embed catalog.json
var catalogJSON []byte

type entry struct {
	Match         string   `json:"match"`
	ContextWindow int      `json:"context_window"`
	InputPerM     float64  `json:"input_per_m"`
	OutputPerM    float64  `json:"output_per_m"`
	CacheReadPerM *float64 `json:"cache_read_per_m"`
}

type catalog struct {
	Entries []entry `json:"entries"`
}

var parsed catalog

func init() {
	if err := json.Unmarshal(catalogJSON, &parsed); err != nil {
		panic("modelmeta: parse embedded catalog.json: " + err.Error())
	}
}

// best 命中最长 match 的条目；未命中返回 false。
func best(model string) (entry, bool) {
	if model == "" {
		return entry{}, false
	}
	lower := strings.ToLower(model)
	found, bestLen := entry{}, 0
	for _, e := range parsed.Entries {
		if e.Match != "" && strings.Contains(lower, strings.ToLower(e.Match)) && len(e.Match) > bestLen {
			found, bestLen = e, len(e.Match)
		}
	}
	return found, bestLen > 0
}

// ContextWindow 模型上下文窗口兜底；未命中返回 0（调用方继续走全局缺省）。
func ContextWindow(model string) int {
	if e, ok := best(model); ok {
		return e.ContextWindow
	}
	return 0
}

// PricingFor 模型单价兜底；未命中或条目未配单价返回零值（上层按未配单价不计费）。
func PricingFor(model string) Pricing {
	e, ok := best(model)
	if !ok {
		return Pricing{}
	}
	p := Pricing{InputPerM: e.InputPerM, OutputPerM: e.OutputPerM}
	if e.CacheReadPerM != nil {
		p.CacheReadPerM = *e.CacheReadPerM
	}
	return p
}
