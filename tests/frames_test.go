// 流式翻译器的块表状态机：增量累加、工具块生命周期、message_end 权威覆盖、收尾与用量都在这一个状态机上。
package tests

import (
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/service/chat"
	"WorkBaby/backend/sse"
)

// frameCapture 收订阅到的帧：广播入队与出队是异步的，断言前要等泵送达。
type frameCapture struct {
	mu     sync.Mutex
	frames []sse.Frame
}

func (c *frameCapture) sink(frame sse.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

func (c *frameCapture) of(event string) []sse.Frame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sse.Frame, 0, len(c.frames))
	for _, f := range c.frames {
		if f.Event == event {
			out = append(out, f)
		}
	}
	return out
}

func (c *frameCapture) waitFrames(t *testing.T, event string, n int) []sse.Frame {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.of(event); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等事件 %s ×%d 超时，实收 %d 帧", event, n, len(c.of(event)))
	return nil
}

// newStream 造一对（翻译器 + 帧捕获）：每段场景各自独立，互不串帧。
func newStream(t *testing.T) (*chat.Stream, *frameCapture) {
	t.Helper()
	hub := sse.New(64)
	cap := &frameCapture{}
	cancel := hub.Subscribe(0, cap.sink)
	t.Cleanup(cancel)
	return chat.NewStream(hub), cap
}

// deltaEvent 造一帧 assistantMessageEvent 增量。
func deltaEvent(kind, delta string, index any) chat.Record {
	return chat.Record{"type": "message_update", "assistantMessageEvent": map[string]any{
		"type": kind, "delta": delta, "contentIndex": index,
	}}
}

