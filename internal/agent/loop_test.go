// 覆盖内核的硬约束：多轮回填顺序、并行批次保序、插话注入、中断收尾、
// 截断轮不执行工具、重复调用被拦、预算裁剪。
package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// scriptedStreamer 按脚本返回若干轮响应，用于在不联网的前提下驱动内核。
type scriptedStreamer struct {
	turns []llm.Message
	stops []string
	idx   int
	calls int32
}

func (s *scriptedStreamer) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	i := s.idx
	s.idx++
	atomic.AddInt32(&s.calls, 1)
	ch := make(chan llm.Event, 8)
	go func() {
		defer close(ch)
		if i >= len(s.turns) {
			ch <- llm.Event{Type: llm.EventError, StopReason: llm.StopError, Err: errors.New("脚本用完")}
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
		if i < len(s.stops) {
			stop = s.stops[i]
		}
		if len(m.ToolCalls) > 0 && stop == llm.StopStop {
			stop = llm.StopToolUse
		}
		ch <- llm.Event{Type: llm.EventDone, StopReason: stop, Usage: &llm.Usage{Input: 1, Output: 1, Total: 2}}
	}()
	return ch, nil
}

// echoTool 回显参数，用于断言结果按调用顺序回填。
type echoTool struct {
	name  string
	mode  tool.ExecutionMode
	seen  *[]string
	delay time.Duration
}

func (t echoTool) Name() string                  { return t.name }
func (t echoTool) Label() string                 { return t.name }
func (t echoTool) Description() string           { return "测试工具" }
func (t echoTool) PromptSnippet() string         { return "测试" }
func (t echoTool) PromptGuidelines() []string    { return nil }
func (t echoTool) Parameters() map[string]any    { return map[string]any{"type": "object", "properties": map[string]any{}} }
func (t echoTool) ExecutionMode() tool.ExecutionMode { return t.mode }
func (t echoTool) RequiresApproval() bool        { return false }
func (t echoTool) Execute(ctx context.Context, in tool.Input) (*tool.Result, error) {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	if t.seen != nil {
		*t.seen = append(*t.seen, t.name)
	}
	return &tool.Result{Content: t.name + "-ok", Title: t.name}, nil
}

func TestLoopMultiTurnThenStop(t *testing.T) {
	var seen []string
	streamer := &scriptedStreamer{
		turns: []llm.Message{
			{Content: "先看看", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}}},
			{Content: "再看", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "two"}}},
			{Content: "做完了"},
		},
		stops: []string{llm.StopToolUse, llm.StopToolUse, llm.StopStop},
	}
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
	// 工具结果必须紧跟其声明顺序，否则上游下一轮会 400。
	if msgs[1].ToolCallID != "c1" || msgs[3].ToolCallID != "c2" || msgs[4].Content != "做完了" {
		t.Fatalf("消息顺序不符合协议约束: %+v", msgs)
	}
}

// 并行批次必须仍按声明顺序回填：执行可以并发，写进上下文的顺序不能乱。
func TestLoopParallelBatchKeepsCallOrder(t *testing.T) {
	streamer := &scriptedStreamer{
		turns: []llm.Message{{
			ToolCalls: []llm.ToolCall{
				{ID: "a", Name: "slow"}, {ID: "b", Name: "fast"}, {ID: "c", Name: "mid"},
			},
		}, {Content: "好了"}},
		stops: []string{llm.StopToolUse, llm.StopStop},
	}
	tools := []tool.Tool{
		echoTool{name: "slow", mode: tool.ExecutionParallel, delay: 30 * time.Millisecond},
		echoTool{name: "fast", mode: tool.ExecutionParallel},
		echoTool{name: "mid", mode: tool.ExecutionParallel, delay: 15 * time.Millisecond},
	}
	loop := New(Config{Streamer: streamer, Tools: tools, Model: "test", Parallel: 4}, nil)
	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	msgs := loop.Messages()
	got := []string{}
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			got = append(got, m.ToolCallID)
		}
	}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("并行结果回填乱序: %v", got)
		}
	}
}

func TestLoopSteeringInjectedBeforeNextTurn(t *testing.T) {
	streamer := &scriptedStreamer{
		turns: []llm.Message{{Content: "第一步"}, {Content: "已按纠正做完"}},
		stops: []string{llm.StopToolUse, llm.StopStop},
	}
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
	// 插话必须排在第一轮 assistant 之前，否则等于让模型无视它。
	if idx > 0 && msgs[idx-1].Role != llm.RoleUser {
		t.Fatal("插话没有并入本轮上下文")
	}
}

func TestLoopStopsOnCancel(t *testing.T) {
	streamer := &scriptedStreamer{
		turns: []llm.Message{{Content: "开始"}},
		stops: []string{llm.StopStop},
	}
	loop := New(Config{Streamer: streamer, Model: "test"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := loop.Run(ctx)
	if err != nil {
		t.Fatalf("取消不应返回错误: %v", err)
	}
	if res.StopReason != StopAborted {
		t.Fatalf("期望 aborted，实际 %s", res.StopReason)
	}
}

func TestLoopNeverExecutesTruncatedToolCalls(t *testing.T) {
	var seen []string
	streamer := &scriptedStreamer{
		turns: []llm.Message{{Content: "被截断", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one"}}}},
		stops: []string{llm.StopLength},
	}
	loop := New(Config{Streamer: streamer, Tools: []tool.Tool{echoTool{name: "one", seen: &seen}}, Model: "test"}, nil)

	res, _ := loop.Run(context.Background())
	if len(seen) != 0 {
		t.Fatal("被截断的响应里工具不该被执行")
	}
	if res.StopReason != StopLength {
		t.Fatalf("期望 length，实际 %s", res.StopReason)
	}
}

// 模型卡在同一个调用上打转时，第三次就该拦下来并把失败结果喂回去。
func TestLoopBlocksRepeatedIdenticalCall(t *testing.T) {
	var seen []string
	streamer := &scriptedStreamer{
		turns: []llm.Message{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "one", Args: map[string]any{"path": "a.txt"}}}},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "one", Args: map[string]any{"path": "a.txt"}}}},
			{ToolCalls: []llm.ToolCall{{ID: "c3", Name: "one", Args: map[string]any{"path": "a.txt"}}}},
			{Content: "换了个办法"},
		},
		stops: []string{llm.StopToolUse, llm.StopToolUse, llm.StopToolUse, llm.StopStop},
	}
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

func TestLoopCompactsOverBudget(t *testing.T) {
	streamer := &scriptedStreamer{
		turns: []llm.Message{{Content: "最后一句"}},
		stops: []string{llm.StopStop},
	}
	history := make([]llm.Message, 0, 40)
	for i := 0; i < 20; i++ {
		history = append(history,
			llm.Message{Role: llm.RoleUser, Content: "历史问题"},
			llm.Message{Role: llm.RoleAssistant, Content: "历史回答，这里写很多字用来撑大 token 估算。"},
		)
	}
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
}
