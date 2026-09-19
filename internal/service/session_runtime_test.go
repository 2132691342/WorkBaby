package service

// 会话运行时链路测试：内核事件 → 前端协议映射、辅助会话（主会话历史前缀截取）、目标模式状态机。
//
// 这三条链路的共同点是「跨层且易静默失效」：事件映射错一个名字前端就收不到终态；
// 辅助会话前缀截取越界会把上下文撑爆或切断 user 轮次；目标模式状态机漏一个分支
// 就会让目标卡在不可恢复的中间态。

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/repo"
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
		m.handle(core.Event{Kind: core.EventRunStart, RunID: "RUN_1"})
		m.handle(core.Event{Kind: core.EventTurnStart, RunID: "RUN_1", Turn: 1})
		m.handle(core.Event{Kind: core.EventTurnDelta, RunID: "RUN_1", Turn: 1,
			Payload: core.DeltaPayload{Kind: "content", Text: "hi"}})
		m.handle(core.Event{Kind: core.EventTurnEnd, RunID: "RUN_1", Turn: 1,
			Payload: core.TurnEndPayload{Usage: core.UsagePayload{Input: 10, Output: 5, Total: 15}}})
		m.handle(core.Event{Kind: core.EventToolCall, RunID: "RUN_1", Turn: 1,
			Payload: core.ToolCallPayload{ID: "t1", Name: "file_read", Arguments: `{"path":"a"}`}})
		m.handle(core.Event{Kind: core.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: core.ToolResultPayload{ToolCallID: "t1", Name: "file_read", Content: "AAA", UIHint: "diff"}})
		m.handle(core.Event{Kind: core.EventRunDone, RunID: "RUN_1", Turn: 1,
			Payload: core.RunDonePayload{Reason: core.ReasonEndTurn, Usage: core.UsagePayload{Total: 15}, Turns: 1}})

		assert.Equal(t, []string{
			"chat:stream.start", "chat:turn-start", "chat:stream", "chat:stats",
			"chat:tool", "chat:tool-result",
		}, names)

		require.Len(t, m.toolCalls, 1, "工具调用需收集进 assistant.tool_calls")
		assert.Equal(t, "file_read", m.toolCalls[0].Function.Name)
		require.NotNil(t, m.doneEvent, "终态延迟到 assistant 落库后由调用方发出")
		assert.Equal(t, "MSG_1", m.doneEvent["message_id"])
	})

	t.Run("工具结果落库为 tool 消息", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, before := seedSession(t, svc, msgRepo, ctx)

		m := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		m.handle(core.Event{Kind: core.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: core.ToolResultPayload{ToolCallID: "t1", Name: "exec", Content: "ok"}})

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
		m.handle(core.Event{Kind: core.EventToolResult, RunID: "RUN_1", Turn: 1,
			Payload: core.ToolResultPayload{ToolCallID: "t1", Name: "exec", Content: "detail", Err: "exit 1"}})

		rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
		require.NoError(t, err)
		last := rows[len(rows)-1]
		assert.Contains(t, last.Content, "exit 1", "模型需要看到上次为什么失败")
		assert.Contains(t, last.Content, "detail")
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
		m.handle(core.Event{Kind: core.EventRunStart, RunID: "RUN_2", Agent: "explore"})
		m.handle(core.Event{Kind: core.EventToolResult, RunID: "RUN_2", Agent: "explore",
			Payload: core.ToolResultPayload{ToolCallID: "t9", Name: "file_read", Content: "child"}})
		m.handle(core.Event{Kind: core.EventRunDone, RunID: "RUN_2", Agent: "explore",
			Payload: core.RunDonePayload{Reason: core.ReasonEndTurn}})

		assert.Equal(t, []string{"chat:subagent-start", "chat:tool-result", "chat:subagent-done"}, names,
			"子 run 复用父 run 的 chat:done 会让前端提前关闭 SSE")

		rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
		require.NoError(t, err)
		assert.Len(t, rows, int(before), "子工具结果不得写进父 run 的 tool 历史")
		assert.Nil(t, m.doneEvent, "子 run 终态不进父 run 的收尾事件")
	})
}

// TestSideConversationLifecycle 辅助会话：ensure 幂等 / 不进侧栏列表 / 随主会话配置。
func TestSideConversationLifecycle(t *testing.T) {
	svc, _ := newChatOpsService(t)
	ctx := t.Context()

	ses, _ := seedSession(t, svc, sideMsgRepo(t, svc), ctx)

	side, err := svc.EnsureSideConversation(ctx, ses.ID)
	require.NoError(t, err)
	require.Equal(t, domain.SessionKindSide, side.Kind)
	require.Equal(t, ses.ID, side.ParentID)
	require.True(t, len(side.Name) > 0)

	// 幂等：再次 ensure 返回同一条
	again, err := svc.EnsureSideConversation(ctx, ses.ID)
	require.NoError(t, err)
	require.Equal(t, side.ID, again.ID)

	// 侧栏列表不含辅助会话
	list, err := svc.ListSessions(ctx, 1, 50)
	require.NoError(t, err)
	for _, it := range list.Items {
		require.NotEqual(t, side.ID, it.ID)
	}
	require.True(t, list.Total >= 1)

	// get 已有
	got, err := svc.GetSideConversation(ctx, ses.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, side.ID, got.ID)
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

// sideMsgRepo 测试种子数据用的消息仓储。
func sideMsgRepo(t *testing.T, svc *ChatService) *repo.MessageRepo {
	t.Helper()
	return svc.messages
}

// TestGoalLifecycle 目标模式状态机：set → pause → resume → clear 全链路，
// 目标持久化在会话元数据（重启后 readSessionMeta 仍可恢复）。
func TestGoalLifecycle(t *testing.T) {
	svc, _ := newChatOpsService(t)
	ctx := context.Background()

	sesResp, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "goal"})
	require.NoError(t, err)
	sesID := sesResp.ID

	// 无参数 set 被拒：目标描述不能为空
	_, err = svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "set"})
	require.Error(t, err)

	// set → active（Round 从 0 起）
	g1, err := svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "set", Text: "修复所有 TS 编译错误并保持测试通过"})
	require.NoError(t, err)
	require.NotNil(t, g1.Goal)
	assert.Equal(t, domain.GoalStatusActive, g1.Goal.Status)
	assert.Equal(t, 0, g1.Goal.Round)
	assert.Equal(t, domain.GoalDefaultMaxRounds, g1.Goal.MaxRounds)

	// 权威拉取（模拟重启 / 切会话）
	got, err := svc.Goal(ctx, sesID)
	require.NoError(t, err)
	require.NotNil(t, got.Goal)
	assert.Equal(t, "修复所有 TS 编译错误并保持测试通过", got.Goal.Text)

	// set 已有活动目标 = replace，Round 保留
	g2, err := svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "set", Text: "新目标"})
	require.NoError(t, err)
	assert.Equal(t, "新目标", g2.Goal.Text)

	// pause / resume
	gp, err := svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "pause"})
	require.NoError(t, err)
	assert.Equal(t, domain.GoalStatusPaused, gp.Goal.Status)
	gr, err := svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "resume"})
	require.NoError(t, err)
	assert.Equal(t, domain.GoalStatusActive, gr.Goal.Status)

	// clear → 无目标
	gc, err := svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "clear"})
	require.NoError(t, err)
	assert.Nil(t, gc.Goal)

	// pause 空目标报错
	_, err = svc.SetGoal(ctx, sesID, domain.GoalREQ{Action: "pause"})
	require.Error(t, err)
}