// TestFrames 断言块表状态机的全部契约：增量按块累加、工具块从出现到收尾、
// message_end 权威覆盖但保留工具读数、用量透传、出错收尾、未知事件静默。
func TestFrames(t *testing.T) {
	// ---- 增量：同下标累加、异下标分块 ----
	stream, _ := newStream(t)
	runId := stream.Begin("你好")
	if !strings.HasPrefix(runId, "RUN_") {
		t.Fatalf("run id 应带 RUN_ 前缀，实得 %s", runId)
	}
	if !stream.Active() {
		t.Fatal("Begin 之后应处于流式态")
	}
	stream.OnEvent(deltaEvent("thinking_delta", "想", 0))
	stream.OnEvent(deltaEvent("text_delta", "答", 1))
	stream.OnEvent(deltaEvent("text_delta", "案", 1))
	blocks := stream.Blocks
	if len(blocks) != 2 {
		t.Fatalf("应有两个块，实得 %d", len(blocks))
	}
	if blocks[0].BlockKind != domain.BlockThinking || blocks[0].BlockText != "想" {
		t.Fatalf("0 号块应为思考块「想」，实得 %+v", blocks[0])
	}
	if blocks[1].BlockKind != domain.BlockText || blocks[1].BlockText != "答案" {
		t.Fatalf("1 号块应为正文块「答案」，实得 %+v", blocks[1])
	}

	// ---- 工具块：出现 → 带参 → 执行 → 收尾；message_end 权威覆盖但按 id 保留读数 ----
	stream, cap := newStream(t)
	stream.Begin("看文件")
	stream.OnEvent(chat.Record{"type": "message_update", "assistantMessageEvent": map[string]any{
		"type": "toolcall_start", "id": "call-1", "toolName": "read", "contentIndex": 0,
	}})
	stream.OnEvent(chat.Record{"type": "message_update", "assistantMessageEvent": map[string]any{
		"type": "toolcall_end", "contentIndex": 0,
		"toolCall": map[string]any{"id": "call-1", "name": "read", "arguments": map[string]any{"path": "a.go"}},
	}})
	stream.OnEvent(chat.Record{"type": "tool_execution_start", "toolCallId": "call-1", "toolName": "read"})
	stream.OnEvent(chat.Record{"type": "tool_execution_end", "toolCallId": "call-1", "isError": false, "durationMs": 12,
		"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "文件内容"}}}})
	blocks = stream.Blocks
	if len(blocks) != 1 || blocks[0].BlockKind != domain.BlockTool {
		t.Fatalf("应有一个工具块，实得 %+v", blocks)
	}
	if got := blocks[0]; got.ToolCallId != "call-1" || got.ToolName != "read" ||
		got.ToolStatus != "done" || got.ToolOutput != "文件内容" || got.DurationMs != 12 {
		t.Fatalf("工具块终态不对：%+v", got)
	}
	stream.OnEvent(chat.Record{"type": "message_end", "message": map[string]any{
		"role": "assistant", "stopReason": "end",
		"content": []any{
			map[string]any{"type": "text", "text": "读完了"},
			map[string]any{"type": "toolCall", "id": "call-1", "name": "read", "arguments": map[string]any{"path": "a.go"}},
		},
	}})
	blocks = stream.Blocks
	if len(blocks) != 2 {
		t.Fatalf("快照后应有两个块，实得 %d", len(blocks))
	}
	if blocks[0].BlockKind != domain.BlockText || blocks[0].BlockText != "读完了" {
		t.Fatalf("快照 0 号块应为正文「读完了」，实得 %+v", blocks[0])
	}
	if blocks[1].ToolCallId != "call-1" || blocks[1].ToolStatus != "done" || blocks[1].ToolOutput != "文件内容" {
		t.Fatalf("快照应保留工具块的执行读数，实得 %+v", blocks[1])
	}
	if snapshots := cap.waitFrames(t, "chat:blocks", 1); len(snapshots) == 0 {
		t.Fatal("message_end 应广播 chat:blocks 快照")
	} else if _, ok := snapshots[0].Data["turn_blocks"]; !ok {
		// 键名必须与前端 readBlocks 读的那个一致：换个名字等于快照静默丢掉。
		t.Fatalf("chat:blocks 应带 turn_blocks，实得 %v", snapshots[0].Data)
	}

	// ---- 用量、user 话术、未知事件 ----
	stream, cap = newStream(t)
	stream.Begin("问一下")
	stream.OnEvent(chat.Record{"type": "message_end", "message": map[string]any{
		"role": "user", "content": "问一下",
	}})
	if stream.UserMessage != "问一下" {
		t.Fatalf("user message_end 应更新话术，实得 %q", stream.UserMessage)
	}
	// 未知事件静默忽略：不炸、不落块、不发帧。
	framesBefore := len(cap.of("chat:live"))
	stream.OnEvent(chat.Record{"type": "future_event", "foo": "bar"})
	if len(stream.Blocks) != 0 {
		t.Fatalf("未知事件不应动块表，实得 %+v", stream.Blocks)
	}
	if got := len(cap.of("chat:live")); got != framesBefore {
		t.Fatalf("未知事件不应发帧：前 %d，后 %d", framesBefore, got)
	}
	stream.OnEvent(chat.Record{
		"type":                  "message_update",
		"usage":                 map[string]any{"input": 10, "output": 7, "cacheRead": 3},
		"assistantMessageEvent": map[string]any{"type": "text_delta", "delta": "字", "contentIndex": 0},
	})
	if live := cap.waitFrames(t, "chat:live", 1); live[0].Data["output_tokens"] != 7 {
		t.Fatalf("chat:live 应带 output_tokens=7，实得 %v", live[0].Data["output_tokens"])
	}
	stream.OnEvent(chat.Record{"type": "turn_end", "message": map[string]any{
		"usage": map[string]any{"input": 10, "output": 7},
	}})
	turnEnds := cap.waitFrames(t, "chat:turn_end", 1)
	usage, ok := turnEnds[0].Data["turn_usage"].(domain.TurnUsageVO)
	if !ok {
		t.Fatalf("turn_end 应带 turn_usage，实得 %T", turnEnds[0].Data["turn_usage"])
	}
	if usage.OutputTokens != 7 || usage.InputTokens != 10 {
		t.Fatalf("用量透传不对：%+v", usage)
	}

	// ---- 收尾：出错带原话退出流式态；settled 透传 abort 并发 done ----
	stream.OnEvent(chat.Record{"type": "message_end", "message": map[string]any{
		"role": "assistant", "stopReason": "error", "errorMessage": "额度不足", "content": []any{},
	}})
	if stream.Active() {
		t.Fatal("stopReason=error 后应退出流式态")
	}
	if errFrames := cap.waitFrames(t, "chat:error", 1); errFrames[0].Data["message"] != "额度不足" {
		t.Fatalf("chat:error 应带引擎原话，实得 %v", errFrames[0].Data["message"])
	}
	stream.OnEvent(chat.Record{"type": "agent_settled", "aborted": true})
	if settled := cap.waitFrames(t, "chat:settled", 1); settled[0].Data["aborted"] != true {
		t.Fatalf("settled 应透传 aborted，实得 %v", settled[0].Data["aborted"])
	}
	cap.waitFrames(t, "chat:done", 1)
}


