package service

// durable pause 长链路测试：审批/补充输入跨重启决策与续跑、上下文重建的悬挂剥离。
// 判据是「进程死亡不再作废人工决策」：重启 → 重武装 → 决策 → 续跑 → 快速放行不二次询问。

import (
	"context"
	"testing"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDurableApprovalSvc 构造带持久化与续跑钩子的审批服务；返回服务与 runID 续跑记录通道。
func newDurableApprovalSvc(t *testing.T, records *repo.ApprovalRecordRepo) (*ApprovalService, chan string) {
	t.Helper()
	resumed := make(chan string, 8)
	svc := NewApprovalService(event.New()).WithRecords(records).
		WithResumeHook(func(_ context.Context, runID string) error {
			resumed <- runID
			return nil
		})
	return svc, resumed
}

// waitPendingRecord 轮询等未决记录落库（Approve/RequestInput 的持久化是异步前置）。
func waitPendingRecord(t *testing.T, records *repo.ApprovalRecordRepo) *domain.ApprovalRecordDO {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := records.ListPending(context.Background())
		require.NoError(t, err)
		if len(rows) > 0 {
			return &rows[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pending approval record not persisted in time")
	return nil
}

// TestDurableApprovalPauseResume 审批与补充输入的跨进程决策闭环。
func TestDurableApprovalPauseResume(t *testing.T) {
	t.Run("approval_decide_after_restart_resumes_and_fast_allows", func(t *testing.T) {
		records := repo.NewApprovalRecordRepo(newChatOpsTestDB(t))
		runCtx, cancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_A", "SES_A"))
		defer cancel()
		svc1, _ := newDurableApprovalSvc(t, records)

		// 进程内挂起：阻塞等待决策
		done := make(chan bool, 1)
		go func() { done <- svc1.Approve(runCtx, "git push --force", tool.RiskApprovalNeeds) }()
		rec := waitPendingRecord(t, records)

		// 「重启」：新的内存态 + 同一份库；重武装后决策卡仍可见、可决策
		svc2, resumed2 := newDurableApprovalSvc(t, records)
		svc2.RearmPending(context.Background())
		cards := svc2.Pending()
		require.Len(t, cards, 1)
		assert.Equal(t, rec.ID, cards[0].ID)
		require.NoError(t, svc2.Decide(rec.ID, true, ""))
		select {
		case runID := <-resumed2:
			assert.Equal(t, "RUN_A", runID, "决策必须触发续跑")
		case <-time.After(2 * time.Second):
			t.Fatalf("resume hook not fired after decide")
		}

		// 续跑重放同一调用（resumed 标记）：已决记录快速放行（不再询问），并消费置 consumed
		assert.True(t, svc2.Approve(core.WithResumedRun(core.WithRunContext(context.Background(), "RUN_A", "SES_A")),
			"git push --force", tool.RiskApprovalNeeds), "已批准记录必须快速放行")
		consumed, err := records.Get(context.Background(), rec.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ApprovalStatusConsumed, consumed.Status)

		// 消费一次性：同命令再次调用（非续跑）必须重新询问，不能演变成永久免审
		askCtx, askCancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_A", "SES_A"))
		defer askCancel()
		again := make(chan bool, 1)
		go func() { again <- svc2.Approve(askCtx, "git push --force", tool.RiskApprovalNeeds) }()
		reAsked := waitPendingRecord(t, records)
		require.NoError(t, svc2.Skip(reAsked.ID))
		select {
		case ok := <-again:
			assert.False(t, ok)
		case <-time.After(2 * time.Second):
			t.Fatalf("skip did not settle re-asked approval")
		}
		// 注：svc1 的原始阻塞 Approve 由 subtest 退出时的 cancel() 唤醒（ctx.Done → false）
	})

	t.Run("input_answer_after_restart_reused_on_resume", func(t *testing.T) {
		records := repo.NewApprovalRecordRepo(newChatOpsTestDB(t))
		question := "部署目录是哪个？"
		runCtx, cancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_B", "SES_B"))
		defer cancel()
		svc1, _ := newDurableApprovalSvc(t, records)

		type answer struct {
			text string
			ok   bool
		}
		done := make(chan answer, 1)
		go func() {
			text, ok := svc1.RequestInput(runCtx, question)
			done <- answer{text, ok}
		}()
		rec := waitPendingRecord(t, records)

		svc2, resumed2 := newDurableApprovalSvc(t, records)
		svc2.RearmPending(context.Background())
		require.Len(t, svc2.Pending(), 1)
		require.NoError(t, svc2.Answer(rec.ID, "/srv/app"))
		select {
		case runID := <-resumed2:
			assert.Equal(t, "RUN_B", runID)
		case <-time.After(2 * time.Second):
			t.Fatalf("resume hook not fired after answer")
		}

		// 续跑重放同一提问（resumed 标记）：直接复用已落库回答，不再二次打扰
		text, ok := svc2.RequestInput(core.WithResumedRun(core.WithRunContext(context.Background(), "RUN_B", "SES_B")), question)
		assert.True(t, ok)
		assert.Equal(t, "/srv/app", text)
		consumed, err := records.Get(context.Background(), rec.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ApprovalStatusConsumed, consumed.Status)

		// skipped 路径：重启后跳过 → 续跑重放得到「未回复」语义
		done2 := make(chan answer, 1)
		go func() {
			text, ok := svc2.RequestInput(runCtx, "另一个问题？")
			done2 <- answer{text, ok}
		}()
		rec2 := waitPendingRecord(t, records)
		require.NoError(t, svc2.Skip(rec2.ID))
		select {
		case a := <-done2:
			assert.False(t, a.ok)
		case <-time.After(2 * time.Second):
			t.Fatalf("skip did not settle pending input")
		}
		text2, ok2 := svc2.RequestInput(core.WithResumedRun(core.WithRunContext(context.Background(), "RUN_B", "SES_B")), "另一个问题？")
		assert.False(t, ok2)
		assert.Empty(t, text2)
		// svc1 的原始阻塞 RequestInput 由 subtest 退出时的 cancel() 唤醒，不在此等待
	})

	t.Run("cancel_session_clears_leftover_pendings", func(t *testing.T) {
		records := repo.NewApprovalRecordRepo(newChatOpsTestDB(t))
		svc, _ := newDurableApprovalSvc(t, records)
		runCtx, cancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_C", "SES_C"))
		defer cancel()
		done := make(chan bool, 1)
		go func() { done <- svc.Approve(runCtx, "cmd", tool.RiskApprovalNeeds) }()
		rec := waitPendingRecord(t, records)

		svc.CancelSession(context.Background(), "SES_C")
		assert.Empty(t, svc.Pending(), "同会话遗留挂起必须清空")
		row, err := records.Get(context.Background(), rec.ID)
		require.NoError(t, err)
		assert.Contains(t, []string{domain.ApprovalStatusCancelled, domain.ApprovalStatusDenied}, row.Status)
		select {
		case ok := <-done:
			assert.False(t, ok, "内存等待者随取消醒来")
		case <-time.After(2 * time.Second):
			t.Fatalf("blocked approver not woken by CancelSession")
		}
	})
}

// TestApprovalGrantsLifecycle 「本会话允许」的持久化授权生命周期：
// 一次性批准不落库；会话级批准落库且跨重启生效；run 失败/取消时回滚。
func TestApprovalGrantsLifecycle(t *testing.T) {
	t.Run("session_grant_persists_and_reloads", func(t *testing.T) {
		gdb := newChatOpsTestDB(t)
		records := repo.NewApprovalRecordRepo(gdb)
		grants := repo.NewApprovalGrantRepo(gdb)
		runCtx, cancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_G", "SES_G"))
		defer cancel()
		svc1, _ := newDurableApprovalSvc(t, records)
		svc1.WithGrants(grants)

		ask := func() (chan bool, *domain.ApprovalRecordDO) {
			ch := make(chan bool, 1)
			go func() { ch <- svc1.Approve(runCtx, "npm publish", tool.RiskApprovalNeeds) }()
			return ch, waitPendingRecord(t, records)
		}
		// 一次性批准：不产生持久化授权
		ch1, rec1 := ask()
		require.NoError(t, svc1.Decide(rec1.ID, true, ""))
		assert.True(t, <-ch1)
		rows, err := grants.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, rows, "一次性批准不得落库")

		// 会话级批准：落库
		ch2, rec2 := ask()
		require.NoError(t, svc1.Decide(rec2.ID, true, "session"))
		assert.True(t, <-ch2)
		rows, err = grants.List(context.Background())
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "npm publish", rows[0].Command)

		// 「重启」：新内存态装载授权后，同命令免审
		svc2, _ := newDurableApprovalSvc(t, records)
		svc2.WithGrants(grants)
		svc2.LoadGrants(context.Background())
		assert.True(t, svc2.Approve(core.WithRunContext(context.Background(), "RUN_G2", "SES_G"),
			"npm publish", tool.RiskApprovalNeeds), "持久化授权必须跨重启免审")
	})

	t.Run("rollback_run_drops_session_grant", func(t *testing.T) {
		gdb := newChatOpsTestDB(t)
		records := repo.NewApprovalRecordRepo(gdb)
		grants := repo.NewApprovalGrantRepo(gdb)
		runCtx, cancel := context.WithCancel(core.WithRunContext(context.Background(), "RUN_R", "SES_R"))
		defer cancel()
		svc, _ := newDurableApprovalSvc(t, records)
		svc.WithGrants(grants)

		ch := make(chan bool, 1)
		go func() { ch <- svc.Approve(runCtx, "rm -rf build", tool.RiskApprovalNeeds) }()
		rec := waitPendingRecord(t, records)
		require.NoError(t, svc.Decide(rec.ID, true, "session"))
		assert.True(t, <-ch)

		// run 失败 → 回滚本次 run 扩出的授权
		svc.RollbackRun("RUN_R")
		rows, err := grants.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, rows, "失败 run 的授权必须回滚")
		again := make(chan bool, 1)
		go func() { again <- svc.Approve(runCtx, "rm -rf build", tool.RiskApprovalNeeds) }()
		reAsked := waitPendingRecord(t, records)
		require.NoError(t, svc.Skip(reAsked.ID))
		select {
		case ok := <-again:
			assert.False(t, ok, "回滚后同命令必须重新询问")
		case <-time.After(2 * time.Second):
			t.Fatalf("rollback did not restore approval prompt")
		}
	})
}

