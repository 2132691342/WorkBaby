// 上下文组装：从会话条目取消息、折叠工具全文、按窗口裁剪并修复工具配对。
package agent

import (
	"WorkBaby/backend/domain"
)

// history 把会话条目里的消息按顺序取出来，组成模型上下文。
// 活动分支上最后一条带摘要的压缩条目是分界点：之前的折成一条摘要引导消息，
// 之后的消息原样带上；没有摘要的旧压缩标记没有可用结果，只能跳过。
func (r *Runner) history() []Message {
	entries := r.Session.Entries()
	// 先找分界点：最后一条带摘要的压缩条目。
	cut := -1
	summary := ""
	for i, entry := range entries {
		if entry.EntryType == domain.SessionEntryCompaction && entry.EntrySummary != "" {
			cut = i
			summary = entry.EntrySummary
		}
	}
	out := make([]Message, 0, len(entries)+1)
	if cut >= 0 {
		out = append(out, Message{
			Role:   RoleUser,
			Blocks: []Block{{Kind: KindText, Text: summaryMessage(summary)}},
		})
	}
	for i, entry := range entries {
		if cut >= 0 && i <= cut {
			// 压缩点之前（含压缩条目本身）不进上下文，真相仍在文件里。
			continue
		}
		if entry.EntryType != domain.SessionEntryMessage || entry.EntryMessage == nil {
			continue
		}
		if entry.EntryMessage.MessageRole == string(RoleSystem) {
			continue
		}
		msg := MessageFromDO(entry.EntryMessage)
		if msg.Role == RoleTool {
			// 全文在会话文件里，进上下文的才折叠：几万字日志不该每轮都付一遍 token。
			if folded, _, _ := r.Registry.Tidy(msg.Text()); folded != msg.Text() {
				msg.Blocks = []Block{{Kind: KindText, Text: folded}}
			}
		}
		out = append(out, msg)
	}
	return out
}

// summaryMessage 组装摘要引导消息的正文。
func summaryMessage(summary string) string {
	return "以下是此前对话的摘要，之后的对话以它为背景继续：\n\n" + summary
}

// trimToWindow 在上下文超出模型窗口时丢掉最早的消息。
// 始终留着最后一条：那是用户刚说的那句话，丢了模型就没法回答。
func trimToWindow(messages []Message, contextWindow int, systemChars int) []Message {
	if contextWindow <= 0 || len(messages) == 0 {
		return messages
	}
	budget := contextWindow - systemChars/3 - 1024
	if budget <= 0 {
		budget = contextWindow / 2
	}
	total := 0
	for _, msg := range messages {
		total += estimateTokens(msg)
	}
	if total <= budget {
		return repairToolPairs(messages)
	}
	// 从最早的消息开始丢，但始终留着最后一条（刚刚说的那句话）。
	cut := 0
	for cut < len(messages)-1 && total > budget {
		total -= estimateTokens(messages[cut])
		cut++
	}
	return repairToolPairs(messages[cut:])
}

// repairToolPairs 补齐工具调用与结果的配对：裁历史或中止都可能留下缺口，
// 严格端点对「孤儿 tool 消息 / 悬空 tool_call」一律整条请求拒掉。
func repairToolPairs(messages []Message) []Message {
	answered := map[string]bool{}
	hosted := map[string]bool{}
	for _, msg := range messages {
		if msg.Role == RoleTool && msg.ToolCallId != "" {
			answered[msg.ToolCallId] = true
		}
		for _, call := range msg.ToolCalls() {
			if call.CallId != "" {
				hosted[call.CallId] = true
			}
		}
	}
	out := make([]Message, 0, len(messages)+2)
	for _, msg := range messages {
		if msg.Role == RoleTool {
			// 宿主 assistant 被裁掉的 tool 结果只能丢：留着就是孤儿。
			if msg.ToolCallId == "" || !hosted[msg.ToolCallId] {
				continue
			}
			out = append(out, msg)
			continue
		}
		out = append(out, msg)
		if msg.Role != RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls() {
			if call.CallId == "" || answered[call.CallId] {
				continue
			}
			answered[call.CallId] = true
			out = append(out, Message{
				Role:       RoleTool,
				ToolCallId: call.CallId,
				ToolName:   call.ToolName,
				IsError:    true,
				Blocks:     []Block{{Kind: KindText, Text: "这次调用没有留下结果（已被中止或裁掉），如仍需要请重新执行。"}},
			})
		}
	}
	return out
}

// estimateTokens 粗估一条消息的 token 数。
// 精确计数要按模型分词器来，这里只需要一个「是否超窗」的判断：
// 中文约 1.5 字符/token，英文约 4 字符/token，取中间偏保守的 3。
func estimateTokens(msg Message) int {
	chars := 0
	for _, block := range msg.Blocks {
		chars += len([]rune(block.Text))
		chars += len(block.ToolArgs)
		chars += len(block.ImageBase64)
	}
	return chars/3 + 8
}
