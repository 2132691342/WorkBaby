// 内核不变量：多轮回填顺序、并行批次保序、轮间插话、取消与截断收尾、
// 重复调用拦截、超预算裁剪。每条断言对应一个「错了上游就会 400 / 会空转」的硬约束。
package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/llmtest"
	"WorkBaby/internal/tool"
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

// 整条主循环：工具轮 → 结果回填 → 再请求 → 纯文本收尾。
// 回填错位会让 assistant(tool_calls) 与 tool 配不上，上游直接 400。
func TestLoopRunsTurnsAndKeepsProtocolOrder(t *testing.T) {
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
}

// 并行批次可以并发执行，但写进上下文的顺序必须等于声明顺序。
func TestLoopParallelBatchFillsInDeclaredOrder(t *testing.T) {
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
}

// 插话必须并入下一轮上下文，且排在首条 assistant 之前——排在后面等于让模型无视它。
func TestLoopInjectsSteeringBeforeNextTurn(t *testing.T) {
	streamer := llmtest.New(
		llm.Message{Content: "第一步"},
		llm.Message{Content: "已按纠正做完"},
	)
	q := NewQueue()
	q.Enqueue(llm.Message{Role: llm.RoleUser, Content: "不对，换个做法"})
	loop := New(Config{Streamer: streamer, Model: "test", Steering: q}, nil)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	msgs := loop.Messages()
	idx := -1
	for i, m := range msgs {
		if m.Role == llm.RoleUser && m.Content == "不对，换个做法" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("插话没有进入上下文")
	}
	if idx > 0 && msgs[idx-1].Role != llm.RoleUser {
		t.Fatal("插话没有并入本轮上下文")
	}
	if q.HasItems() {
		t.Fatal("插话被取走后队列应为空")
	}
}

// 收尾判据：取消不产生错误、截断轮的工具一律不执行、上游报错也以 error 收尾。
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
	loop := New(Config{
		Streamer: streamer,
		Tools:    []tool.Tool{echoTool{name: "one", seen: &seen}},
		Model:    "test",
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
}

// 发送前必须过预算：超了就裁，且裁完必须从 user 消息起头。
func TestLoopCompactsHistoryOverBudget(t *testing.T) {
	history := make([]llm.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history,
			llm.Message{Role: llm.RoleUser, Content: "历史问题"},
			llm.Message{Role: llm.RoleAssistant, Content: "历史回答，这里写很多字用来撑大 token 估算。"},
		)
	}
	streamer := llmtest.New(llm.Message{Content: "最后一句"})
	loop := New(Config{
		Streamer: streamer,
		Model:    "test",
		Budget:   Budget{Window: 1000, Reserve: 200, Keep: 200},
	}, history)

	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if len(res.Messages) >= len(history) {
		t.Fatalf("超预算却没有裁剪: %d -> %d", len(history), len(res.Messages))
	}
	if res.Messages[0].Role != llm.RoleUser {
		t.Fatal("裁剪后必须从 user 消息开始")
	}
	if streamer.Calls() == 0 {
		t.Fatal("裁剪后仍应发起请求")
	}
}

// 事件出口必须把并发调用串行化：并行工具各自在 goroutine 里发事件，
// 上层靠「同一时刻只有一个事件在处理」维护落库位点。没有这把锁时，
// 两个 goroutine 会同时读写同一份状态，assistant 声明因此撞主键、整条丢失。
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
