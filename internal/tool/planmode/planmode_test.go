// 计划模式测试：写操作硬拦、退出必须经审批、无会话上下文时 fail-closed。

package planmode

import (
	"context"
	"testing"

	"WorkBaby/internal/core"
	"WorkBaby/internal/pkg"
)

// funcApprover Approver 测试桩（签名与 Approve 一致，隐式实现接口）。
type funcApprover func(ctx context.Context, command string, risk string) bool

func (f funcApprover) Approve(ctx context.Context, command string, risk string) bool {
	return f(ctx, command, risk)
}

func newTestStore() *Store {
	return NewStore(func(name string) bool {
		return name == "file_read" || name == "file_grep" || name == "knowledge_search"
	})
}

// TestPlanMode 计划模式的三条安全底线：激活后写/执行被硬拦、退出必须经审批、缺少会话上下文时 fail-closed。
func TestPlanMode(t *testing.T) {
	newStore := func() *Store { return newTestStore() }

	t.Run("guard_blocks_side_effects", func(t *testing.T) {
		s := newStore()
		if reason := s.Guard("S1", "file_write"); reason != "" {
			t.Fatalf("未激活时不应拦截: %q", reason)
		}
		s.Enable("S1")
		for _, name := range []string{"file_read", "file_grep", "exit_plan_mode"} {
			if reason := s.Guard("S1", name); reason != "" {
				t.Fatalf("%s 在计划模式下应放行: %q", name, reason)
			}
		}
		for _, name := range []string{"file_write", "file_edit", "exec", "run_skill_script"} {
			if s.Guard("S1", name) == "" {
				t.Fatalf("%s 在计划模式下应被拒绝", name)
			}
		}
		s.Disable("S1")
		if reason := s.Guard("S1", "file_write"); reason != "" {
			t.Fatalf("关闭后应恢复放行: %q", reason)
		}
	})

	t.Run("exit_requires_approval", func(t *testing.T) {
		s := newStore()
		s.Enable("S1")
		denied := NewExit(s, funcApprover(func(context.Context, string, string) bool { return false }))
		if res := denied.Execute(core.WithRunContext(context.Background(), "R", "S1"), nil); res.Err != nil {
			t.Fatalf("unexpected err: %v", res.Err)
		}
		if !s.Active("S1") {
			t.Fatal("审批拒绝后应保持计划模式")
		}
		approved := NewExit(s, funcApprover(func(context.Context, string, string) bool { return true }))
		if res := approved.Execute(core.WithRunContext(context.Background(), "R", "S1"), nil); res.Err != nil {
			t.Fatalf("unexpected err: %v", res.Err)
		}
		if s.Active("S1") {
			t.Fatal("审批批准后应退出计划模式")
		}
	})

	t.Run("enter_without_session_fails_closed", func(t *testing.T) {
		res := NewEnter(newStore()).Execute(context.Background(), nil)
		if res.Err == nil {
			t.Fatal("无会话上下文应报错而非静默")
		}
		if _, ok := res.Err.(*pkg.AppError); !ok {
			t.Fatalf("应返回 AppError，got %T", res.Err)
		}
	})
}
