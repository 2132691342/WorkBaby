package agent

import (
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// CompressInfo 压缩证据，随 EventCompressed 下发，让用户看得见发生了什么。
type CompressInfo struct {
	Removed   int    // 被折叠的历史条数
	Summary   string // 交接摘要；确定性折叠为空
	Truncated bool   // 是否按轮截断
}

// MicroCompressor 确定性折叠器（不调 LLM）：折叠最旧的「assistant(tool_calls) +
// 连续 tool 结果」整段，仍超预算则按轮裁剪。绝不制造孤儿 tool 或空 assistant。
type MicroCompressor struct{}

// collapsedMarker 已折叠标记。折叠不改结构（tool_calls 与结果都要保留配对），
// 因此必须靠它识别「这段已折叠过」，否则同一段会被反复折叠而占用不再下降。
const collapsedMarker = "（该轮工具调用已折叠）"

// Compress 实现 Compressor。两个裁剪循环按「本段省下的 token 增量」维护剩余预算：
// 每轮全量重算会让长会话压缩退化成 O(n²)，而压缩发生在每轮请求前。
func (MicroCompressor) Compress(msgs []*llm.Message, budget int) ([]*llm.Message, CompressInfo) {
	total := totalTokens(msgs)
	if budget <= 0 || total <= budget {
		return msgs, CompressInfo{}
	}
	out := cloneMessages(msgs)
	info := CompressInfo{}

	// 折叠最旧的 tool 段；无段可折或折叠不再降低占用即停（防无进展死循环）。
	for total > budget {
		i := oldestToolSegment(out)
		if i < 0 {
			break
		}
		n, saved := collapseSegment(out, i)
		info.Removed += n
		if saved <= 0 {
			break
		}
		total -= saved
	}
	// 仍超预算则逐条裁掉最旧一轮的 user 锚点；最后一轮永不丢。
	for total > budget {
		i := oldestCompleteTurn(out)
		if i < 0 {
			break
		}
		total -= totalTokens(out[i : i+1])
		out = append(out[:i], out[i+1:]...)
		info.Removed++
		info.Truncated = true
	}
	return out, info
}

// oldestToolSegment 找最旧的未折叠「assistant(带 tool_calls) + 其后连续 tool」段起点；无则 -1。
func oldestToolSegment(msgs []*llm.Message) int {
	for i, m := range msgs {
		if m.Role != llm.RoleAssistant || len(m.ToolCalls) == 0 || m.Content == collapsedMarker {
			continue
		}
		if i+1 < len(msgs) && msgs[i+1].Role == llm.RoleTool {
			return i
		}
	}
	return -1
}

// collapseSegment 就地折叠第 i 段：tool_calls 与 ID 锚点保留（配对完整），正文换成占位。
// 返回被折叠的条数与节省的 token 数；节省为 0 表示这一段已无可压缩空间。
func collapseSegment(msgs []*llm.Message, i int) (int, int) {
	before := totalTokens(msgs[i : i+1])

	head := *msgs[i]
	head.Content = collapsedMarker
	msgs[i] = &head

	n := 0
	j := i + 1
	for ; j < len(msgs) && msgs[j].Role == llm.RoleTool; j++ {
		before += totalTokens(msgs[j : j+1])
		cp := *msgs[j]
		cp.Content = "（已消费，勿重跑）" + msgs[j].ToolName
		msgs[j] = &cp
		n++
	}
	after := totalTokens(msgs[i:j])
	return n, before - after
}

// oldestCompleteTurn 找最旧一轮的起点（system 之后的第一条 user）；不足两轮时返回 -1。
// 最后一轮永不丢——返回 -1 即停止裁剪。
func oldestCompleteTurn(msgs []*llm.Message) int {
	first := -1
	for i, m := range msgs {
		if m.Role == llm.RoleUser {
			if first < 0 {
				first = i
				continue
			}
			return first // 存在第二条 user，说明删除 first 这一轮后仍有内容
		}
	}
	return -1
}

// totalTokens 粗估消息序列 token 数。
func totalTokens(msgs []*llm.Message) int {
	n := 0
	for _, m := range msgs {
		n += pkg.EstimateTextTokens(m.Content) + pkg.EstimateTextTokens(m.Thinking)
		for _, c := range m.ToolCalls {
			n += pkg.EstimateTextTokens(c.Function.Name) + pkg.EstimateTextTokens(c.Function.Arguments)
		}
	}
	return n
}

// cloneMessages 浅拷贝序列；折叠时对被改动的单条再深拷贝，避免污染调用方切片。
func cloneMessages(msgs []*llm.Message) []*llm.Message {
	out := make([]*llm.Message, len(msgs))
	copy(out, msgs)
	return out
}

// addUsage 累加用量。llm.TokenUsage 未自带 Add，内核内部收敛一处。
func addUsage(a, b llm.TokenUsage) llm.TokenUsage {
	return llm.TokenUsage{
		InputTokens:      a.InputTokens + b.InputTokens,
		OutputTokens:     a.OutputTokens + b.OutputTokens,
		CacheReadTokens:  a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
	}
}
