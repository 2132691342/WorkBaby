// 覆盖服务层集成链路：建会话 → 发消息 → 内核跑完落库；写操作审批的闭环。
package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"WorkBaby/internal/config"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/db"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/factory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool"
)

type fakeStream struct {
	turns []llm.Message
	idx   int
}

func (s *fakeStream) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	i := s.idx
	s.idx++
	ch := make(chan llm.Event, 8)
	go func() {
		defer close(ch)
		if i >= len(s.turns) {
			ch <- llm.Event{Type: llm.EventDone, StopReason: llm.StopStop, Usage: &llm.Usage{Total: 9}}
			return
		}
		m := s.turns[i]
		if m.Content != "" {
			ch <- llm.Event{Type: llm.EventDelta, Delta: m.Content}
		}
		for _, tc := range m.ToolCalls {
			cp := tc
			ch <- llm.Event{Type: llm.EventToolCall, ToolCall: &cp}
		}
		stop := llm.StopStop
		if len(m.ToolCalls) > 0 {
			stop = llm.StopToolUse
		}
		ch <- llm.Event{Type: llm.EventDone, StopReason: stop, Usage: &llm.Usage{Total: 9}}
	}()
	return ch, nil
}

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

func newEnv(t *testing.T) (*Env, *Container) {
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
	em := NewEmitter()
	paths := runtime.Paths{DataDir: dir, LogDir: dir, TmpDir: dir, SkillsDir: filepath.Join(dir, "skills")}
	_ = pkg.EnsureDir(paths.SkillsDir)
	env := &Env{
		Repo: repo.New(gdb), Paths: paths, Emitter: em, Registry: tool.New(),
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

func waitAssistant(t *testing.T, r *repo.Repo, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := r.ListEntries(sessionID)
		if err == nil && len(entries) > 0 && entries[len(entries)-1].Role == llm.RoleAssistant {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("等待助手回复超时")
}

func TestChatSendPersistsConversation(t *testing.T) {
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer {
		return &fakeStream{turns: []llm.Message{{Content: "你好，我是 WorkBaby"}}}
	})
	defer factory.SetOverride("test", nil)

	env, svc := newEnv(t)
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
	resp, err := svc.Chat.Send(sess.ID, "在吗", nil)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if resp.RunID == "" {
		t.Fatal("应返回 run id")
	}
	waitAssistant(t, env.Repo, sess.ID)

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

func TestApprovalFlowPersistsAndRunsAfterApprove(t *testing.T) {
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer {
		return &fakeStream{turns: []llm.Message{
			{Content: "我来操作", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			{Content: "完成了"},
		}}
	})
	defer factory.SetOverride("test", nil)

	env, svc := newEnv(t)
	env.Registry.Register(approveTool{})

	p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{Name: "t", API: "test", Models: []string{"m1"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.Providers.SetDefault(p.ID)

	sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Chat.Send(sess.ID, "帮我执行危险操作", nil); err != nil {
		t.Fatal(err)
	}

	// 等审批出现
	deadline := time.Now().Add(5 * time.Second)
	var list []domain.ApprovalVO
	for time.Now().Before(deadline) {
		list, _ = svc.Approvals.Pending(sess.ID)
		if len(list) == 1 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if len(list) != 1 {
		t.Fatalf("应产生一条待审批，实际 %d", len(list))
	}
	if err := svc.Approvals.Decide(list[0].ID, true, "once"); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	waitAssistant(t, env.Repo, sess.ID)

	entries, _ := env.Repo.ListEntries(sess.ID)
	found := false
	for _, e := range entries {
		if e.Role == llm.RoleTool {
			found = true
		}
	}
	if !found {
		t.Fatal("审批放行后工具结果应落库")
	}
}

// 「本会话内都放行」必须真的生效：第二次同类调用不再产生新的审批卡。
// 决策只回传布尔值会让这条分支永远走不到，因此单独锁一条测试。
func TestApprovalSessionScopeSkipsSecondPrompt(t *testing.T) {
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer {
		return &fakeStream{turns: []llm.Message{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "risky"}}},
			{Content: "两次都做完了"},
		}}
	})
	defer factory.SetOverride("test", nil)

	env, svc := newEnv(t)
	env.Registry.Register(approveTool{})

	p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{Name: "t", API: "test", Models: []string{"m1"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.Providers.SetDefault(p.ID)

	sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Chat.Send(sess.ID, "连做两次", nil); err != nil {
		t.Fatal(err)
	}

	first := waitPending(t, svc, sess.ID)
	if err := svc.Approvals.Decide(first.ID, true, domain.ApprovalScopeSession); err != nil {
		t.Fatalf("审批失败: %v", err)
	}
	waitAssistant(t, env.Repo, sess.ID)

	if pending, _ := svc.Approvals.Pending(sess.ID); len(pending) != 0 {
		t.Fatalf("会话级放行后不该再问，实际仍有 %d 条待审批", len(pending))
	}
	entries, _ := env.Repo.ListEntries(sess.ID)
	tools := 0
	for _, e := range entries {
		if e.Role == llm.RoleTool {
			tools++
		}
	}
	if tools != 2 {
		t.Fatalf("两次工具结果都应落库，实际 %d", tools)
	}
}

func waitPending(t *testing.T, svc *Container, sessionID string) domain.ApprovalVO {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := svc.Approvals.Pending(sessionID)
		if len(list) == 1 {
			return list[0]
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("等待审批出现超时")
	return domain.ApprovalVO{}
}
