// 上下文清洗与压缩：发模型前保证协议合法，并按 token 预算裁剪历史。
package agent

import (
	"strings"
	"unicode"

	"WorkBaby/backend/llm"
)

// 估算参数：CJK 按字计，其余按 4 字符 1 token，再加每条消息的结构开销。
const (
	perMessageOverhead = 4
	charsPerToken      = 4
)

// EstimateTokens 粗估上下文 token 数：不追求精确，只用于判断是否该压缩。
func EstimateTokens(system string, msgs []llm.Message) int {
	total := len(system)/charsPerToken + perMessageOverhead
	for _, m := range msgs {
		total += perMessageOverhead
		total += countTokens(m.Thinking)
		total += countTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += countTokens(tc.Name) + 8
		}
	}
	return total
}

func countTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			cjk++
		}
	}
	ascii := len(s) - cjk
	if ascii < 0 {
		ascii = 0
	}
	return cjk + ascii/charsPerToken
}

// CleanForProtocol 在调上游前清洗消息，守住两个协议硬约束：
// assistant 不能空着上阵；tool_calls 与 tool 结果必须严格配对。
func CleanForProtocol(msgs []llm.Message) []llm.Message {
	msgs = mergeConsecutiveAssistants(msgs)
	declared := map[string]bool{}
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant {
			for _, tc := range m.ToolCalls {
				declared[tc.ID] = true
			}
		}
	}
	answered := map[string]bool{}
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			answered[m.ToolCallID] = true
		}
	}

	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleAssistant:
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Thinking) == "" && len(m.ToolCalls) == 0 {
				continue
			}
			out = append(out, m)
			for _, tc := range m.ToolCalls {
				if !answered[tc.ID] {
					out = append(out, llm.Message{
						Role: llm.RoleTool, ToolCallID: tc.ID,
						Content: "（这个工具没有执行）", IsError: true,
					})
				}
			}
		case llm.RoleTool:
			// 没有 assistant 声明过的孤儿结果直接丢弃，比补一条假声明更安全。
			if declared[m.ToolCallID] {
				out = append(out, m)
			}
		default:
			out = append(out, m)
		}
	}
	return out
}

// mergeConsecutiveAssistants 合并相邻的 assistant 消息。
// T(c1) 与它的声明之间隔着另一条 assistant 时上游直接 400，而相邻 assistant 本就是同一轮的产物。
func mergeConsecutiveAssistants(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant && len(out) > 0 && out[len(out)-1].Role == llm.RoleAssistant {
			last := &out[len(out)-1]
			last.Content = joinText(last.Content, m.Content)
			last.Thinking = joinText(last.Thinking, m.Thinking)
			last.ToolCalls = append(last.ToolCalls, m.ToolCalls...)
			continue
		}
		out = append(out, m)
	}
	return out
}

// joinText 拼接两段文本，空串不产生多余换行。
func joinText(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n" + b
}

// FindCutPoint 找压缩切点：优先落在 user 消息处，保证 assistant(tool_calls) 与其结果不被拆散。
// 返回切点下标（切点之前的消息可以被摘要替代）。
func FindCutPoint(msgs []llm.Message, keepTokens int) int {
	if len(msgs) == 0 {
		return 0
	}
	used := 0
	cut := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		used += EstimateTokens("", []llm.Message{msgs[i]})
		if used > keepTokens {
			break
		}
		if msgs[i].Role == llm.RoleUser {
			cut = i
		}
	}
	if cut >= len(msgs) {
		cut = len(msgs)
	}
	return cut
}

// TruncateDeterministic 是预算太紧时的降级：只保留最近 keep 条，并切成完整的 turn。
func TruncateDeterministic(msgs []llm.Message, keep int) []llm.Message {
	if keep <= 0 {
		keep = 6
	}
	if len(msgs) <= keep {
		return msgs
	}
	start := len(msgs) - keep
	for start < len(msgs) && msgs[start].Role != llm.RoleUser {
		start++
	}
	if start >= len(msgs) {
		start = len(msgs) - keep
	}
	return CleanForProtocol(msgs[start:])
}

// Compact 是每轮发送前的必经之路：超预算就裁一刀，裁不出完整 turn 就原样返回。
// 纯函数、无 IO、不调模型；预算必须含 system 提示词，漏算会让判断偏乐观。
func Compact(msgs []llm.Message, b Budget) ([]llm.Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}
	before := b.SystemTokens + EstimateTokens("", msgs)
	budget := b.Window - b.Reserve
	if budget <= 0 {
		budget = b.Window / 2
	}
	if b.Window <= 0 || before <= budget {
		return msgs, before
	}
	keep := b.Keep
	if keep <= 0 || keep > budget {
		keep = budget
	}

	cleaned := CleanForProtocol(msgs)
	cut := FindCutPoint(cleaned, keep)
	// 切不出一个完整 turn 时宁可不裁：发残缺上下文给上游一定被拒。
	if cut <= 1 || cut >= len(cleaned) {
		return msgs, before
	}
	out := CleanForProtocol(cleaned[cut:])
	return out, EstimateTokens("", out)
}
