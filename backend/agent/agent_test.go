// 内核链路：主循环的不变量（回填顺序、收尾判据、重复调用拦截、事件串行化）
// 与上下文裁剪 / 协议清洗（切点必须落在完整 turn 边界，输出能直发上游）。
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
func (t echoTool) Parameters() map[string]any { return map[string]any{"type": "object", "properties": map[string]any{}} }
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

// assertNoOrphanToolResults 断言每条工具结果都落在「最近一条 assistant 的声明窗口」里。
// 上游只认紧邻配对：结果与声明之间隔着另一条 assistant 或 user 消息都会被 400。
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

// 回填顺序：多轮逐次声明与同轮并行两种形态，写进上下文的顺序都必须等于声明顺序。
// 顺序本身就是协议约束——错位会让 assistant(tool_calls) 与 tool 配不上，上游直接 400。
func TestLoopProtocolOrder(t *testing.T) {
	t.Run("多轮按声明顺序回填", func(t *testing.T) {
		var seen []string
		streamer := llmtest.New(
			llm.Message{Content: "先看看", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}}},
			llm.Message{Content: "再看", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "two"}}},
			llm.Message{Content: "做完了"},
		)
		loop := New(Config{
			Streamer: streamer,
			Tools:    []tool.Tool{echoTool{name: "one", seen: &seen}, echoTool{name: "two", seen: &seen}},
			Model:    "test",
		}, nil)

		res, err := loop.Run(context.Background())
		if err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		if res.Turns != 3 {
			t.Fatalf("期望 3 轮，实际 %d", res.Turns)
		}
		msgs := loop.Messages()
		if len(msgs) != 5 {
			t.Fatalf("期望 5 条消息（2 assistant + 2 tool + 1 assistant），实际 %d", len(msgs))
		}
		if msgs[1].ToolCallID != "c1" || msgs[3].ToolCallID != "c2" || msgs[4].Content != "做完了" {
			t.Fatalf("消息顺序不符合协议约束: %+v", msgs)
		}
		if len(seen) != 2 {
			t.Fatalf("两个工具都应执行过，实际 %d", len(seen))
		}
	})

	t.Run("并行批次保序回填", func(t *testing.T) {
		streamer := llmtest.New(
			llm.Message{ToolCalls: []llm.ToolCall{
				{ID: "a", Name: "slow"}, {ID: "b", Name: "fast"}, {ID: "c", Name: "mid"},
			}},
			llm.Message{Content: "好了"},
		)
		tools := []tool.Tool{
			echoTool{name: "slow", mode: tool.ExecutionParallel, delay: 30 * time.Millisecond},
			echoTool{name: "fast", mode: tool.ExecutionParallel},
			echoTool{name: "mid", mode: tool.ExecutionParallel, delay: 15 * time.Millisecond},
		}
		loop := New(Config{Streamer: streamer, Tools: tools, Model: "test", Parallel: 4}, nil)
		if _, err := loop.Run(context.Background()); err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		got := toolIDs(loop.Messages())
		for i, want := range []string{"a", "b", "c"} {
			if i >= len(got) || got[i] != want {
				t.Fatalf("并行结果回填乱序: %v", got)
			}
		}
	})
}

