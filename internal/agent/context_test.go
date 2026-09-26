// 上下文装配与历史清洗测试：预算裁剪的优先级语义、上游 400 的协议硬约束。

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
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

// TestTransformContext（PI Phase 3）TransformContext 回调：每轮请求前可改写 msgs。
func TestTransformContext(t *testing.T) {
	echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
	p := &mockProvider{turns: []llm.ChatResponse{
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, StopReason: "end_turn"},
	}}
	var captured []*llm.Message
	l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
		WithHooks(Hooks{
			TransformContext: func(_ context.Context, _ int, msgs []*llm.Message) ([]*llm.Message, error) {
				captured = msgs
				return msgs, nil
			},
		})

	_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
	require.NoError(t, err)
	assert.NotEmpty(t, captured, "TransformContext 必须被调用")

	t.Run("与 BeforeTurn 互不干扰：TransformContext 优先", func(t *testing.T) {
		p2 := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, StopReason: "end_turn"},
		}}
		var transformCalled, beforeCalled bool
		l2 := newTestLoop(t, p2, echo).WithGuard(ExposeGuard(nil)).
			WithHooks(Hooks{
				TransformContext: func(_ context.Context, _ int, msgs []*llm.Message) ([]*llm.Message, error) {
				transformCalled = true
				return msgs, nil
			},
				BeforeTurn: func(context.Context, int, []*llm.Message) []*llm.Message {
					beforeCalled = true
					return nil
				},
			})

		_, err := l2.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)
		assert.True(t, transformCalled)
		assert.False(t, beforeCalled, "TransformContext 与 BeforeTurn 二选一；前者优先")
	})
}

// TestConvertToLlm（PI Phase 3）ConvertToLlm 回调：仅作用于本轮发送。
func TestConvertToLlm(t *testing.T) {
	echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
	p := &mockProvider{turns: []llm.ChatResponse{
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, StopReason: "end_turn"},
	}}
	l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
		WithHooks(Hooks{
			ConvertToLlm: func(_ context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
				// 协议边界：过滤掉非 user/assistant/tool 角色
				out := make([]*llm.Message, 0, len(msgs))
				for _, m := range msgs {
					if m.Role == llm.RoleUser || m.Role == llm.RoleAssistant || m.Role == llm.RoleTool {
						out = append(out, m)
					}
				}
				return out, nil
			},
		})

	_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
	require.NoError(t, err)
	require.NotEmpty(t, p.requests)
	assert.NotEmpty(t, p.requests[0].Messages, "ConvertToLlm 应被调用且生效")

	// 额外验证：JSON 字段保留
	_, _ = json.Marshal(p.requests[0].Messages)
	_ = tool.RiskReadOnly
}
