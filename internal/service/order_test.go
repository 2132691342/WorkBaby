// 落库顺序：assistant(tool_calls) 声明必须排在它自己的 tool 结果之前。
//
// 真实故障：工具开始时只把调用记在内存里，声明等 turn_end 才落库，
// 于是工具结果先入链。历史回放时上游收到一条找不到声明的 tool 消息，
// 直接 400：tool result's tool id not found——而且第二轮对话才开始报错，
// 第一轮完全正常，排查时极难关联。
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// parallelTool 无需审批且声明为并发执行，用来打中「同一轮多个工具」这条路径。
// 执行里故意停一下，把两个 goroutine 同时在飞的时间窗撑开。
type parallelTool struct{ ws string }

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

// 跨轮对话里，声明与结果的相对顺序必须始终成立。
func TestToolDeclarationPersistedBeforeResult(t *testing.T) {
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
	// 审批卡一张张冒出来，逐个放行，直到两次调用都跑完。
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if list, _ := svc.Approvals.Pending(sess.ID); len(list) > 0 {
			_ = svc.Approvals.Decide(list[0].ID, true, "once")
		}
		entries, _ := env.Repo.ListEntries(sess.ID)
		n := 0
		for _, e := range entries {
			if e.Role == llm.RoleTool {
				n++
			}
		}
		if n == 2 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	waitUntil(t, "两条工具结果落库", func() bool {
		entries, _ := env.Repo.ListEntries(sess.ID)
		n := 0
		for _, e := range entries {
			if e.Role == llm.RoleTool {
				n++
			}
		}
		return n == 2
	})

	assertDeclaredBeforeResult(t, svc, sess.ID)
}

// 单轮场景：一条声明配一条结果，顺序必须成立。
func TestSingleToolTurnKeepsDeclarationFirst(t *testing.T) {
	useScripted(t,
		llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "risky"}}},
		llm.Message{Content: "做完了"},
	)
	env, svc := newEnv(t, approveTool{})
	sess := newProviderSession(t, svc)

	if _, err := svc.Chat.Send(sess.ID, "做一下", nil); err != nil {
		t.Fatal(err)
	}
	// 审批工具必须先放行才会真正执行，顺序才有意义。
	var pending domain.ApprovalVO
	waitUntil(t, "审批出现", func() bool {
		list, _ := svc.Approvals.Pending(sess.ID)
		if len(list) == 1 {
			pending = list[0]
		}
		return pending.ID != ""
	})
	if err := svc.Approvals.Decide(pending.ID, true, "once"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "工具结果落库", func() bool {
		entries, _ := env.Repo.ListEntries(sess.ID)
		for _, e := range entries {
			if e.Role == llm.RoleTool {
				return true
			}
		}
		return false
	})
	assertDeclaredBeforeResult(t, svc, sess.ID)
}

// 同一轮里并发跑多个工具：事件从多个 goroutine 同时到达，落库位点必须仍是单线程的。
//
// 真实故障：emit 是同步调用，只继承调用方的 goroutine。并行工具各自 emit 时，
// 两个 goroutine 读到同一个 turnEntryID，先后用同一个 id 落库，第二个撞主键，
// 那条 assistant 声明永远丢在链外——历史回放时上游直接 400。
func TestParallelToolsInOneTurn(t *testing.T) {
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

	// 声明丢了就是主键冲突把 Append 顶掉了，这里按 id 数量再确认一次。
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
}

// 直接对历史做协议体检：任何一条 tool 结果前面必须有对应的 assistant 声明。
func assertDeclaredBeforeResult(t *testing.T, svc *Container, sessionID string) {	t.Helper()
	msgs, err := svc.Sessions.History(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for i, m := range msgs {
		if m.Role == llm.RoleAssistant {
			for _, tc := range m.ToolCalls {
				declared[tc.ID] = true
			}
			continue
		}
		if m.Role != llm.RoleTool {
			continue
		}
		if !declared[m.ToolCallID] {
			t.Fatalf("第 %d 条 tool 结果 %q 出现在它的 assistant 声明之前，上游会 400\n全链：%s",
				i, m.ToolCallID, describe(msgs))
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
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				out += "[decl:" + tc.ID + "]"
			}
		}
		out += " → "
	}
	return out
}
