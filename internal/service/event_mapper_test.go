package service

// 事件链路测试：内核事件 → 前端协议映射、子 Agent 事件隔离、辅助会话前缀截取、事件出口契约。

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
)

// TestCoreEventMapper 覆盖事件映射与父子 run 分流。
func TestCoreEventMapper(t *testing.T) {
	t.Run("事件序列映射为 chat 协议", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		var mu sync.Mutex
		var names []string
		svc.bus.Subscribe(event.MatchPrefix("chat:"), func(name string, _ any) {
			mu.Lock()
			defer mu.Unlock()
			names = append(names, name)
		})

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(agent.Event{Kind: agent.EventRunStart, RunID: "RUN_1"})
		m.handle(agent.Event{Kind: agent.EventTurnStart, RunID: "RUN_1", Turn: 1})
		m.handle(agent.Event{Kind: agent.EventTurnDelta, RunID: "RUN_1", Turn: 1,
			Payload: agent.DeltaPayload{Kind: "content", Text: "hi"}})
		m.handle(agent.Event{Kind: agent.EventTurnEnd, RunID: "RUN_1", Turn: 1,
			Payload: agent.TurnEndPayload{Usage: agent.UsagePayload{Input: 10, Output: 5, Total: 15}}})
		m.handle(agent.Event{Kind: agent.EventToolCall, RunID: "RUN_1", Turn: 1,
			Payload: agent.ToolCallPayload{ID: "t1", Name: "file_read", Arguments: `{"path":"a"}`}})
		m.handle(agent.Event{Kind: agent.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: agent.ToolResultPayload{ToolCallID: "t1", Name: "file_read", Content: "AAA", UIHint: "diff"}})
		m.handle(agent.Event{Kind: agent.EventRunDone, RunID: "RUN_1", Turn: 1,
			Payload: agent.RunDonePayload{Reason: agent.ReasonEndTurn, Usage: agent.UsagePayload{Total: 15}, Turns: 1}})

		assert.Equal(t, []string{
			"chat:stream.start", "chat:turn-start", "chat:stream", "chat:stats",
			"chat:tool", "chat:tool-result",
		}, names)

		require.Len(t, m.toolCalls, 1, "工具调用需收集进 assistant.tool_calls")
		assert.Equal(t, "file_read", m.toolCalls[0].Function.Name)
		require.NotNil(t, m.doneEvent, "终态延迟到 assistant 落库后由调用方发出")
		done, ok := m.doneEvent.(domain.ChatDoneEvent)
		require.True(t, ok, "终态载荷类型固定为 domain.ChatDoneEvent")
		assert.Equal(t, "MSG_1", done.MessageID)
		assert.Equal(t, string(domain.StopReasonCompleted), done.Reason)
	})

	t.Run("工具结果落库为 tool 消息", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, before := seedSession(t, svc, msgRepo, ctx)

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(agent.Event{Kind: agent.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: agent.ToolResultPayload{ToolCallID: "t1", Name: "exec", Content: "ok"}})

		rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
		require.NoError(t, err)
		require.Len(t, rows, int(before)+1, "工具结果必须落库：下次 run 重建上下文要用")

		last := rows[len(rows)-1]
		assert.Equal(t, domain.MessageRoleTool, last.Role)
		assert.Equal(t, "t1", last.ToolCallID, "缺少配对 id 会被上游以 400 拒绝")
		assert.Equal(t, "ok", last.Content)
	})

	t.Run("工具失败时错误并入正文", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(agent.Event{Kind: agent.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: agent.ToolResultPayload{ToolCallID: "t1", Name: "exec", Content: "detail", Err: "exit 1"}})

		rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
		require.NoError(t, err)
		last := rows[len(rows)-1]
		assert.Contains(t, last.Content, "exit 1", "模型需要看到上次为什么失败")
		assert.Contains(t, last.Content, "detail")
	})

	t.Run("tool_result meta 透传 cwd / same_failure_count / adaptive_hint", func(t *testing.T) {
		svc, _ := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "meta"})

		// 订阅 chat:tool-result 收集载荷
		var mu sync.Mutex
		var captured map[string]any
		svc.bus.Subscribe(event.MatchExact("chat:tool-result"), func(_ string, payload any) {
			m, _ := payload.(map[string]any)
			mu.Lock()
			defer mu.Unlock()
			captured = m
		})

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(agent.Event{Kind: agent.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: agent.ToolResultPayload{
				ToolCallID: "t1", Name: "exec", Content: "ok",
				Meta: map[string]string{
					"cwd":                `D:\WorkBaby\test`,
					"same_failure_count": "3",
					"adaptive_hint":      "1",
				},
			}})

		require.NotNil(t, captured, "必须发出 chat:tool-result 事件")
		// chat:tool-result 由强类型 domain.ChatToolResultEvent 发出，Emitter 归一为 map 后广播；
		// meta 子 map 的键值仍是 string（json.Marshal map[string]string 保持值类型）。
		meta, ok := captured["meta"].(map[string]any)
		require.True(t, ok, "meta 必须透传为 map")
		assert.Equal(t, `D:\WorkBaby\test`, meta["cwd"], "exec 实际目录必须透出")
		assert.Equal(t, "3", meta["same_failure_count"], "失败计数必须透出")
		assert.Equal(t, "1", meta["adaptive_hint"], "改道标记必须透出")
	})

	t.Run("子 Agent 事件分流不写父 run 历史", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, before := seedSession(t, svc, msgRepo, ctx)

		var mu sync.Mutex
		var names []string
		svc.bus.Subscribe(event.MatchPrefix("chat:"), func(name string, _ any) {
			mu.Lock()
			defer mu.Unlock()
			names = append(names, name)
		})

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(agent.Event{Kind: agent.EventRunStart, RunID: "RUN_2", Agent: "explore"})
		m.handle(agent.Event{Kind: agent.EventToolResult, RunID: "RUN_2", Agent: "explore",
			Payload: agent.ToolResultPayload{ToolCallID: "t9", Name: "file_read", Content: "child"}})
		m.handle(agent.Event{Kind: agent.EventRunDone, RunID: "RUN_2", Agent: "explore",
			Payload: agent.RunDonePayload{Reason: agent.ReasonEndTurn}})

		assert.Equal(t, []string{"chat:subagent-start", "chat:tool-result", "chat:subagent-done"}, names,
			"子 run 复用父 run 的 chat:done 会让前端提前关闭 SSE")

		rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
		require.NoError(t, err)
		assert.Len(t, rows, int(before), "子工具结果不得写进父 run 的 tool 历史")
		assert.Nil(t, m.doneEvent, "子 run 终态不进父 run 的收尾事件")
	})
}

