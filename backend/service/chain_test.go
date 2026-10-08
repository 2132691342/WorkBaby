// 服务层链路：一次 run 的落库判据（审批 / 错误轮 / 插话 / 预算）、工具声明与结果的配对、
// 默认模型的继承与回填。装配较贵，父测试只做一次。
package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"WorkBaby/backend/config"
	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/factory"
	"WorkBaby/backend/llm/llmtest"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/runtime/runtimetest"
	"WorkBaby/backend/tool"
)

// approveTool 永远要求审批且无副作用，用来打通审批闭环。
type approveTool struct{}

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

// parallelTool 声明为并发执行，用来打中「同一轮多个工具」这条路径；
// 执行里停一下，把两个 goroutine 在飞的时间窗撑开。
type parallelTool struct{}

func (parallelTool) Name() string               { return "par" }
func (parallelTool) Label() string              { return "并发操作" }
func (parallelTool) Description() string        { return "测试" }
func (parallelTool) PromptSnippet() string      { return "测试" }
func (parallelTool) PromptGuidelines() []string { return nil }
func (parallelTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (parallelTool) ExecutionMode() tool.ExecutionMode { return tool.ExecutionParallel }
func (parallelTool) RequiresApproval() bool            { return false }
func (parallelTool) Execute(ctx context.Context, in tool.Input) (*tool.Result, error) {
	time.Sleep(20 * time.Millisecond)
	return &tool.Result{Content: "done", Title: "并发操作"}, nil
}

// newEnv 起一套干净的依赖：DB + 工具注册表 + 服务容器，并完成 Bootstrap。
func newEnv(t *testing.T, register ...tool.Tool) (*Env, *Container) {
	t.Helper()
	dir := t.TempDir()
	runtimetest.SeedMarkers(t, dir)
	gdb, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool, _ := gdb.DB()
		_ = pool.Close()
	})
	paths := runtime.Paths{
		DataDir: dir, LogDir: dir, TmpDir: dir, SkillsDir: filepath.Join(dir, "skills"),
		RuntimeDir: filepath.Join(dir, "runtime"),
		PythonDir:  filepath.Join(dir, "runtime", "python"),
		PowerShellDir: filepath.Join(dir, "runtime", "powershell"),
	}
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

// countRole 统计某种角色的落库条目数是否达到 want。
func countRole(r *repo.Repo, sessionID, role string, want int) func() bool {
	return func() bool {
		entries, err := r.ListEntries(sessionID)
		if err != nil {
			return false
		}
		n := 0
		for _, e := range entries {
			if e.Role == role {
				n++
			}
		}
		return n >= want
	}
}

// lastRoleIs 判断最后一条落库条目的角色。
func lastRoleIs(r *repo.Repo, sessionID, role string) func() bool {
	return func() bool {
		entries, err := r.ListEntries(sessionID)
		return err == nil && len(entries) > 0 && entries[len(entries)-1].Role == role
	}
}

// assertDeclaredBeforeResult 断言每条工具结果都落在「最近一条 assistant 的声明窗口」里。
func assertDeclaredBeforeResult(t *testing.T, svc *Container, sessionID string) {
	t.Helper()
	msgs, err := svc.Sessions.History(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	open := map[string]bool{}
	for i, m := range msgs {
		switch m.Role {
		case llm.RoleAssistant:
			open = map[string]bool{}
			for _, tc := range m.ToolCalls {
				open[tc.ID] = true
			}
		case llm.RoleTool:
			if !open[m.ToolCallID] {
				t.Fatalf("第 %d 条工具结果 %q 不在最近一条 assistant 的声明里，上游会 400\n全链：%s",
					i, m.ToolCallID, describe(msgs))
			}
		default:
			open = map[string]bool{}
		}
	}
}

func describe(msgs []llm.Message) string {
	out := ""
	for _, m := range msgs {
		out += m.Role
		if m.Role == llm.RoleTool {
			out += "(" + m.ToolCallID + ")"
		}
		for _, tc := range m.ToolCalls {
			out += "[decl:" + tc.ID + "]"
		}
		out += " → "
	}
	return out
}

// 一次 run 的生命周期：发起 → 工具 / 审批 / 插话 → 落库。
// 四个分支共用一次装配，各自断言一条会在界面上「看得见」的判据。
func TestChatRunChain(t *testing.T) {
	// 装配较贵（DB + 注册表 + 服务容器），四个分支共用一套；
	// 脚本替身很便宜，各分支按需挂载自己的那一版。
	env, svc := newEnv(t, approveTool{})

	// 会话级放行必须真的生效：第二次同类调用不再弹卡，否则用户要点两遍。
	t.Run("审批闭环（会话级放行）", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "risky"}}},
			llm.Message{Content: "两次都做完了"},
		)
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
		waitUntil(t, "两次工具结果落库", countRole(env.Repo, sess.ID, llm.RoleTool, 2))
		if pending, _ := svc.Approvals.Pending(sess.ID); len(pending) != 0 {
			t.Fatalf("会话级放行后不该再问，实际仍有 %d 条待审批", len(pending))
		}
	})

	// 错误轮的半成品必须落库：UI 已流式显示的内容，刷新后不能凭空消失。
	t.Run("错误轮半成品落库", func(t *testing.T) {
		s := useScripted(t, llm.Message{Content: "占位"})
		s.ErrAfterDelta = "我先查一下——"
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "帮我看看", nil); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "半成品落库", countRole(env.Repo, sess.ID, llm.RoleAssistant, 1))

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
	})

	// 插话必须按「上一轮结果 → 插话 → 本轮回复」落库：
	// 早写会插进 assistant(tool_calls) 与工具结果之间撕裂配对，晚写则链序失真。
	t.Run("插话按注入链序落库", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{Content: "完成了"},
		)
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

	// 输出预算必须真的下发到上游：不下发时各家网关各按自己的默认（常见 4096/8192），
	// 带思考的模型把预算全花在推理上，用户看到的就是「助手只思考、没有任何动作」。
	t.Run("输出预算下发且余量跟得上", func(t *testing.T) {
		s := useScripted(t, llm.Message{Content: "好了"})
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "你好", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		waitUntil(t, "回复落库", countRole(env.Repo, sess.ID, domain.RoleAssistant, 1))

		if len(s.Requests) == 0 {
			t.Fatal("没有发出上游请求")
		}
		if s.Requests[0].MaxTokens <= 0 {
			t.Fatalf("max_tokens 未下发（%d）：上游会用自己的默认值把回答截断", s.Requests[0].MaxTokens)
		}
		// 上下文余量必须容得下真正下发的输出预算：余量算小了，压缩会按虚高的
		// 空间往窗口里塞内容，总占用顶破窗口。
		const big = "gpt-4.1"
		b := svc.Chat.budget("", big, "sys")
		if want := domain.ModelCapabilityOf(big).MaxOutput; b.Reserve < want {
			t.Fatalf("上下文余量 %d 没跟上输出预算 %d：压缩会按虚高的空间往窗口里塞内容", b.Reserve, want)
		}
	})
}

