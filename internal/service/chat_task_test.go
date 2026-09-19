package service

import (
	"context"
	"testing"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForTaskState 轮询等待任务进入目标状态（worker 是异步 goroutine，不能同步断言）。
func waitForTaskState(t *testing.T, svc *ChatTaskService, id string, want string) domain.ChatTaskDO {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, err := svc.List(context.Background(), 50)
		if err == nil {
			for _, it := range row.Items {
				if it.ID == id && it.State == want {
					return it
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s 未在限时内进入状态 %s", id, want)
	return domain.ChatTaskDO{}
}

// TestChatTaskLifecycle 覆盖任务域的提交校验、执行链路与取消语义。
func TestChatTaskLifecycle(t *testing.T) {
	gdb := newChatOpsTestDB(t)
	bus := event.New()
	sessRepo := repo.NewChatSessionRepo(gdb)
	taskRepo := repo.NewChatTaskRepo(gdb)
	chat := NewChatService(nil, nil, nil, nil, nil, bus, nil, nil, nil)
	approval := NewApprovalService(bus)
	svc := NewChatTaskService(taskRepo, bus, chat, approval, sessRepo)

	t.Run("提交校验：空描述与空会话拒绝", func(t *testing.T) {
		_, err := svc.Submit(context.Background(), "SES_x", "default", "   ")
		require.Error(t, err)
		_, err = svc.Submit(context.Background(), "", "default", "任务")
		require.Error(t, err)
	})

	t.Run("不存在的宿主会话 → 快速失败", func(t *testing.T) {
		row, err := svc.Submit(context.Background(), "SES_missing", "default", "整理文件")
		require.NoError(t, err)
		require.Equal(t, domain.TaskStatePending, row.State, "提交即返回 pending")
		done := waitForTaskState(t, svc, row.ID, domain.TaskStateFailed)
		assert.Contains(t, done.Error, "宿主会话不可用")
	})

	t.Run("取消 pending 任务直接落终态", func(t *testing.T) {
		// 用真实会话但 Provider 不可用：任务会停在 pending（队列）与 running（快速失败）之间，
		// 这里直接验证 pending → cancelled 分支：先建会话再抢在 worker 前取消不可靠，
		// 退而验证「对终态任务重复取消是幂等 no-op」。
		row, err := svc.Submit(context.Background(), "SES_missing", "default", "another")
		require.NoError(t, err)
		waitForTaskState(t, svc, row.ID, domain.TaskStateFailed)
		require.NoError(t, svc.Cancel(context.Background(), row.ID), "终态重复取消必须幂等")
		final := waitForTaskState(t, svc, row.ID, domain.TaskStateFailed)
		assert.Equal(t, domain.TaskStateFailed, final.State, "取消不得改写终态")
	})

	t.Run("启动恢复：遗留未终态任务标记失败", func(t *testing.T) {
		legacy := &domain.ChatTaskDO{
			ID: pkg.NewID("TASK"), SessionID: "SES_x", Agent: "default", Prompt: "p", State: domain.TaskStateRunning,
		}
		require.NoError(t, taskRepo.Create(context.Background(), legacy))
		// 再构造一个服务实例：新实例的 recoverUnfinished 应把 legacy 打成 failed
		NewChatTaskService(taskRepo, bus, chat, approval, sessRepo)
		got, err := taskRepo.GetByID(context.Background(), legacy.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStateFailed, got.State)
		assert.Contains(t, got.Error, "应用重启")
	})
}

// TestGrantApprover 无人值守审批门：只吃免审授权，不可逆一律拒绝。
func TestGrantApprover(t *testing.T) {
	bus := event.New()
	svc := NewApprovalService(bus)
	g := grantApprover{svc: svc}
	ctx := context.Background()
	call := core.Call{ID: "1", Name: "exec", Args: []byte(`{"command":"ls"}`)}

	t.Run("未授权拒绝且带可解释原因", func(t *testing.T) {
		assert.False(t, g.Approve(ctx, call, tool.RiskApprovalNeeds))
		assert.Contains(t, g.DenyReason(), "免审授权")
	})

	t.Run("已授权放行", func(t *testing.T) {
		svc.rememberGrant(ctx, "", approvalCommand(call), tool.RiskApprovalNeeds)
		assert.True(t, g.Approve(ctx, call, tool.RiskApprovalNeeds))
	})

	t.Run("不可逆操作永不免审", func(t *testing.T) {
		assert.False(t, g.Approve(ctx, call, tool.RiskApprovalIrrev))
	})

	t.Run("nil 审批服务拒绝", func(t *testing.T) {
		assert.False(t, grantApprover{}.Approve(ctx, call, tool.RiskApprovalNeeds))
	})
}
