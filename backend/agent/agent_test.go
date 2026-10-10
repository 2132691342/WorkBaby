// 内核链路：主循环的协议约束（结果顺序回填、收尾路径、降级重试、事件串行化、重复调用拦截）
// 与上下文裁剪的硬约束（清洗、切点、预算边界）。坏了的表现：上游 400、半截内容落库、事件错位。
package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/llmtest"
	"WorkBaby/backend/tool"
)

// echoTool 回显调用名与延时，用于断言执行次数与结果顺序。
type echoTool struct {
	name  string
	mode  tool.ExecutionMode
	seen  *[]string
	delay time.Duration
	err   error
}

func (t echoTool) Name() string               { return t.name }
func (t echoTool) Label() string              { return t.name }
func (t echoTool) Description() string        { return "测试工具" }
func (t echoTool) PromptSnippet() string      { return "测试" }
func (t echoTool) PromptGuidelines() []string { return nil }
func (t echoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t echoTool) ExecutionMode() tool.ExecutionMode { return t.mode }
func (t echoTool) RequiresApproval() bool            { return false }

func (t echoTool) Execute(ctx context.Context, in tool.Input) (*tool.Result, error) {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	if t.seen != nil {
		*t.seen = append(*t.seen, t.name)
	}
	if t.err != nil {
		return nil, t.err
	}
	return &tool.Result{Content: t.name + "-ok", Title: t.name}, nil
}

// toolIDs 按出现顺序收集 tool 消息的 ToolCallID。
func toolIDs(msgs []llm.Message) []string {
	var out []string
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			out = append(out, m.ToolCallID)
		}
	}
	return out
}

// callAssistant 造一条带工具调用的助手消息。
func callAssistant(id string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: "ls"}}}
}

// toolResult 造一条对应的工具结果。
func toolResult(id, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: content}
}

// assertNoOrphanToolResults 断言每条工具结果都落在最近一条 assistant 的声明窗口里：
// 上游只认紧邻配对，结果与声明之间隔着别的消息都会被 400。
func assertNoOrphanToolResults(t *testing.T, msgs []llm.Message, label string) {
	t.Helper()
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
				t.Fatalf("%s：第 %d 条工具结果 %q 不在最近一条 assistant 的声明里，上游会 400",
					label, i, m.ToolCallID)
			}
		default:
			open = map[string]bool{}
		}
	}
}

