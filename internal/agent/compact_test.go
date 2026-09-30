// 上下文清洗的协议硬约束与压缩策略。
// CleanForProtocol 的输出必须能直发上游（空消息 / 孤儿结果 / 未配对调用都在这一层兜底）；
// Compact 是纯函数，只在超预算时才动手。
package agent

import (
	"testing"

	"WorkBaby/internal/llm"
)

func TestCleanForProtocolHardConstraints(t *testing.T) {
	t.Run("剔除空 assistant 与孤儿结果", func(t *testing.T) {
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

	// 模型在流中途断掉时 tool_calls 就没有对应结果，补一条错误结果才不会让上游拒收。
	t.Run("补齐未配对的工具调用", func(t *testing.T) {
		in := []llm.Message{
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read"}}},
		}
		out := CleanForProtocol(in)
		if len(out) != 2 {
			t.Fatalf("未配对的工具调用必须补一条错误结果，实际 %d 条", len(out))
		}
		if out[1].Role != llm.RoleTool || out[1].ToolCallID != "c1" || !out[1].IsError {
			t.Fatalf("补的结果形态不对: %+v", out[1])
		}
	})
}

// 降级路径：只保留最近的完整 turn，不能切出残缺的 user/assistant 配对。
func TestTruncateDeterministicKeepsRecentTurns(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "旧1"},
		{Role: llm.RoleUser, Content: "旧2"},
		{Role: llm.RoleUser, Content: "新1"},
	}
	out := TruncateDeterministic(msgs, 1)
	if len(out) != 1 || out[0].Content != "新1" {
		t.Fatalf("降级截断应只保留最近的完整 turn: %+v", out)
	}
}

func TestCompactBudgetPolicy(t *testing.T) {
	t.Run("没超预算就不动", func(t *testing.T) {
		msgs := []llm.Message{
			{Role: llm.RoleUser, Content: "一句"},
			{Role: llm.RoleAssistant, Content: "一句"},
		}
		out, after := Compact(msgs, Budget{Window: 100000, Reserve: 1000, Keep: 500})
		if len(out) != 2 || after == 0 {
			t.Fatalf("没超预算就不该动: %+v", out)
		}
	})

	// 切不出一个完整 turn 时宁可原样返回：发残缺上下文给上游一定被拒。
	t.Run("切点不足时原样返回", func(t *testing.T) {
		msgs := []llm.Message{
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
			{Role: llm.RoleAssistant, Content: "很长很长很长很长很长很长很长很长的回答"},
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
		}
		out, _ := Compact(msgs, Budget{Window: 10, Reserve: 1, Keep: 1})
		if len(out) != len(msgs) {
			t.Fatalf("切点不足时应原样返回，实际裁成 %d 条", len(out))
		}
	})
}
