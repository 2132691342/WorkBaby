// 压缩与协议清洗链路：CleanForProtocol 的输出必须能直发上游，
// Compact 的切点必须落在完整 turn 边界上。切点把 assistant(tool_calls) 的声明
// 裁掉、结果留在保留侧，上游会直接 400（tool result's tool id not found）——
// 这是协议硬约束，不是优化目标。
package agent

import (
	"context"
	"strings"
	"testing"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/llmtest"
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

// assertNoOrphanToolResults 断言切点之后没有「结果在、声明不在」的孤儿 tool 消息。
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

	// 模型在流中途断掉时 tool_calls 就没有对应结果，补一条错误结果才不会让上游拒收。
	t.Run("清洗补齐未配对的工具调用", func(t *testing.T) {
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

	// 降级路径：只保留最近的完整 turn，不能切出残缺的 user/assistant 配对。
	t.Run("降级截断保留最近整轮", func(t *testing.T) {
		msgs := []llm.Message{
			{Role: llm.RoleUser, Content: "旧1"},
			{Role: llm.RoleUser, Content: "旧2"},
			{Role: llm.RoleUser, Content: "新1"},
		}
		out := TruncateDeterministic(msgs, 1)
		if len(out) != 1 || out[0].Content != "新1" {
			t.Fatalf("降级截断应只保留最近的完整 turn: %+v", out)
		}
	})
}

// 循环内真实裁剪：发请求前按预算裁，且裁完必须从 user 起头。
func TestLoopCompactsHistoryOverBudget(t *testing.T) {
	history := make([]llm.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history,
			llm.Message{Role: llm.RoleUser, Content: "历史问题"},
			llm.Message{Role: llm.RoleAssistant, Content: "历史回答，这里写很多字用来撑大 token 估算。"},
		)
	}
	streamer := llmtest.New(llm.Message{Content: "最后一句"})
	loop := New(Config{
		Streamer: streamer,
		Model:    "test",
		Budget:   Budget{Window: 1000, Reserve: 200, Keep: 200},
	}, history)

	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if len(res.Messages) >= len(history) {
		t.Fatalf("超预算却没有裁剪: %d -> %d", len(history), len(res.Messages))
	}
	if res.Messages[0].Role != llm.RoleUser {
		t.Fatal("裁剪后必须从 user 消息开始")
	}
	if streamer.Calls() == 0 {
		t.Fatal("裁剪后仍应发起请求")
	}
}