// 主循环的协议约束：结果按声明顺序回填，收尾路径不越界，降级重试有上限，
// 并行工具的事件串行出门。这些都是协议硬约束，错位即整轮 400。
func TestLoopChain(t *testing.T) {
	// 失败原因必须同时给模型、界面与日志：只写进给模型的那份，工具卡展开是空白、
	// 日志只剩 ok=false，谁都查不出为什么。
	t.Run("工具失败带原因", func(t *testing.T) {
		var ends []Event
		streamer := llmtest.New(
			llm.Message{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "bad"}}},
			llm.Message{Content: "换个办法"},
		)
		loop := New(Config{
			Streamer: streamer,
			Tools:    []tool.Tool{echoTool{name: "bad", err: errors.New("路径不存在")}},
			Model:    "test",
			Emit: func(e Event) {
				if e.Kind == EventToolEnd {
					ends = append(ends, e)
				}
			},
		}, nil)
		if _, err := loop.Run(context.Background()); err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		if len(ends) != 1 || ends[0].ToolOK {
			t.Fatalf("工具报错时应有且只有一条 ToolOK=false 的结束事件: %+v", ends)
		}
		if !strings.Contains(ends[0].ToolOutput, "路径不存在") {
			t.Fatalf("失败原因没进事件输出（工具卡与日志都会是空白）: %q", ends[0].ToolOutput)
		}
	})

	// 收尾路径：取消 / 截断 / 上游报错都必须如实收尾，且截断轮的工具调用不执行、不重试。
	t.Run("四条收尾路径", func(t *testing.T) {
		canceled := New(Config{Streamer: llmtest.New(llm.Message{Content: "开始"}), Model: "test"}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res, err := canceled.Run(ctx)
		if err != nil || res.StopReason != StopAborted {
			t.Fatalf("取消应以 aborted 收尾: %+v %v", res, err)
		}

		var seen []string
		truncated := llmtest.New(llm.Message{
			Content:   "被截断",
			ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}},
		})
		truncated.Stops = []string{llm.StopLength}
		loop := New(Config{
			Streamer: truncated,
			Tools:    []tool.Tool{echoTool{name: "one", seen: &seen}},
			Model:    "test",
		}, nil)
		res, _ = loop.Run(context.Background())
		if len(seen) != 0 {
			t.Fatal("被截断的响应里工具不该被执行")
		}
		if res.StopReason != StopLength || truncated.Calls() != 1 {
			t.Fatalf("截断应以 length 收尾且不重试: stop=%s calls=%d", res.StopReason, truncated.Calls())
		}

		// 零产出截断同样不重试：预算由 service 层给定，内核悄悄重试只会在上游上限更低时换来 400。
		thinking := llmtest.New(llm.Message{Thinking: "先想想该怎么做。"})
		thinking.Stops = []string{llm.StopLength}
		loop2 := New(Config{Streamer: thinking, Model: "test", MaxTokens: 32768}, nil)
		res2, err := loop2.Run(context.Background())
		if err != nil || res2.StopReason != StopLength || thinking.Calls() != 1 {
			t.Fatalf("零产出截断应以 length 收尾且不重试: stop=%s calls=%d err=%v",
				res2.StopReason, thinking.Calls(), err)
		}

		failed := llmtest.New(llm.Message{Content: "半句"})
		failed.Err = errors.New("上游 500")
		loop3 := New(Config{Streamer: failed, Model: "test"}, nil)
		res3, err := loop3.Run(context.Background())
		if err == nil || res3 == nil || res3.StopReason != StopError {
			t.Fatalf("上游报错应向上返回并以 error 收尾: %+v %v", res3, err)
		}
	})

	// 上游拒绝输出预算时降级重试：窗口是用户手填的、模型是网关转发的，本地算不出上游
	// 真实上限，而报错里通常写着真实上限，照着改一次就能过；到下限则放弃。
	t.Run("输出预算被拒时降级", func(t *testing.T) {
		streamer := &llmtest.Scripted{
			Turns: []llm.Message{{Content: "不该走到这里"}, {Content: "降级后成功"}},
			Errs:  []error{errors.New("Invalid max_tokens value: 128000 > 32768")},
		}
		loop := New(Config{Streamer: streamer, Model: "test", MaxTokens: 128000}, nil)
		res, err := loop.Run(context.Background())
		if err != nil {
			t.Fatalf("降级重试后应成功: %v", err)
		}
		if streamer.Calls() != 2 {
			t.Fatalf("期望重试一次，实际发起 %d 次请求", streamer.Calls())
		}
		if got := streamer.Requests[0].MaxTokens; got != 128000 {
			t.Fatalf("首次下发的预算不该被改动，实际 %d", got)
		}
		if got := streamer.Requests[1].MaxTokens; got != 32768 {
			t.Fatalf("重试的预算应取上游报出的上限 32768，实际 %d", got)
		}
		if got := res.Messages[len(res.Messages)-1].Content; got != "降级后成功" {
			t.Fatalf("重试产出的正文没进结果: %q", got)
		}

		// 上游不给数字（只说太长了）时对折，对折到「短到没用」就不再试：
		// 反复试只会把一次可读的失败拖成多次无用往返。
		noNumber := llmtest.New(llm.Message{Content: "不该走到这里"})
		noNumber.Err = errors.New("max_tokens is too large")
		loop2 := New(Config{Streamer: noNumber, Model: "test", MaxTokens: 6000}, nil)
		if _, err := loop2.Run(context.Background()); err == nil {
			t.Fatal("降级到下限后仍被拒，应把上游原话返回")
		}
		if noNumber.Calls() != 1 {
			t.Fatalf("预算 6000 对折会低于下限，不该重试，实际发起 %d 次请求", noNumber.Calls())
		}
	})

	// 模型卡在同一个调用上打转时按重复上限拦下，并把失败结果喂回去让它换方案；
	// 被拦的调用也必须留下完整的开始 / 结束事件对，否则重载历史后协议配对就断了。
	t.Run("重复调用拦到上限", func(t *testing.T) {
		var seen []string
		call := func(id string) llm.ToolCall {
			return llm.ToolCall{ID: id, Name: "one", Args: map[string]any{"path": "a.txt"}}
		}
		streamer := llmtest.New(
			llm.Message{ToolCalls: []llm.ToolCall{call("c1")}},
			llm.Message{ToolCalls: []llm.ToolCall{call("c2")}},
			llm.Message{ToolCalls: []llm.ToolCall{call("c3")}},
			llm.Message{ToolCalls: []llm.ToolCall{call("c4")}},
			llm.Message{Content: "换了个办法"},
		)
		var events []Event
		loop := New(Config{
			Streamer: streamer,
			Tools:    []tool.Tool{echoTool{name: "one", seen: &seen}},
			Model:    "test",
			Emit:     func(e Event) { events = append(events, e) },
		}, nil)
		if _, err := loop.Run(context.Background()); err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		if len(seen) != repeatCallLimit {
			t.Fatalf("期望只执行 %d 次，实际 %d", repeatCallLimit, len(seen))
		}
		for _, m := range loop.Messages() {
			if m.Role == llm.RoleTool && m.ToolCallID == "c4" && !m.IsError {
				t.Fatal("超出重复上限的调用应当判失败")
			}
		}
		startIdx, endIdx := -1, -1
		for i, e := range events {
			if e.ToolCall == nil || e.ToolCall.ID != "c4" {
				continue
			}
			if e.Kind == EventToolStart && startIdx < 0 {
				startIdx = i
			}
			if e.Kind == EventToolEnd {
				endIdx = i
			}
		}
		if startIdx < 0 || endIdx < 0 || startIdx > endIdx {
			t.Fatalf("被拦调用 c4 的事件对不完整: start=%d end=%d", startIdx, endIdx)
		}
	})

	// 回填顺序是内核最险的一段：串行多轮逐轮推进，并行批次三个工具在各自 goroutine
	// 里执行与发事件——结果回填必须仍按声明顺序（否则上游 400），事件出口必须串行
	// （上层靠「同一时刻只有一条事件在处理」维护落库位点）。
	t.Run("回填顺序与轮数（串行 + 并行）", func(t *testing.T) {
		// 串行多轮：两个工具分两轮调用，中间穿插助手正文。
		var seen []string
		serial := New(Config{
			Streamer: llmtest.New(
				llm.Message{Content: "先看看", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}}},
				llm.Message{Content: "再看", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "two"}}},
				llm.Message{Content: "做完了"},
			),
			Tools: []tool.Tool{echoTool{name: "one", seen: &seen}, echoTool{name: "two", seen: &seen}},
			Model: "test",
		}, nil)
		res, err := serial.Run(context.Background())
		if err != nil {
			t.Fatalf("串行运行失败: %v", err)
		}
		if res.Turns != 3 || len(seen) != 2 {
			t.Fatalf("串行三轮回填异常：turns=%d，执行了 %d 个工具", res.Turns, len(seen))
		}
		if got := toolIDs(serial.Messages()); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
			t.Fatalf("串行结果回填乱序: %v（声明顺序 c1,c2）", got)
		}

		// 并行批次：完成顺序 fast → mid → slow，回填仍须按 a,b,c。
		var inFlight, peak int32
		par := New(Config{
			Streamer: llmtest.New(
				llm.Message{ToolCalls: []llm.ToolCall{
					{ID: "a", Name: "slow"}, {ID: "b", Name: "fast"}, {ID: "c", Name: "mid"},
				}},
				llm.Message{Content: "好了"},
			),
			Tools: []tool.Tool{
				echoTool{name: "slow", mode: tool.ExecutionParallel, delay: 30 * time.Millisecond},
				echoTool{name: "fast", mode: tool.ExecutionParallel},
				echoTool{name: "mid", mode: tool.ExecutionParallel, delay: 15 * time.Millisecond},
			},
			Model:    "test",
			Parallel: 4,
			Emit: func(Event) {
				n := atomic.AddInt32(&inFlight, 1)
				for {
					old := atomic.LoadInt32(&peak)
					if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
						break
					}
				}
				// 撑开窗口：没有锁时几个 goroutine 必然同时在飞
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
			},
		}, nil)
		if _, err := par.Run(context.Background()); err != nil {
			t.Fatalf("并行运行失败: %v", err)
		}
		got := toolIDs(par.Messages())
		for i, want := range []string{"a", "b", "c"} {
			if i >= len(got) || got[i] != want {
				t.Fatalf("并行结果回填乱序: %v（声明顺序 a,b,c）", got)
			}
		}
		if peak > 1 {
			t.Fatalf("事件出口同时进入了 %d 个事件，上层落库位点会被读脏", peak)
		}
	})
}