// 工具声明与结果的落库配对：跨轮逐个放行与同轮并发两种形态都要成立。
// emit 不串行化时第二个 goroutine 撞主键，那条声明永远丢在链外。
func TestToolResultOrdering(t *testing.T) {
	t.Run("跨轮声明先于结果", func(t *testing.T) {
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
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if list, _ := svc.Approvals.Pending(sess.ID); len(list) > 0 {
				_ = svc.Approvals.Decide(list[0].ID, true, "once")
			}
			if countRole(env.Repo, sess.ID, llm.RoleTool, 2)() {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}
		waitUntil(t, "两条工具结果落库", countRole(env.Repo, sess.ID, llm.RoleTool, 2))
		assertDeclaredBeforeResult(t, svc, sess.ID)
	})

	t.Run("同轮并发保序且声明齐全", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "par"}, {ID: "c2", Name: "par"}}},
			llm.Message{Content: "两个都跑完了"},
		)
		env, svc := newEnv(t, parallelTool{})
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "同时做两个", nil); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "两条工具结果与收尾都落库", func() bool {
			entries, _ := env.Repo.ListEntries(sess.ID)
			tools, assistants := 0, 0
			for _, e := range entries {
				switch e.Role {
				case llm.RoleTool:
					tools++
				case llm.RoleAssistant:
					assistants++
				}
			}
			return tools == 2 && assistants >= 2
		})

		entries, _ := env.Repo.ListEntries(sess.ID)
		declared := map[string]bool{}
		for _, e := range entries {
			var p domain.MessagePayload
			if err := json.Unmarshal([]byte(e.PayloadJSON), &p); err != nil {
				continue
			}
			for _, tc := range p.ToolCalls {
				declared[tc.ID] = true
			}
		}
		for _, want := range []string{"c1", "c2"} {
			if !declared[want] {
				t.Fatalf("工具 %q 的声明没落库（多半是主键冲突顶掉了）", want)
			}
		}
		assertDeclaredBeforeResult(t, svc, sess.ID)
	})
}

// 默认模型的继承链：设默认服务 → 落默认模型 → 新会话继承 → 存量空模型会话跑一次补齐。
// 这条链断掉时界面显示「默认模型」且上下文窗口永远算不出来，症状分散在多处。
func TestDefaultModelChain(t *testing.T) {
	useScripted(t, llm.Message{Content: "好的"})
	env, svc := newEnv(t)

	p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{
		Name: "m", API: "test", Models: []string{"MiniMax-M3", "gpt-4o"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Providers.SetDefault(p.ID); err != nil {
		t.Fatal(err)
	}
	// 只存服务不存模型时，能力查询与上下文水位全失效。
	if got, _ := env.Repo.GetSetting(domain.SettingDefaultModel); got != "MiniMax-M3" {
		t.Fatalf("设默认服务后应落默认模型，实际 %q", got)
	}

	sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.ProviderID != p.ID {
		t.Fatalf("会话没继承到默认服务: %q", sess.ProviderID)
	}
	if sess.Model == "" {
		t.Fatal("新会话没有继承到模型名，输入区会一直显示「默认模型」")
	}

	// 存量会话（模型名为空）跑一次 run 必须就地补齐。
	if err := env.Repo.UpdateSessionColumns(sess.ID, map[string]any{"model": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Chat.Send(sess.ID, "在吗", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "模型名回写", func() bool {
		fresh, err := env.Repo.GetSession(sess.ID)
		return err == nil && fresh.Model != ""
	})
	fresh, _ := env.Repo.GetSession(sess.ID)
	if fresh.Model != "MiniMax-M3" {
		t.Fatalf("模型名未回写: %q", fresh.Model)
	}
}