// TestSideParentMessages 主会话历史前缀：有界截取 + user 轮次对齐 + system 来源标注。
func TestSideParentMessages(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := t.Context()

	ses, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "main-long"})
	require.NoError(t, err)
	// 20 条 × ~900 rune：超过 12000 rune 预算，前缀必须截断
	for i := 1; i <= 20; i++ {
		role := domain.MessageRoleUser
		if i%2 == 0 {
			role = domain.MessageRoleAssistant
		}
		content := make([]rune, 900)
		for j := range content {
			content[j] = '字'
		}
		require.NoError(t, msgRepo.Insert(ctx, &domain.MessageDO{
			ID: "M_SIDE_" + itoa(i), SessionID: ses.ID, Seq: int64(i),
			Role: role, Content: string(content), Status: domain.MessageStatusCompleted,
		}))
	}
	row, err := svc.sessions.GetByID(ctx, ses.ID)
	require.NoError(t, err)

	side, err := svc.EnsureSideConversation(ctx, ses.ID)
	require.NoError(t, err)
	sideRow, err := svc.sessions.GetByID(ctx, side.ID)
	require.NoError(t, err)

	msgs, err := svc.sideParentMessages(ctx, sideRow, false)
	require.NoError(t, err)
	require.NotNil(t, msgs)
	// 首条是来源标注 system
	require.Equal(t, "system", string(msgs[0].Role))
	// 预算截断：总 rune 数不应远超预算（header + 界内消息）
	used := 0
	for _, m := range msgs[1:] {
		used += len([]rune(m.Content))
	}
	require.LessOrEqual(t, used, sideParentHistoryRunes+2000)
	// 有界截断确实发生：20 条 × 900 rune 未全量带入
	require.Less(t, len(msgs)-1, 20)
	// 起点对齐 user 轮次
	require.Equal(t, "user", string(msgs[1].Role))

	// 普通会话（非 side）没有前缀
	normalMsgs, err := svc.sideParentMessages(ctx, row, false)
	require.NoError(t, err)
	require.Nil(t, normalMsgs)
}

// TestEmitterContract 事件出口契约：结构体载荷归一、归属注入、重放日志入账。
// 这三条是 SSE 断线重放与前端渲染正确性的前提，任一失效都只表现为「前端莫名少事件」。
func TestEmitterContract(t *testing.T) {
	bus := event.New()
	log := event.NewRunEventLog(0, 0)
	em := NewEmitter(bus, log)

	var got map[string]any
	bus.Subscribe(event.MatchPrefix("chat:"), func(_ string, payload any) {
		got, _ = payload.(map[string]any)
	})

	// 结构体载荷：经归一化后成为带归属字段的 map（前端契约形状固定）
	em.Emit("RUN_1", "SES_1", "chat:done", domain.ChatDoneEvent{
		Status: "completed", Reason: "completed", MessageID: "MSG_1",
	})
	require.NotNil(t, got, "事件必须广播到总线")
	assert.Equal(t, "RUN_1", got["run_id"])
	assert.Equal(t, "SES_1", got["session_id"])
	assert.Equal(t, "MSG_1", got["message_id"])
	assert.Equal(t, "completed", got["status"])

	// run 事件进重放缓冲：断线重连时按 Last-Event-ID 补齐
	events, covered := log.Replay("RUN_1", 0)
	assert.True(t, covered)
	require.Len(t, events, 1)
	assert.Equal(t, "chat:done", events[0].Name)

	// 会话级事件（run_id 为空）：仍然广播（按会话过滤由 SSE 完成），但不进 run 重放缓冲
	got = nil
	em.Emit("", "SES_1", "chat:goal", map[string]any{"goal": nil})
	require.NotNil(t, got, "无 run 归属的事件不能因订阅带 run 就丢弃")
	assert.Equal(t, "", got["run_id"])
	assert.Equal(t, "SES_1", got["session_id"])
}
