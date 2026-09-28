// 覆盖上下文清洗的协议硬约束与压缩切点选择。
package agent

import (
	"testing"

	"WorkBaby/internal/llm"
)

func TestCleanForProtocolDropsEmptyAssistantAndOrphans(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: "在吗"},
		{Role: llm.RoleAssistant},
		{Role: llm.RoleTool, ToolCallID: "gone", Content: "孤儿结果"},
	}
	out := CleanForProtocol(in)
	if len(out) != 1 || out[0].Content != "在吗" {
		t.Fatalf("空 assistant 与孤儿结果应被剔除: %+v", out)
	}
}

func TestCleanForProtocolPairsUnansweredToolCalls(t *testing.T) {
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
}

func TestFindCutPointPrefersUserBoundary(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "第一句"},
		{Role: llm.RoleAssistant, Content: "回应"},
		{Role: llm.RoleUser, Content: "第二句"},
		{Role: llm.RoleAssistant, Content: "再回应"},
	}
	cut := FindCutPoint(msgs, 30)
	if cut != 2 {
		t.Fatalf("切点应落在第二个 user 处，实际 %d", cut)
	}
}

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

func TestCompactKeepsWithinBudgetUntouched(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "一句"},
		{Role: llm.RoleAssistant, Content: "一句"},
	}
	out, after := Compact(msgs, Budget{Window: 100000, Reserve: 1000, Keep: 500})
	if len(out) != 2 || after == 0 {
		t.Fatalf("没超预算就不该动: %+v", out)
	}
}

// 切不出一个完整 turn 时宁可原样返回：发残缺上下文给上游一定被拒。
func TestCompactGivesUpWhenCutTooSmall(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
		{Role: llm.RoleAssistant, Content: "很长很长很长很长很长很长很长很长的回答"},
		{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
	}
	out, _ := Compact(msgs, Budget{Window: 10, Reserve: 1, Keep: 1})
	if len(out) != len(msgs) {
		t.Fatalf("切点不足时应原样返回，实际裁成 %d 条", len(out))
	}
}
