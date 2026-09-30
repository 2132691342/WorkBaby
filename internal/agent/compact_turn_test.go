// 压缩切点必须落在完整的 turn 边界上。
//
// 真实故障：切点落在一个 assistant(tool_calls) 之前，把「工具调用声明」丢进裁掉的
// 那一侧，而它的 tool 结果留在保留侧。上游收到一条找不到声明的 tool 消息，
// 直接 400：invalid params, tool result's tool id not found。
// 这条约束是协议硬约束，不是优化目标。
package agent

import (
	"strings"
	"testing"

	"WorkBaby/internal/llm"
)

// callAssistant 造一条带工具调用的助手消息。
func callAssistant(id string) llm.Message {
	return llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: id, Name: "ls"}},
	}
}

// toolResult 造一条对应的工具结果。
func toolResult(id, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: content}
}

// 切点之后不允许出现「结果在、声明不在」的孤儿 tool 消息——上游会直接拒收。
func assertNoOrphanToolResults(t *testing.T, msgs []llm.Message, label string) {
	t.Helper()
	declared := map[string]bool{}
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant {
			for _, tc := range m.ToolCalls {
				declared[tc.ID] = true
			}
		}
	}
	for _, m := range msgs {
		if m.Role == llm.RoleTool && !declared[m.ToolCallID] {
			t.Fatalf("%s：出现孤儿工具结果 %q，上游会返回 tool id not found", label, m.ToolCallID)
		}
	}
}

// 覆盖各种切点位置：无论 FindCutPoint 落在哪，结果都必须协议合法。
func TestCompactNeverOrphansToolResults(t *testing.T) {
	// 一段典型的多轮对话：user → assistant(tool) → tool → assistant → user → …
	base := []llm.Message{
		{Role: llm.RoleUser, Content: "看桌面"},
		callAssistant("c1"),
		toolResult("c1", strings.Repeat("桌面上有这些文件 ", 40)),
		{Role: llm.RoleAssistant, Content: "桌面有这些文件"},
		{Role: llm.RoleUser, Content: "看 skill 文件夹"},
		callAssistant("c2"),
		toolResult("c2", strings.Repeat("skills 目录里有 ", 40)),
		{Role: llm.RoleAssistant, Content: "skills 里有这些"},
	}

	// keepTokens 从 0 扫到大，让切点落在每一个可能的位置上。
	for keep := 0; keep <= 4000; keep += 37 {
		cut := FindCutPoint(base, keep)
		out, _ := Compact(base, Budget{Window: 1, Reserve: 0, Keep: keep})
		assertNoOrphanToolResults(t, out, "Compact")
		_ = cut
	}
}

// 切点必须整轮丢弃：保留下来的第一条不能是 tool 消息或带 tool_calls 的助手消息。
func TestCompactKeepsWholeTurns(t *testing.T) {
	base := []llm.Message{
		{Role: llm.RoleUser, Content: "看桌面"},
		callAssistant("c1"),
		toolResult("c1", strings.Repeat("x", 400)),
		{Role: llm.RoleAssistant, Content: "好的"},
		{Role: llm.RoleUser, Content: "再看看"},
		callAssistant("c2"),
		toolResult("c2", strings.Repeat("y", 400)),
		{Role: llm.RoleAssistant, Content: "搞定"},
	}
	for keep := 0; keep <= 2000; keep += 53 {
		out, _ := Compact(base, Budget{Window: 1, Reserve: 0, Keep: keep})
		if len(out) == 0 {
			continue
		}
		// 保留侧的第一条必须是 user：它才代表一个完整回合的起点。
		if out[0].Role != llm.RoleUser {
			t.Fatalf("keep=%d：保留侧第一条是 %q，turn 被从中间切开了", keep, out[0].Role)
		}
	}
}