// TestRebuildStripsDanglingToolCalls 取消/崩溃残段的上下文重建：悬挂 tool_calls 必须剥掉
// （tool_use 无对应 tool_result 上游必 400），半截正文保留（中断续聊叙事不断裂）。
func TestRebuildStripsDanglingToolCalls(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, _ := seedSession(t, svc, msgRepo, ctx)

	calls := `[{"id":"T1","type":"function","function":{"name":"exec","arguments":"{}"}},
	           {"id":"T2","type":"function","function":{"name":"exec","arguments":"{}"}}]`
	// 取消残段：assistant 发起 T1/T2，只有 T1 的结果落库
	require.NoError(t, msgRepo.Insert(ctx, &domain.MessageDO{
		ID: "M_DA", SessionID: ses.ID, Seq: 6, RunID: "RUN_DA",
		Role: domain.MessageRoleAssistant, Content: "开始执行，已完成第一步",
		ToolCalls: calls, Status: domain.MessageStatusFailed,
	}))
	require.NoError(t, msgRepo.Insert(ctx, &domain.MessageDO{
		ID: "M_DT1", SessionID: ses.ID, Seq: 7, RunID: "RUN_DA",
		Role: domain.MessageRoleTool, ToolCallID: "T1", Content: "step1 ok", Status: domain.MessageStatusCompleted,
	}))
	hists, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
	require.NoError(t, err)

	msgs, err := svc.toLLMMessages(hists)
	require.NoError(t, err)
	var assistant *llm.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistant = m
		}
	}
	require.NotNil(t, assistant, "残段 assistant 必须保留")
	assert.Equal(t, "开始执行，已完成第一步", assistant.Content, "半截正文保留")
	require.Len(t, assistant.ToolCalls, 1)
	assert.Equal(t, "T1", assistant.ToolCalls[0].ID, "悬挂的 T2 必须剥离")

	// 全悬挂且无正文：等价空占位，整体剥掉
	require.NoError(t, msgRepo.Insert(ctx, &domain.MessageDO{
		ID: "M_DB", SessionID: ses.ID, Seq: 8, RunID: "RUN_DB",
		Role:      domain.MessageRoleAssistant,
		ToolCalls: `[{"id":"T3","type":"function","function":{"name":"exec","arguments":"{}"}}]`,
		Status:    domain.MessageStatusFailed,
	}))
	hists2, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
	require.NoError(t, err)
	msgs2, err := svc.toLLMMessages(hists2)
	require.NoError(t, err)
	assistantCount := 0
	for _, m := range msgs2 {
		if m.Role == "assistant" {
			assistantCount++
		}
	}
	assert.Equal(t, 1, assistantCount, "全悬挂空正文占位应被剥离（只剩 M_DA 的残段）")
}
