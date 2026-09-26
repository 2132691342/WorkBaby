// 主循环测试：多轮 ReAct、只读并行、终局工具、检查点续跑与异常收尾。

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// TestLoopReAct 覆盖主循环的多轮推进、只读并行与事件时序。
func TestLoopReAct(t *testing.T) {
	t.Run("工具调用后收尾", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{"msg":"hi"}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn",
				Usage: llm.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
		}}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil), SchemaGuard())

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hello")})
		require.NoError(t, err)

		assert.Equal(t, 2, p.callCount(), "应发起两轮模型请求")
		assert.Equal(t, ReasonEndTurn, out.Reason)
		assert.Equal(t, "end_turn", out.StopReason)
		assert.Equal(t, 1, echo.callCount())
		require.Len(t, out.Messages, 3, "assistant + tool + assistant")
		assert.Equal(t, "done", out.Messages[2].Content)

		toolMsg := out.Messages[1]
		assert.Equal(t, llm.RoleTool, toolMsg.Role)
		assert.Equal(t, "1", toolMsg.ToolCallID, "tool 结果必须按 tool_call id 配对")
		assert.Equal(t, "echo", toolMsg.ToolName)
		assert.Equal(t, 15, out.Usage.TotalTokens)
	})

	t.Run("只读工具并行且结果按序", func(t *testing.T) {
		readA := &mockTool{name: "read_a", risk: tool.RiskReadOnly, meta: tool.ToolMeta{ReadOnly: true}}
		readB := &mockTool{name: "read_b", risk: tool.RiskReadOnly, meta: tool.ToolMeta{ReadOnly: true}}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{
				{ID: "1", Name: "read_a", Arguments: json.RawMessage(`{}`)},
				{ID: "2", Name: "read_b", Arguments: json.RawMessage(`{}`)},
			}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "both"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, readA, readB).WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("read both")})
		require.NoError(t, err)

		assert.Equal(t, 1, readA.callCount())
		assert.Equal(t, 1, readB.callCount())
		require.Len(t, out.Messages, 4)
		assert.Equal(t, "1", out.Messages[1].ToolCallID, "并行执行也必须按调用顺序回填")
		assert.Equal(t, "2", out.Messages[2].ToolCallID)
	})

	t.Run("事件时序与终态", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "hi"}, StopReason: "end_turn"},
		}}
		sink := &captureSink{}
		l := newTestLoop(t, p, echo).WithSink(sink).WithGuard(ExposeGuard(nil))

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, []EventKind{
			EventRunStart, EventTurnStart, EventTurnDelta, EventTurnEnd, EventRunDone,
		}, sink.kinds())
		assert.Equal(t, 1, sink.count(EventRunDone), "单一终态：所有退出路径只发一次 run.done")
	})
}

// TestLoopResilience 覆盖取消与建流失败两条非乐观路径。
func TestLoopResilience(t *testing.T) {
	t.Run("工具执行中取消归一为 cancelled", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		ctx, cancel := context.WithCancel(context.Background())
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).WithHooks(Hooks{
			AfterToolCall: func(context.Context, Call, tool.ToolResult) { cancel() },
		})

		out, err := l.Run(ctx, []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err, "取消不是错误")

		assert.Equal(t, ReasonCancelled, out.Reason)
		assert.Equal(t, 1, p.callCount(), "取消后不应再发起下一轮")
	})

	t.Run("未装配护栏链直接拒绝执行", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
		}}
		l := newTestLoop(t, p, echo) // 故意不调 WithGuard

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.Error(t, err, "缺护栏必须失败，不能回落裸执行器")

		assert.Equal(t, 0, echo.callCount(), "失败前不应有任何工具被执行")
	})

	t.Run("流式报错且无产出视为轮失败", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{chunkErr: errors.New("upstream closed")}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.Error(t, err)
		require.NotNil(t, out, "终态仍需回传，供上层发 run.done")
		assert.Equal(t, ReasonError, out.Reason)

		assert.Equal(t, 0, echo.callCount())
	})

	t.Run("流式报错但有产出时保留部分正文", func(t *testing.T) {
		p := &mockProvider{
			turns: []llm.ChatResponse{
				{Message: llm.Message{Role: llm.RoleAssistant, Content: "partial"}, StopReason: "stop"},
			},
			chunkErr: errors.New("upstream closed"),
		}
		l := newTestLoop(t, p).WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err, "已有正文的报错轮按成功收尾，不留半成品错误")

		assert.Equal(t, "partial", out.Content)
		assert.Equal(t, ReasonEndTurn, out.Reason)
	})

	t.Run("轮次上限归一为 max_turns", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		turn := llm.ChatResponse{
			ToolCalls:  []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}},
			StopReason: "tool_use",
		}
		p := &mockProvider{turns: []llm.ChatResponse{turn, turn, turn, turn}}
		l := newTestLoop(t, p, echo)
		l.cfg.MaxTurns = 2
		l = l.WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("loop")})
		require.NoError(t, err)
		assert.Equal(t, ReasonMaxTurns, out.Reason)
		assert.Equal(t, 2, p.callCount())
	})
}