// 收尾判据：取消不报错误、截断轮的工具一律不执行、上游报错也以 error 收尾。
func TestLoopStopsOnCancelLengthAndError(t *testing.T) {
	t.Run("取消", func(t *testing.T) {
		loop := New(Config{Streamer: llmtest.New(llm.Message{Content: "开始"}), Model: "test"}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res, err := loop.Run(ctx)
		if err != nil {
			t.Fatalf("取消不应返回错误: %v", err)
		}
		if res.StopReason != StopAborted {
			t.Fatalf("期望 aborted，实际 %s", res.StopReason)
		}
	})

	// 半个 JSON 调用执行出去比不执行更危险。
	t.Run("截断轮不执行工具", func(t *testing.T) {
		var seen []string
		streamer := llmtest.New(llm.Message{
			Content:   "被截断",
			ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}},
		})
		streamer.Stops = []string{llm.StopLength}
		loop := New(Config{
			Streamer: streamer,
			Tools:    []tool.Tool{echoTool{name: "one", seen: &seen}},
			Model:    "test",
		}, nil)
		res, _ := loop.Run(context.Background())
		if len(seen) != 0 {
			t.Fatal("被截断的响应里工具不该被执行")
		}
		if res.StopReason != StopLength {
			t.Fatalf("期望 length，实际 %s", res.StopReason)
		}
	})

	// 「只想不说」：预算全花在推理上。内核不自作主张重试——预算由 service 层按模型上限给，
	// 悄悄重试只会在上游上限更低时换来一个 400。
	t.Run("零产出截断如实收尾", func(t *testing.T) {
		streamer := llmtest.New(llm.Message{Thinking: "先想想该怎么做。"})
		streamer.Stops = []string{llm.StopLength}
		loop := New(Config{Streamer: streamer, Model: "test", MaxTokens: 32768}, nil)

		res, err := loop.Run(context.Background())
		if err != nil {
			t.Fatalf("运行失败: %v", err)
		}
		if res.StopReason != StopLength {
			t.Fatalf("期望 length，实际 %s", res.StopReason)
		}
		if streamer.Calls() != 1 {
			t.Fatalf("不该重试同一轮，实际发起 %d 次请求", streamer.Calls())
		}
		// 两次请求的下发预算必须一致：内核不该在重试路径上偷偷抬高它。
		if got := streamer.Requests[0].MaxTokens; got != 32768 {
			t.Fatalf("下发预算应为装配时的值 32768，实际 %d", got)
		}
	})

	// 「报错但零产出」不算成功，否则会落一条空 assistant，下一轮直接 400。
	t.Run("上游报错", func(t *testing.T) {
		streamer := llmtest.New(llm.Message{Content: "半句"})
		streamer.Err = errors.New("上游 500")
		loop := New(Config{Streamer: streamer, Model: "test"}, nil)
		res, err := loop.Run(context.Background())
		if err == nil {
			t.Fatal("上游报错应向上返回")
		}
		if res == nil || res.StopReason != StopError {
			t.Fatalf("期望 error 收尾，实际 %+v", res)
		}
	})
}

