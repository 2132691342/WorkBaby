// 服务层链路：一次 run 的完整生命周期（审批 / 错误轮 / 插话 / 预算 / 落库配对）
// 与默认模型的继承回填。装配昂贵（DB + 注册表 + 容器），全文件只做一次。
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

// parallelTool 声明为并发执行；执行里停一下，把多个 goroutine 在飞的时间窗撑开。
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

// newEnv 起一套干净的依赖（DB + 注册表 + 容器）并完成 Bootstrap。
func newEnv(t *testing.T) (*Env, *Container) {
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
		RuntimeDir:    filepath.Join(dir, "runtime"),
		PythonDir:     filepath.Join(dir, "runtime", "python"),
		PowerShellDir: filepath.Join(dir, "runtime", "powershell"),
	}
	_ = pkg.EnsureDir(paths.SkillsDir)
	reg := tool.New()
	reg.Register(approveTool{})
	reg.Register(parallelTool{})
	env := &Env{
		Repo: repo.New(gdb), Paths: paths, Emitter: NewEmitter(), Registry: reg,
		Cfg: &config.Config{Workspace: dir},
	}
	env.SetToolDeps(NewToolDeps(paths, nil))
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

// 服务层唯一一条完整链路：发送 → 工具 / 审批 / 插话 → 落库 → 收尾。
// 所有子测试共用同一套装配；脚本替身很便宜，各自按需挂载。
func TestServiceRunChain(t *testing.T) {
	env, svc := newEnv(t)

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

	// 落库链序：错误轮的半成品必须留下（UI 已流式显示的内容，刷新后不能凭空消失）；
	// 插话必须按「上一轮结果 → 插话 → 本轮回复」落库（早写撕裂协议配对，晚写链序失真）。
	t.Run("落库链序：错误轮半成品 / 插话注入", func(t *testing.T) {
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

		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{Content: "完成了"},
		)
		inject := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(inject.ID, "帮我执行", nil); err != nil {
			t.Fatal(err)
		}
		var pending []domain.ApprovalVO
		waitUntil(t, "审批出现", func() bool {
			pending, _ = svc.Approvals.Pending(inject.ID)
			return len(pending) == 1
		})
		if err := svc.Chat.Steer(inject.ID, "顺便说一句"); err != nil {
			t.Fatal(err)
		}
		if err := svc.Approvals.Decide(pending[0].ID, true, "once"); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "回复落库", lastRoleIs(env.Repo, inject.ID, llm.RoleAssistant))

		injected, err := svc.Sessions.Detail(inject.ID)
		if err != nil {
			t.Fatal(err)
		}
		var roles []string
		for _, m := range injected.Messages {
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
		cap := domain.ModelCapabilityOf(big)
		b := svc.Chat.budget(cap, "sys")
		if want := cap.OutputBudget(); b.Reserve < want {
			t.Fatalf("上下文余量 %d 没跟上输出预算 %d：压缩会按虚高的空间往窗口里塞内容", b.Reserve, want)
		}
		// 1M 窗口的模型按 1/8 会算出 12.5 万输出，超过厂商硬上限就是整轮 400；
		// 输出预算必须被目录里的硬上限钳住。
		if cap.OutputBudget() > cap.MaxOutputLimit || cap.MaxOutputLimit <= 0 {
			t.Fatalf("输出预算 %d 没被厂商硬上限 %d 钳住", cap.OutputBudget(), cap.MaxOutputLimit)
		}
		// 窗口是用户手填的（同名模型能在别的服务上跑，目录里的硬上限描述的不是这个端点）：
		// 输出预算按他填的窗口 1/8 走，不能再被目录的旧上限钳住。
		custom := cap.WithWindow(1024000)
		if got, want := custom.OutputBudget(), 1024000/8; got != want {
			t.Fatalf("手填窗口 %d 的输出预算应为窗口 1/8 = %d，实际 %d（被目录硬上限钳住了）",
				custom.ContextWindow, want, got)
		}
		// 水位环的分母必须与压缩触发线同口径：拿整窗口当分母的话，整理已经发生、
		// 环上才 87%，用户看着「还有空间」却已经在丢内容。
		reserve := cap.OutputBudget()
		if got := ratioOf(cap.ContextWindow-reserve, cap.ContextWindow, reserve); got != 100 {
			t.Fatalf("水位刚到整理线时 ratio 应为 100，实际 %d", got)
		}
		if got := ratioOf(cap.ContextWindow/2, cap.ContextWindow, 0); got != 50 {
			t.Fatalf("没有余量时 ratio 应等于窗口占用率 50，实际 %d", got)
		}
		if got := ratioOf(cap.ContextWindow, cap.ContextWindow, cap.ContextWindow); got != 100 {
			t.Fatalf("余量吃掉整个窗口时该退到半窗兜底并封顶 100，实际 %d", got)
		}
	})

	// 上游把输入报成「未命中缓存的部分」时口径必须补回总量，界面读数才不会越界：
	// 命中量大于输入量会让消息卡片与仪表盘算出 229% 这种数，用户只会以为统计坏了。
	t.Run("用量口径：归一落库与命中率封顶", func(t *testing.T) {
		s := useScripted(t, llm.Message{Content: "好了"})
		s.Usage = &llm.Usage{Input: 100, Output: 20, Total: 120, Cached: 300}
		sess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(sess.ID, "你好", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		waitUntil(t, "回复落库", countRole(env.Repo, sess.ID, domain.RoleAssistant, 1))

		detail, err := svc.Sessions.Detail(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		u := detail.Messages[len(detail.Messages)-1].Usage
		if u == nil {
			t.Fatal("这轮用量没落库，消息底部会读不到 token 数")
		}
		if u.Cached > u.Input {
			t.Fatalf("命中量 %d 大于输入量 %d：界面会算出超过 100%% 的命中率", u.Cached, u.Input)
		}
		if u.Input < 400 {
			t.Fatalf("输入量没补回未命中部分：期望 ≥ 400（100 未命中 + 300 命中），实际 %d", u.Input)
		}
		if u.Context < u.Input {
			t.Fatalf("上下文 %d 小于输入 %d：同一张卡片上两个读数会互相矛盾", u.Context, u.Input)
		}

		// 老数据的命中量可能大于输入量（输入只记了未命中部分），汇总侧必须兜在 100% 以内。
		useScripted(t, llm.Message{Content: "占位"}) // 建服务要过适配器校验，这里只用它注册 test 类型
		legacy := newProviderSession(t, svc)
		if err := env.Repo.AddUsage(&domain.TokenUsageDO{
			ID: pkg.NewID(domain.PrefixUsage), SessionID: legacy.ID,
			Model: "m1", Input: 100, Output: 20, Total: 120, Cached: 300, Context: 100,
		}); err != nil {
			t.Fatal(err)
		}
		st, err2 := svc.Settings.Stats(14)
		if err2 != nil {
			t.Fatal(err2)
		}
		if st.Totals.CacheHitRate > 1 {
			t.Fatalf("命中率读数 %.2f 越过 100%%：用户会以为统计坏了", st.Totals.CacheHitRate)
		}
	})

	// 声明与结果紧邻配对：跨轮交错或同轮并发时，落库顺序错位都会被上游以
	// 「结果找不到紧邻的声明」拒绝整轮；同轮并发还要求每个调用的声明都落进库里。
	t.Run("声明与结果紧邻配对", func(t *testing.T) {
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "risky"}}},
			llm.Message{Content: "两次都做完了"},
		)
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

		// 同轮并发：emit 不串行化时第二个 goroutine 撞主键，那条声明永远丢在链外。
		useScripted(t,
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "par"}, {ID: "c2", Name: "par"}}},
			llm.Message{Content: "两个都跑完了"},
		)
		parSess := newProviderSession(t, svc)

		if _, err := svc.Chat.Send(parSess.ID, "同时做两个", nil); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "两条工具结果与收尾都落库", func() bool {
			entries, _ := env.Repo.ListEntries(parSess.ID)
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

		entries, _ := env.Repo.ListEntries(parSess.ID)
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
		assertDeclaredBeforeResult(t, svc, parSess.ID)
	})

	// 默认模型的继承链：设默认服务 → 落默认模型 → 新会话继承 → 存量空模型会话跑一次补齐。
	// 这条链断掉时界面显示「默认模型」且上下文窗口永远算不出来，症状分散在多处。
	t.Run("默认模型继承与回填", func(t *testing.T) {
		useScripted(t, llm.Message{Content: "好的"})
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
	})
}
