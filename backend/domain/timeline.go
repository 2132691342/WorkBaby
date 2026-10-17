// 执行时间线：一次 run 里工具动作与正文长度的时间账，随执行记录留存（specs/17 / 18 共用）。
package domain

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// 时间线步骤类型。
const (
	StepTool = "tool" // 一次工具调用
)

// RunStep 时间线上的一步：瀑布图的一根条。
type RunStep struct {
	Kind   string `json:"kind"`  // 见 StepTool
	Name   string `json:"name"`  // 工具名
	Label  string `json:"label"` // 界面显示名
	At     int64  `json:"at"`    // 开始时刻，毫秒
	Ms     int64  `json:"ms"`    // 耗时，毫秒
	OK     bool   `json:"ok"`    // false = 这一步失败或被拒绝
	Detail string `json:"detail"`
}

// RunTimeline 一次 run 的时间线。Chars 是助手正文总字符数——
// 纯文本角色一根条都没有，靠它表示「这一格确实产出了东西」。
type RunTimeline struct {
	Chars int       `json:"chars"`
	Steps []RunStep `json:"steps"`
}

// IsEmpty 时间线里没有任何可读信息。
func (t *RunTimeline) IsEmpty() bool {
	return t == nil || (t.Chars == 0 && len(t.Steps) == 0)
}

// EncodeRunTimeline 把时间线编码成一个 JSON 列的值。
func EncodeRunTimeline(t RunTimeline) string {
	raw, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(raw)
}

// DecodeRunTimeline 解析时间线列；空串或解析不了都返回 nil（界面按「没有时间线」渲染）。
func DecodeRunTimeline(raw string) *RunTimeline {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var t RunTimeline
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil
	}
	return &t
}

// TimelineDetailLimit 每步输出摘要的字符上限：时间线是索引，不是原文。
const TimelineDetailLimit = 300

// TrimTimelineDetail 截断一步的输出摘要（按 rune 截）。
func TrimTimelineDetail(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= TimelineDetailLimit {
		return s
	}
	r := []rune(s)
	return string(r[:TimelineDetailLimit]) + "…"
}
