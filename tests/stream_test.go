// 流式翻译器的块表状态机：增量累加、工具块生命周期、权威覆盖、收尾与用量都在这一个状态机上。
// 界面「此刻该显示什么」只有这一个答案，所以它的每条规则都要钉死。
package tests

import (
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

// frameCapture 收订阅到的帧：广播入队与出队是异步的，断言前要等泵送达。
type frameCapture struct {
	mu     sync.Mutex
	frames []sse.Frame
}

// sink 把一帧记下来。
func (c *frameCapture) sink(frame sse.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

// of 取某个事件的所有帧。
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

// waitFrames 等某个事件攒够 n 帧；超时即失败。
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
func newStream(t *testing.T) (*service.Stream, *frameCapture) {
	t.Helper()
	hub := sse.New(64)
	cap := &frameCapture{}
	cancel := hub.Subscribe(0, cap.sink)
	t.Cleanup(cancel)
	return service.NewStream(hub), cap
}

// TestStreamBlockTable 断言块表状态机的全部契约：增量按块累加、工具块从出现到收尾、
// 落盘消息权威覆盖但保留工具读数、用量透传、失败与收尾都收得住。
func TestStreamBlockTable(t *testing.T) {
	// ---- 增量：同类续写、异类另开一块 ----
	stream, _ := newStream(t)
	runId := stream.Begin("你好")
	if !strings.HasPrefix(runId, "RUN_") {
		t.Fatalf("run id 应带 RUN_ 前缀，实得 %s", runId)
	}
	if !stream.Active() {
		t.Fatal("Begin 之后应处于流式态")
	}
	stream.ThinkingDelta("想")
	stream.TextDelta("答")
	stream.TextDelta("案")
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

	// ---- 工具块：开始 → 结束（带输出与耗时）；落盘消息覆盖时保留工具读数 ----
	stream, cap := newStream(t)
	stream.Begin("看文件")
	stream.ToolStart("call-1", "read", `{"path":"a.go"}`)
	stream.ToolEnd("call-1", "read", agent.Result{Text: "文件内容"}, 12)
	blocks = stream.Blocks
	if len(blocks) != 1 || blocks[0].BlockKind != domain.BlockTool {
		t.Fatalf("应有一个工具块，实得 %+v", blocks)
	}
	if got := blocks[0]; got.ToolCallId != "call-1" || got.ToolName != "read" ||
		got.ToolStatus != "done" || got.ToolOutput != "文件内容" || got.DurationMs != 12 {
		t.Fatalf("工具块终态不对：%+v", got)
	}
	// 助手消息落盘：整表覆盖，但工具块沿用实时阶段已填好的输出与状态——
	// 落盘的消息里只有入参，覆盖会把执行读数抹掉。
	stream.AssistantMessage(agent.Message{
		Role: agent.RoleAssistant,
		Blocks: []agent.Block{
			{Kind: agent.KindText, Text: "读完了"},
			{Kind: agent.KindToolCall, CallId: "call-1", ToolName: "read", ToolArgs: `{"path":"a.go"}`},
		},
	})
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
	if snapshots := cap.waitFrames(t, "chat:turn", 1); len(snapshots) == 0 {
		t.Fatal("落盘时应广播 chat:turn 快照")
	} else if _, ok := snapshots[0].Data["turn_blocks"]; !ok {
		// 键名必须与前端 readBlocks 读的那个一致：换个名字等于快照静默丢掉。
		t.Fatalf("chat:turn 应带 turn_blocks，实得 %v", snapshots[0].Data)
	}

	// ---- 用量帧与收尾 ----
	stream, cap = newStream(t)
	stream.Begin("问一下")
	stream.Usage(agent.Usage{InputTokens: 11, OutputTokens: 7})
	stream.AssistantMessage(agent.Message{
		Role:   agent.RoleAssistant,
		Blocks: []agent.Block{{Kind: agent.KindText, Text: "答"}},
	})
	if len(cap.waitFrames(t, "chat:turn_end", 1)) == 0 {
		t.Fatal("落盘时应广播 chat:turn_end 用量帧")
	}
	stream.Finish(false)
	if stream.Active() {
		t.Fatal("Finish 之后不该还在流式态")
	}
	if len(cap.waitFrames(t, "chat:done", 1)) == 0 {
		t.Fatal("收尾应广播 chat:done")
	}

	// ---- 失败：消息要清掉 ANSI 码，且要解除流式态 ----
	stream, cap = newStream(t)
	stream.Begin("问")
	stream.Fail("\x1b[31m上游报错\x1b[0m")
	if stream.Active() {
		t.Fatal("失败后不该还在流式态")
	}
	errFrames := cap.waitFrames(t, "chat:error", 1)
	if len(errFrames) == 0 {
		t.Fatal("失败应广播 chat:error")
	}
	message, _ := errFrames[0].Data["message"].(string)
	if strings.Contains(message, "\x1b") {
		t.Fatalf("错误文案应清掉 ANSI 码，实得 %q", message)
	}
}
