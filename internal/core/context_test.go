// 上下文装配与历史清洗测试：预算裁剪的优先级语义、上游 400 的协议硬约束。

package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"WorkBaby/internal/llm"
)

// TestBuildSystem 上下文装配：顺序稳定、超预算可解释地丢弃。
func TestBuildSystem(t *testing.T) {
	t.Run("超预算丢弃低优先级段并回传段名", func(t *testing.T) {
		body, dropped := BuildSystem([]Section{
			{Title: "人格", Body: "PERSONA", Order: OrderPersona},
			{Title: "知识库", Body: "KNOWLEDGE", Order: OrderKnowledge},
			{Title: "技能", Body: "SKILL", Order: OrderSkill},
		}, 20)

		assert.Contains(t, body, "PERSONA", "高优先级段永不丢")
		assert.NotContains(t, body, "SKILL")
		assert.Equal(t, []string{"技能", "知识库"}, dropped, "被丢段按裁剪优先级降序回传")
	})

	t.Run("常驻段永不丢且回传用 Key", func(t *testing.T) {
		body, dropped := BuildSystem([]Section{
			{Key: "persona", Title: "角色", Body: "PERSONA", Order: OrderPersona, Priority: PriorityEssential},
			{Key: "memory", Title: "本会话长期记忆", Body: strings.Repeat("m", 500), Order: OrderMemory},
		}, 10)

		assert.Contains(t, body, "PERSONA", "常驻段即使超预算也不丢")
		assert.Equal(t, []string{"memory"}, dropped, "段标识优先取 Key，便于前端对上号")
	})
}

// TestRebuildHistory 历史清洗的硬约束：不制造孤儿 tool，也不留下空 assistant 占位。
func TestRebuildHistory(t *testing.T) {
	t.Run("剔除空 assistant 占位与孤儿 tool", func(t *testing.T) {
		msgs := []*llm.Message{
			llm.SystemMessage("sys"),
			llm.AssistantMessage("", nil), // 空占位
			llm.AssistantMessage("调用", []llm.ToolCall{
				{ID: "t1", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: "{}"}},
			}),
			llm.ToolMessage("t1", "file_read", "AAA"),
			llm.ToolMessage("t9", "ghost", "孤儿"), // 无前置 assistant 配对
		}

		out := RebuildHistory(msgs)
		assert.Len(t, out, 3, "五条中剔除空 assistant 与孤儿 tool 各一条")
		for _, m := range out {
			assert.NotEqual(t, "t9", m.ToolCallID, "孤儿 tool 结果会导致上游 400")
			if m.Role == llm.RoleAssistant {
				assert.NotEmpty(t, m.Content)
			}
		}
	})

	t.Run("空 tool 结果补占位而非剔除", func(t *testing.T) {
		msgs := []*llm.Message{
			llm.AssistantMessage("调用", []llm.ToolCall{
				{ID: "t1", Type: "function", Function: llm.FunctionCall{Name: "exec", Arguments: "{}"}},
			}),
			llm.ToolMessage("t1", "exec", ""),
		}

		out := RebuildHistory(msgs)
		assert.Len(t, out, 2, "剔除会让 tool_calls 失去配对，只能补占位")
		assert.Equal(t, "(empty)", out[1].Content)
	})
}
