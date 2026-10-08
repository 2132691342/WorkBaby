// 压缩与协议清洗链路：CleanForProtocol 的输出必须能直发上游，Compact 的切点
// 必须落在完整 turn 边界上——这是协议硬约束，不是优化目标。
package agent

import (
	"strings"
	"testing"

	"WorkBaby/backend/llm"
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

// assertNoOrphanToolResults 断言每条工具结果都落在「最近一条 assistant 的声明窗口」里。
// 上游只认紧邻配对：结果与声明之间隔着另一条 assistant 或 user 消息都会被 400
// （tool result's tool id not found），「前面某处声明过」不算数。
func assertNoOrphanToolResults(t *testing.T, msgs []llm.Message, label string) {
	t.Helper()
	open := map[string]bool{}
	for i, m := range msgs {
		switch m.Role {
		case llm.RoleAssistant:
			open = map[string]bool{}
			for _, tc := range m.ToolCalls {
				open[tc.ID] = true
			}
		case llm.RoleTool:
			if !open[m.ToolCallID] {
				t.Fatalf("%s：第 %d 条工具结果 %q 不在最近一条 assistant 的声明里，上游会 400",
					label, i, m.ToolCallID)
			}
		default:
			open = map[string]bool{}
		}
	}
}

func TestCompactProtocol(t *testing.T) {
	t.Run("清洗剔除空 assistant 与孤儿结果", func(t *testing.T) {
		in := []llm.Message{
			{Role: llm.RoleUser, Content: "在吗"},
			{Role: llm.RoleAssistant},
			{Role: llm.RoleTool, ToolCallID: "gone", Content: "孤儿结果"},
		}
		out := CleanForProtocol(in)
		if len(out) != 1 || out[0].Content != "在吗" {
			t.Fatalf("空 assistant 与孤儿结果应被剔除: %+v", out)
		}
	})

	// 并行工具的声明若被逐条落库，链上会留下 A(c1) → A(c2) → T(c1) → T(c2)：
	// T(c1) 与它的声明之间隔着 A(c2)，上游以 tool result's tool id not found 拒绝。
	// 相邻 assistant 本就是同一轮的产物，清洗必须把它们合并回一条。
	t.Run("拆散的并行声明合并为一条", func(t *testing.T) {
		in := []llm.Message{
			{Role: llm.RoleUser, Content: "两件事一起做"},
			callAssistant("c1"),
			callAssistant("c2"),
			toolResult("c1", "结果一"),
			toolResult("c2", "结果二"),
		}
		out := CleanForProtocol(in)
		if len(out) != 4 {
			t.Fatalf("相邻 assistant 应合并成一条，实际 %d 条", len(out))
		}
		if out[1].Role != llm.RoleAssistant || len(out[1].ToolCalls) != 2 {
			t.Fatalf("合并后的声明应带全部调用: %+v", out[1])
		}
		assertNoOrphanToolResults(t, out, "CleanForProtocol")
	})

	// keepTokens 从 0 扫到大，让切点落在每一个可能的位置上，结果都必须协议合法。
	t.Run("压缩永不孤儿化工具结果", func(t *testing.T) {
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
		for keep := 0; keep <= 4000; keep += 37 {
			out, _ := Compact(base, Budget{Window: 1, Reserve: 0, Keep: keep})
			assertNoOrphanToolResults(t, out, "Compact")
		}
	})

	// 保留侧的第一条必须是 user：切在回合中间会把半截 turn 发给上游。
	t.Run("压缩整轮丢弃", func(t *testing.T) {
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
			if out[0].Role != llm.RoleUser {
				t.Fatalf("keep=%d：保留侧第一条是 %q，turn 被从中间切开了", keep, out[0].Role)
			}
		}
	})

	// 预算判定与降级：没超不动、切不出完整 turn 就放弃、降级只留最近整轮。
	// 三条都是「发残缺上下文就一定被拒」的兜底，合成一条扫一次边界。
	t.Run("预算边界与降级", func(t *testing.T) {
		small := []llm.Message{
			{Role: llm.RoleUser, Content: "一句"},
			{Role: llm.RoleAssistant, Content: "一句"},
		}
		if out, after := Compact(small, Budget{Window: 100000, Reserve: 1000, Keep: 500}); len(out) != 2 || after == 0 {
			t.Fatalf("没超预算就不该动: %+v", out)
		}
		tight := []llm.Message{
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
			{Role: llm.RoleAssistant, Content: "很长很长很长很长很长很长很长很长的回答"},
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
		}
		if out, _ := Compact(tight, Budget{Window: 10, Reserve: 1, Keep: 1}); len(out) != len(tight) {
			t.Fatalf("切点不足时应原样返回，实际裁成 %d 条", len(out))
		}
		if out := TruncateDeterministic(tight, 1); len(out) != 1 || out[0].Content != tight[2].Content {
			t.Fatalf("降级截断应只保留最近的完整 turn: %+v", out)
		}
	})
}