// TestLoopTerminalTools 覆盖终局工具：全部结果自述终局时跳过下一轮模型调用。
func TestLoopTerminalTools(t *testing.T) {
	terminal := func(name string) *mockTool {
		return &mockTool{name: name, risk: tool.RiskReadOnly, fn: func(json.RawMessage) tool.ToolResult {
			return tool.ToolResult{Content: "done:" + name, Meta: map[string]string{tool.MetaTerminate: "1"}}
		}}
	}

	t.Run("全部终局则直接收尾", func(t *testing.T) {
		fin := terminal("finish")
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "finish", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "不该被请求"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, fin).WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err)

		assert.Equal(t, 1, p.callCount(), "终局后不应再请求模型")
		assert.Equal(t, ReasonEndTurn, out.Reason)
		assert.Equal(t, "done:finish", out.Messages[1].Content)
	})

	t.Run("混合批次不提前收尾", func(t *testing.T) {
		fin := terminal("finish")
		read := &mockTool{name: "read_a", risk: tool.RiskReadOnly, meta: tool.ToolMeta{ReadOnly: true}}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{
				{ID: "1", Name: "finish", Arguments: json.RawMessage(`{}`)},
				{ID: "2", Name: "read_a", Arguments: json.RawMessage(`{}`)},
			}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, fin, read).WithGuard(ExposeGuard(nil))

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err)

		assert.Equal(t, 2, p.callCount(), "有一个结果需要继续处理时必须进入下一轮")
		assert.Equal(t, "done", out.Content)
	})
}

// TestLoopCheckpoints 覆盖续跑位点与幂等复用（不重放副作用）。
func TestLoopCheckpoints(t *testing.T) {
	t.Run("每轮工具回填后写检查点", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		store := &memCheckpoints{}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).WithCheckpoints(store)

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err)

		require.NotNil(t, store.last)
		assert.Equal(t, 1, store.last.Turn, "只在跑过工具的那一轮留位点")
	})

	t.Run("同参复用已完成调用", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		call := llm.NormalizedToolCall{ID: "1", Name: "echo", Arguments: json.RawMessage(`{"msg":"same"}`)}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{call}, StopReason: "tool_use"},
			{ToolCalls: []llm.NormalizedToolCall{call}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, echo).WithGuard(RepeatGuard(0, newMemStepStore()))

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("twice")})
		require.NoError(t, err)

		assert.Equal(t, 1, echo.callCount(), "第二次同参调用应复用结果，不重放副作用")
	})

	t.Run("从检查点续跑且用量跨段累计", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "resumed"}, StopReason: "end_turn",
				Usage: llm.TokenUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}},
		}}
		store := &memCheckpoints{}
		require.NoError(t, store.Append(&Checkpoint{
			RunID: "RUN_x",
			Turn:  1,
			Messages: []*llm.Message{
				llm.SystemMessage("sys"),
				llm.UserMessage("go"),
				llm.AssistantMessage("first", nil),
			},
			Usage: llm.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		}))
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).WithCheckpoints(store).
			WithMeta(Meta{RunID: "RUN_x", SessionID: "SESSION_x"})

		out, err := l.Resume(context.Background())
		require.NoError(t, err)

		assert.Equal(t, ReasonEndTurn, out.Reason)
		assert.Equal(t, 2, out.Turns, "续跑从上一位点的下一轮开始")
		assert.Equal(t, 20, out.Usage.TotalTokens, "用量跨段累计：续跑不是新的计费周期")
		require.Len(t, out.PerTurn, 1, "明细只记本次实际发起的轮次")
		assert.Equal(t, 2, out.PerTurn[0].Turn)
		assert.Equal(t, 5, out.PerTurn[0].Usage.TotalTokens)
	})

}