// 裁剪与清洗的硬约束：输出必须能直发上游，切点必须落在完整 turn 边界。
func TestCompactProtocol(t *testing.T) {
	// CleanForProtocol 的三条硬约束：空 assistant 与孤儿结果必须剔除（上游 400），
	// 相邻的并行声明必须合并成一条——逐条落库会留下 A(c1) → A(c2) → T(c1) → T(c2)，
	// T(c1) 与它的声明之间隔着 A(c2)，上游以 tool result's tool id not found 拒绝。
	t.Run("清洗与声明合并：空 assistant / 孤儿结果 / 拆散声明", func(t *testing.T) {
		orphan := []llm.Message{
			{Role: llm.RoleUser, Content: "在吗"},
			{Role: llm.RoleAssistant},
			{Role: llm.RoleTool, ToolCallID: "gone", Content: "孤儿结果"},
		}
		if out := CleanForProtocol(orphan); len(out) != 1 || out[0].Content != "在吗" {
			t.Fatalf("空 assistant 与孤儿结果应被剔除: %+v", out)
		}

		split := []llm.Message{
			{Role: llm.RoleUser, Content: "两件事一起做"},
			callAssistant("c1"),
			callAssistant("c2"),
			toolResult("c1", "结果一"),
			toolResult("c2", "结果二"),
		}
		out := CleanForProtocol(split)
		if len(out) != 4 {
			t.Fatalf("相邻 assistant 应合并成一条，实际 %d 条", len(out))
		}
		if out[1].Role != llm.RoleAssistant || len(out[1].ToolCalls) != 2 {
			t.Fatalf("合并后的声明应带全部调用: %+v", out[1])
		}
		assertNoOrphanToolResults(t, out, "CleanForProtocol")
	})

	// keepTokens 从 0 扫到大，让切点落在每一个可能的位置上：结果都必须协议合法，
	// 且保留侧的第一条必须是 user（切在回合中间会把半截 turn 发给上游）。
	// 顺带覆盖预算边界：没超不动、切不出完整 turn 就放弃、降级只留最近整轮。
	t.Run("压缩不孤儿化结果且切在整轮边界", func(t *testing.T) {
		base := []llm.Message{
			{Role: llm.RoleUser, Content: "看桌面"},
			callAssistant("c1"),
			toolResult("c1", strings.Repeat("桌面上有这些文件 ", 40)),
			{Role: llm.RoleAssistant, Content: "桌面有这些文件"},
			{Role: llm.RoleUser, Content: "看 skill 文件夹"},
			callAssistant("c2"),
			toolResult("c2", strings.Repeat("skills 目录里有 ", 40)),
			{Role: llm.RoleAssistant, Content: "skills 里有这些"},
		}
		for keep := 0; keep <= 4000; keep += 37 {
			out, _ := Compact(base, Budget{Window: 1, Reserve: 0, Keep: keep})
			assertNoOrphanToolResults(t, out, "Compact")
			if len(out) > 0 && out[0].Role != llm.RoleUser {
				t.Fatalf("keep=%d：保留侧第一条是 %q，turn 被从中间切开了", keep, out[0].Role)
			}
		}

		small := []llm.Message{
			{Role: llm.RoleUser, Content: "一句"},
			{Role: llm.RoleAssistant, Content: "一句"},
		}
		if out, after := Compact(small, Budget{Window: 100000, Reserve: 1000, Keep: 500}); len(out) != 2 || after == 0 {
			t.Fatalf("没超预算就不该动: %+v", out)
		}
		tight := []llm.Message{
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
			{Role: llm.RoleAssistant, Content: "很长很长很长很长很长很长很长很长的回答"},
			{Role: llm.RoleUser, Content: "很长很长很长很长很长很长很长很长的问题"},
		}
		if out, _ := Compact(tight, Budget{Window: 10, Reserve: 1, Keep: 1}); len(out) != len(tight) {
			t.Fatalf("切点不足时应原样返回，实际裁成 %d 条", len(out))
		}
	})
}
