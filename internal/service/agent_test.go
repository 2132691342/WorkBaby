// 服务层集成链路：建会话 → 发消息 → 内核跑完落库；写操作审批的闭环（含会话级放行）。
// 覆盖的是跨模块协作（repo + agent + registry + emitter），单层单测测不到。
package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"WorkBaby/internal/config"
	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/factory"
	"WorkBaby/internal/llm/llmtest"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool"
)

// approveTool 永远要求审批，用来打通审批闭环。
type approveTool struct{ ws string }

func (approveTool) Name() string               { return "risky" }
func (approveTool) Label() string              { return "危险操作" }
func (approveTool) Description() string        { return "测试" }
func (approveTool) PromptSnippet() string      { return "测试" }
func (approveTool) PromptGuidelines() []string { return nil }
func (approveTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (approveTool) ExecutionMode() tool.ExecutionMode { return tool.ExecutionSequential }
func (approveTool) RequiresApproval() bool            { return true }
func (approveTool) Execute(ctx context.Context, in tool.Input) (*tool.Result, error) {
	return &tool.Result{Content: "done", Title: "危险操作"}, nil
}

// newEnv 起一套干净的依赖：DB + 工具注册表 + 服务容器，并完成 Bootstrap。
func newEnv(t *testing.T, register ...tool.Tool) (*Env, *Container) {
	t.Helper()
	dir := t.TempDir()
	gdb, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool, _ := gdb.DB()
		_ = pool.Close()
	})
	paths := runtime.Paths{DataDir: dir, LogDir: dir, TmpDir: dir, SkillsDir: filepath.Join(dir, "skills")}
	_ = pkg.EnsureDir(paths.SkillsDir)
	reg := tool.New()
	for _, x := range register {
		reg.Register(x)
	}
	env := &Env{
		Repo: repo.New(gdb), Paths: paths, Emitter: NewEmitter(), Registry: reg,
		ToolDeps: NewToolDeps(paths, nil),
		Cfg:      &config.Config{Workspace: dir},
	}
	svc, err := New(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	return env, svc
}

// newProviderSession 装好默认 provider 并建一个新会话。
func newProviderSession(t *testing.T, svc *Container) *domain.SessionVO {
	t.Helper()
	p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{Name: "t", API: "test", Models: []string{"m1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Providers.SetDefault(p.ID); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

// useScripted 把假模型接到 factory 上，测试结束自动摘除，绝不真联网。
func useScripted(t *testing.T, turns ...llm.Message) *llmtest.Scripted {
	t.Helper()
	s := llmtest.New(turns...)
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer { return s })
	t.Cleanup(func() { factory.SetOverride("test", nil) })
	return s
}

// waitUntil 轮询直到 cond 成立或超时，避免为异步落库写 sleep。
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待 %s 超时", what)
}

func lastRoleIs(r *repo.Repo, sessionID string, role string) func() bool {
	return func() bool {
		entries, err := r.ListEntries(sessionID)
		return err == nil && len(entries) > 0 && entries[len(entries)-1].Role == role
	}
}

func countRole(r *repo.Repo, sessionID string, role string) func() bool {
	return func() bool {
		entries, _ := r.ListEntries(sessionID)
		n := 0
		for _, e := range entries {
			if e.Role == role {
				n++
			}
		}
		return n > 0
	}
}

// 完整发送链路：消息入库 → 内核跑完 → 回复落库。
func TestChatSendPersistsConversation(t *testing.T) {
	useScripted(t, llm.Message{Content: "你好，我是 WorkBaby"})
	env, svc := newEnv(t)
	sess := newProviderSession(t, svc)

	resp, err := svc.Chat.Send(sess.ID, "在吗", nil)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if resp.RunID == "" {
		t.Fatal("应返回 run id")
	}
	waitUntil(t, "助手回复落库", lastRoleIs(env.Repo, sess.ID, llm.RoleAssistant))

	detail, err := svc.Sessions.Detail(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("应落库 2 条消息，实际 %d", len(detail.Messages))
	}
	if detail.Messages[0].Content != "在吗" || detail.Messages[1].Content != "你好，我是 WorkBaby" {
		t.Fatalf("消息内容不对: %+v", detail.Messages)
	}
}

// 审批闭环：模型请求写操作 → 审批卡 → 放行 → 执行并落库；
// 「本会话内都放行」必须真的生效，第二次同类调用不再弹卡。
// 决策只回传布尔值会让会话级放行这条分支永远走不到，因此单独锁住。
func TestApprovalLoop(t *testing.T) {
	t.Run("单次放行后工具执行并落库", func(t *testing.T) {
		useScripted(t,
			llm.Message{Content: "我来操作", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{Content: "完成了"},
		)
		env, svc := newEnv(t, approveTool{})
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "帮我执行危险操作", nil); err != nil {
			t.Fatal(err)
		}
		var pending []domain.ApprovalVO
		waitUntil(t, "审批出现", func() bool {
			pending, _ = svc.Approvals.Pending(sess.ID)
			return len(pending) == 1
		})
		if err := svc.Approvals.Decide(pending[0].ID, true, "once"); err != nil {
			t.Fatalf("审批失败: %v", err)
		}
		waitUntil(t, "工具结果落库", countRole(env.Repo, sess.ID, llm.RoleTool))
	})

	t.Run("会话级放行后不再弹卡", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "risky"}}},
			llm.Message{Content: "两次都做完了"},
		)
		env, svc := newEnv(t, approveTool{})
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "连做两次", nil); err != nil {
			t.Fatal(err)
		}
		var first domain.ApprovalVO
		waitUntil(t, "首条审批出现", func() bool {
			list, _ := svc.Approvals.Pending(sess.ID)
			if len(list) == 1 {
				first = list[0]
			}
			return first.ID != ""
		})
		if err := svc.Approvals.Decide(first.ID, true, domain.ApprovalScopeSession); err != nil {
			t.Fatalf("审批失败: %v", err)
		}
		waitUntil(t, "两次工具结果落库", func() bool {
			entries, _ := env.Repo.ListEntries(sess.ID)
			n := 0
			for _, e := range entries {
				if e.Role == llm.RoleTool {
					n++
				}
			}
			return n == 2
		})
		if pending, _ := svc.Approvals.Pending(sess.ID); len(pending) != 0 {
			t.Fatalf("会话级放行后不该再问，实际仍有 %d 条待审批", len(pending))
		}
	})
}

