package agent

import (
	"fmt"
)

// tidyPolicy 是工具输出折叠的策略。
// 大工具输出（几百行的文件内容、几万字的日志）是上下文的主要消耗：
// 模型读到的往往只需要头尾各一部分，中间对它没用。
type tidyPolicy struct {
	// enabled 是否开启折叠
	enabled bool
	// maxChars 单条输出进上下文的字符上限
	maxChars int
	// headChars 保留的开头字符数
	headChars int
	// tailChars 保留的结尾字符数
	tailChars int
}

// 默认策略：单条 4000 字符进上下文，头 3000 尾 1000。
// 头留得多是因为报错信息与函数签名通常在前面。
const (
	defaultTidyMaxChars  = 4000
	defaultTidyHeadChars = 3000
	defaultTidyTailChars = 1000
)

// newTidyPolicy 造一份默认策略。
func newTidyPolicy() *tidyPolicy {
	return &tidyPolicy{
		enabled:   true,
		maxChars:  defaultTidyMaxChars,
		headChars: defaultTidyHeadChars,
		tailChars: defaultTidyTailChars,
	}
}

// tidyResult 是折叠后的结果。
type tidyResult struct {
	// Text 进上下文的文本
	Text string
	// Truncated 是否发生了折叠
	Truncated bool
	// OriginalChars 折叠前的字符数
	OriginalChars int
}

// Fold 按策略折叠一段输出。
// 折叠只在「超出上限」时发生：短输出原样保留，
// 折叠过的输出对模型来说会少掉细节，短内容不该被切。
func (p *tidyPolicy) Fold(text string) tidyResult {
	if p == nil || !p.enabled {
		return tidyResult{Text: text, OriginalChars: len([]rune(text))}
	}
	runes := []rune(text)
	if len(runes) <= p.maxChars {
		return tidyResult{Text: text, OriginalChars: len(runes)}
	}
	head := runes[:p.headChars]
	tail := runes[len(runes)-p.tailChars:]
	omitted := len(runes) - p.headChars - p.tailChars
	marker := fmt.Sprintf("\n\n...（中间 %d 字已折叠，完整内容在工具结果里，需要时用 read 或 grep 去看）...\n\n", omitted)
	return tidyResult{
		Text:          string(head) + marker + string(tail),
		Truncated:     true,
		OriginalChars: len(runes),
	}
}

// SetTidyEnabled 开关工具输出折叠。
// 关掉之后大输出会原样进上下文：用户想要「模型看到全部」时可以这么设。
func (r *Registry) SetTidyEnabled(enabled bool) {
	if r.TidyPolicy == nil {
		r.TidyPolicy = newTidyPolicy()
	}
	r.TidyPolicy.enabled = enabled
}

// Tidy 折叠一段工具输出的正文。
// 完整内容仍留在会话文件里：折叠只是不给模型看，不是丢掉。
func (r *Registry) Tidy(text string) (string, bool, int) {
	result := r.TidyPolicy.Fold(text)
	return result.Text, result.Truncated, result.OriginalChars
}
