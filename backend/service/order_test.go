// 落库顺序链路：assistant(tool_calls) 的声明必须排在它自己的 tool 结果之前，
// 跨轮与同轮并发两种形态都要成立。声明晚于结果入链时，历史回放的上游会直接 400。
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/tool"
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

// 落库顺序链路：跨轮与同轮并发两种形态都要成立，且工具声明不能丢。
func TestToolResultOrdering(t *testing.T) {
	// 跨轮：审批卡一张张冒出来，逐个放行，直到两次调用都跑完。
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
	})

	// 同一轮里并发跑多个工具：事件从多个 goroutine 同时到达，落库位点必须仍是单线程的。
	// emit 不串行化时，两个 goroutine 先后用同一个 id 落库，第二个撞主键，
	// 那条 assistant 声明永远丢在链外——历史回放时上游直接 400。
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
	})
}

// 直接对历史做协议体检：每条工具结果必须落在「最近一条 assistant 的声明窗口」里。
// 上游只认紧邻配对，结果与声明之间隔着另一条 assistant 或 user 消息都会被拒，
// 所以断言必须比「前面某处声明过」更严格。
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
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				out += "[decl:" + tc.ID + "]"
			}
		}
		out += " → "
	}
	return out
}