// 错误轮的半成品必须落库：UI 已流式显示的内容不能在刷新后凭空消失。
func TestChatErrorPersistsPartialContent(t *testing.T) {
	s := useScripted(t, llm.Message{Content: "占位"})
	s.ErrAfterDelta = "我先查一下——"
	env, svc := newEnv(t)
	sess := newProviderSession(t, svc)

	if _, err := svc.Chat.Send(sess.ID, "帮我看看", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "半成品落库", countRole(env.Repo, sess.ID, llm.RoleAssistant))

	detail, err := svc.Sessions.Detail(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("应落库 user + 半成品共 2 条，实际 %d", len(detail.Messages))
	}
	if detail.Messages[1].Content != "我先查一下——" {
		t.Fatalf("半成品正文不符: %q", detail.Messages[1].Content)
	}
}

// 插话落库两态：空闲时直接落库（没有 run 来消费队列，不落就永远丢）；
// 运行中在轮间注入时落库，链序必须是 上一轮结果 → 插话 → 本轮回复。
func TestChatSteerPersists(t *testing.T) {
	t.Run("空闲时直接落库", func(t *testing.T) {
		useScripted(t, llm.Message{Content: "好的"})
		env, svc := newEnv(t)
		sess := newProviderSession(t, svc)

		if err := svc.Chat.Steer(sess.ID, "记得用中文回复"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "插话落库", countRole(env.Repo, sess.ID, llm.RoleUser))

		detail, err := svc.Sessions.Detail(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Messages) != 1 || detail.Messages[0].Content != "记得用中文回复" {
			t.Fatalf("插话应作为唯一 user 条目落库: %+v", detail.Messages)
		}
	})

	t.Run("运行中按注入链序落库", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{Content: "完成了"},
		)
		env, svc := newEnv(t, approveTool{})
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "帮我执行", nil); err != nil {
			t.Fatal(err)
		}
		var pending []domain.ApprovalVO
		waitUntil(t, "审批出现", func() bool {
			pending, _ = svc.Approvals.Pending(sess.ID)
			return len(pending) == 1
		})
		if err := svc.Chat.Steer(sess.ID, "顺便说一句"); err != nil {
			t.Fatal(err)
		}
		if err := svc.Approvals.Decide(pending[0].ID, true, "once"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "回复落库", lastRoleIs(env.Repo, sess.ID, llm.RoleAssistant))

		detail, err := svc.Sessions.Detail(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		var roles []string
		for _, m := range detail.Messages {
			roles = append(roles, m.Role)
		}
		want := []string{llm.RoleUser, llm.RoleAssistant, llm.RoleTool, llm.RoleUser, llm.RoleAssistant}
		if len(roles) != len(want) {
			t.Fatalf("链路条数不符: %v", roles)
		}
		for i := range want {
			if roles[i] != want[i] {
				t.Fatalf("链序不符: got %v want %v", roles, want)
			}
		}
	})
}