// 模型卡在同一个调用上打转时按重复上限拦下，并把失败结果喂回去让它换方案。
func TestLoopBlocksRepeatedIdenticalCall(t *testing.T) {
	var seen []string
	call := func(id string) llm.ToolCall {
		return llm.ToolCall{ID: id, Name: "one", Args: map[string]any{"path": "a.txt"}}
	}
	streamer := llmtest.New(
		llm.Message{ToolCalls: []llm.ToolCall{call("c1")}},
		llm.Message{ToolCalls: []llm.ToolCall{call("c2")}},
		llm.Message{ToolCalls: []llm.ToolCall{call("c3")}},
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
	if len(seen) != repeatCallLimit-1 {
		t.Fatalf("期望只执行 %d 次，实际 %d", repeatCallLimit-1, len(seen))
	}
	for _, m := range loop.Messages() {
		if m.Role == llm.RoleTool && m.ToolCallID == "c3" && !m.IsError {
			t.Fatal("第三次重复调用应当判失败")
		}
	}
	// 被拦的调用也必须有完整的开始 / 结束事件对：落库侧只在 tool_start 里写工具声明，
	// 缺了开始事件，重载历史后协议配对就断了。
	startIdx, endIdx := -1, -1
	for i, e := range events {
		if e.ToolCall == nil || e.ToolCall.ID != "c3" {
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
		t.Fatalf("被拦调用 c3 的事件对不完整: start=%d end=%d", startIdx, endIdx)
	}
}

// 并行工具各自在 goroutine 里发事件，上层靠「同一时刻只有一个事件在处理」维护落库位点。
func TestEmitSerializesConcurrentTools(t *testing.T) {
	var inFlight, peak int32
	streamer := llmtest.New(
		llm.Message{ToolCalls: []llm.ToolCall{
			{ID: "a", Name: "par"}, {ID: "b", Name: "par"},
			{ID: "c", Name: "par"}, {ID: "d", Name: "par"},
		}},
		llm.Message{Content: "好了"},
	)
	loop := New(Config{
		Streamer: streamer,
		Tools:    []tool.Tool{echoTool{name: "par", mode: tool.ExecutionParallel}},
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
			// 撑开窗口：没有锁时四个 goroutine 必然同时在飞
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
		},
	}, nil)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if peak > 1 {
		t.Fatalf("事件出口同时进入了 %d 个事件，上层落库位点会被读脏", peak)
	}
}

// 裁剪与清洗的硬约束：输出必须能直发上游，切点必须落在完整 turn 边界。
func TestCompactProtocol(t *testing.T) {
	t.Run("清洗剔除空 assistant 与孤儿结果", func(t *testing.T) {
		in := []llm.Message{
			{Role: llm.RoleUser, Content: "在吗"},
			{Role: llm.RoleAssistant},
			{Role: llm.RoleTool, ToolCallID: "gone", Content: "孤儿结果"},
		}
		out := CleanForProtocol(in)
		if len(out) != 1 || out[0].Content != "在吗" {
			t.Fatalf("空 assistant 与孤儿结果应被剔除: %+v", out)
		}
	})

	// 并行工具的声明若被逐条落库，链上会留下 A(c1) → A(c2) → T(c1) → T(c2)：
	// T(c1) 与它的声明之间隔着 A(c2)，上游以 tool result's tool id not found 拒绝。
	t.Run("拆散的并行声明合并为一条", func(t *testing.T) {
		in := []llm.Message{
			{Role: llm.RoleUser, Content: "两件事一起做"},
			callAssistant("c1"),
			callAssistant("c2"),
			toolResult("c1", "结果一"),
			toolResult("c2", "结果二"),
		}
		out := CleanForProtocol(in)
		if len(out) != 4 {
			t.Fatalf("相邻 assistant 应合并成一条，实际 %d 条", len(out))
		}
		if out[1].Role != llm.RoleAssistant || len(out[1].ToolCalls) != 2 {
			t.Fatalf("合并后的声明应带全部调用: %+v", out[1])
		}
		assertNoOrphanToolResults(t, out, "CleanForProtocol")
	})

	// keepTokens 从 0 扫到大，让切点落在每一个可能的位置上，结果都必须协议合法。
	t.Run("压缩永不孤儿化工具结果", func(t *testing.T) {
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
		}
	})

	// 保留侧的第一条必须是 user：切在回合中间会把半截 turn 发给上游。
	t.Run("压缩整轮丢弃", func(t *testing.T) {
		base := []llm.Message{
			{Role: llm.RoleUser, Content: "看桌面"},
			callAssistant("c1"),
			toolResult("c1", strings.Repeat("x", 400)),
			{Role: llm.RoleAssistant, Content: "好的"},
			{Role: llm.RoleUser, Content: "再看看"},
			callAssistant("c2"),
			toolResult("c2", strings.Repeat("y", 400)),
			{Role: llm.RoleAssistant, Content: "搞定"},
		}
		for keep := 0; keep <= 2000; keep += 53 {
			out, _ := Compact(base, Budget{Window: 1, Reserve: 0, Keep: keep})
			if len(out) == 0 {
				continue
			}
			if out[0].Role != llm.RoleUser {
				t.Fatalf("keep=%d：保留侧第一条是 %q，turn 被从中间切开了", keep, out[0].Role)
			}
		}
	})

	// 预算判定与降级：没超不动、切不出完整 turn 就放弃、降级只留最近整轮。
	t.Run("预算边界与降级", func(t *testing.T) {
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
		if out := TruncateDeterministic(tight, 1); len(out) != 1 || out[0].Content != tight[2].Content {
			t.Fatalf("降级截断应只保留最近的完整 turn: %+v", out)
		}
	})
}
