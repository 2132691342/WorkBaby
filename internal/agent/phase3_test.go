// PI Phase 3 测试：2 层循环、PrepareNextTurn、Steering/FollowUp 队列、
// 工具 ExecutionMode。覆盖 loop_test.go 没碰到的 4 个新能力。

package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// TestTwoLayerLoop 验证外层 follow-up 队列触发 run 续跑。
func TestTwoLayerLoop(t *testing.T) {
	t.Run("follow-up 队列非空时外层继续", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			// 第 1 轮：模型产出正文，准备收尾
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "first"}, StopReason: "end_turn"},
			// 第 2 轮：follow-up 触发后的下一轮
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "second"}, StopReason: "end_turn"},
		}}
		fuQueue := &memMessageQueue{mode: QueueAll}
		fuQueue.Enqueue(llm.UserMessage("please continue"))
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithFollowUpQueue(fuQueue).
			WithSink(&captureSink{})

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, 2, p.callCount(), "外层循环触发第二轮请求")
		assert.Equal(t, ReasonEndTurn, out.Reason)
		assert.Contains(t, out.Content, "first")
		assert.Contains(t, out.Content, "second")
	})

	t.Run("follow-up 队列空则单次收尾", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "only"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithFollowUpQueue(&memMessageQueue{mode: QueueAll})

		out, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, 1, p.callCount(), "队列空时直接收尾")
		assert.Equal(t, ReasonEndTurn, out.Reason)
	})
}

// TestPrepareNextTurn 验证 PrepareNextTurn 回调：模型切换 / 思考档切换 / 注入消息 / 压缩。
func TestPrepareNextTurn(t *testing.T) {
	t.Run("切换模型与思考档生效", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "v1"}, StopReason: "end_turn"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "v2"}, StopReason: "end_turn"},
		}}
		// 触发外层循环：follow-up 队列让 run 跑两轮，PrepareNextTurn 在两轮之间生效
		fu := &memMessageQueue{mode: QueueAll}
		fu.Enqueue(llm.UserMessage("again"))
		var modelCalls []string
		wrapped := &modelCapturingProvider{p: p, captured: &modelCalls}
		l := newTestLoop(t, wrapped, echo).WithGuard(ExposeGuard(nil)).
			WithFollowUpQueue(fu).
			WithPrepareNextTurn(func(_ context.Context, _ LastTurnContext) (NextTurnUpdate, error) {
				return NextTurnUpdate{Model: "downgraded-model"}, nil
			})
		l.cfg.Model = "initial-model"

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err)
		assert.Contains(t, modelCalls, "downgraded-model", "PrepareNextTurn 切换的模型应透传到 provider")
	})

	t.Run("ExtraMessages 注入到历史", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "first"}, StopReason: "end_turn"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "second"}, StopReason: "end_turn"},
		}}
		fu := &memMessageQueue{mode: QueueAll}
		fu.Enqueue(llm.UserMessage("again"))
		injected := llm.UserMessage("injected between turns")
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithFollowUpQueue(fu).
			WithPrepareNextTurn(func(_ context.Context, _ LastTurnContext) (NextTurnUpdate, error) {
				return NextTurnUpdate{ExtraMessages: []*llm.Message{injected}}, nil
			})

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(p.requests), 2)
		var saw bool
		for _, m := range p.requests[1].Messages {
			if m.Content == "injected between turns" {
				saw = true
				break
			}
		}
		assert.True(t, saw, "ExtraMessages 应进入下一轮的请求")
	})

	t.Run("CompressInfo 触发 compressed 事件", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "v1"}, StopReason: "end_turn"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "v2"}, StopReason: "end_turn"},
		}}
		fu := &memMessageQueue{mode: QueueAll}
		fu.Enqueue(llm.UserMessage("again"))
		sink := &captureSink{}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithFollowUpQueue(fu).
			WithPrepareNextTurn(func(_ context.Context, _ LastTurnContext) (NextTurnUpdate, error) {
				return NextTurnUpdate{CompressInfo: &CompressInfo{Removed: 5, Truncated: true}}, nil
			}).
			WithSink(sink)

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, sink.count(EventCompressed), 1, "PrepareNextUpdate.CompressInfo 应触发 compressed 事件")
	})
}

// TestSteeringQueue 验证 steering 队列与 QueueMode。
func TestSteeringQueue(t *testing.T) {
	t.Run("one-at-a-time 模式每次取 1 条", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
			{ToolCalls: []llm.NormalizedToolCall{{ID: "2", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		steerQ := &memMessageQueue{mode: QueueOneAtATime}
		steerQ.Enqueue(llm.UserMessage("steer 1"))
		steerQ.Enqueue(llm.UserMessage("steer 2"))
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithSteeringQueue(steerQ)

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		// 第 1 轮 tool → drain 1 条；第 2 轮 tool → drain 第 2 条 → 第 3 轮收尾
		assert.Equal(t, 3, p.callCount(), "one-at-a-time 应触发 3 轮")
	})

	t.Run("all 模式一次取完所有消息", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		steerQ := &memMessageQueue{mode: QueueAll}
		steerQ.Enqueue(llm.UserMessage("steer 1"))
		steerQ.Enqueue(llm.UserMessage("steer 2"))
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithSteeringQueue(steerQ)

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, 2, p.callCount(), "all 模式一次 drain 后只追加一轮")
		// 第 2 轮应包含两条 steer 消息
		require.GreaterOrEqual(t, len(p.requests), 2)
		var seen int
		for _, m := range p.requests[1].Messages {
			if m.Content == "steer 1" || m.Content == "steer 2" {
				seen++
			}
		}
		assert.Equal(t, 2, seen, "all 模式两条 steer 都应进入下一轮请求")
	})

	t.Run("steering drain 触发 EventQueueDrained", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		steerQ := &memMessageQueue{mode: QueueOneAtATime}
		steerQ.Enqueue(llm.UserMessage("steer"))
		sink := &captureSink{}
		l := newTestLoop(t, p, echo).WithGuard(ExposeGuard(nil)).
			WithSteeringQueue(steerQ).
			WithSink(sink)

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.GreaterOrEqual(t, sink.count(EventQueueDrained), 1)
	})
}

// TestExecutionMode 验证工具级执行模式：声明 Sequential 整批串行，未声明默认 Parallel（不看只读启发式）。
func TestExecutionMode(t *testing.T) {
	t.Run("声明 Sequential 的工具整批串行", func(t *testing.T) {
		relA := make(chan struct{})
		a := &seqTool{name: "a", entered: make(chan struct{}, 1), delay: relA}
		b := &seqTool{name: "b", entered: make(chan struct{}, 1)}
		c := &seqTool{name: "c", entered: make(chan struct{}, 1)}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{
				{ID: "1", Name: "a", Arguments: json.RawMessage(`{}`)},
				{ID: "2", Name: "b", Arguments: json.RawMessage(`{}`)},
				{ID: "3", Name: "c", Arguments: json.RawMessage(`{}`)},
			}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		reg := tool.NewRegistry()
		require.NoError(t, reg.Register(a))
		require.NoError(t, reg.Register(b))
		require.NoError(t, reg.Register(c))
		l := New(p, reg, Config{Model: "mock-model"}).WithGuard(ExposeGuard(nil))

		done := make(chan error, 1)
		go func() { _, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")}); done <- err }()

		<-a.entered
		select {
		case <-b.entered:
			close(relA)
			<-done
			t.Fatal("声明 Sequential 的批次出现了并发执行")
		case <-time.After(300 * time.Millisecond):
			// A 阻塞期间 B 未启动 → 严格串行
		}
		close(relA)
		require.NoError(t, <-done)
		assert.Equal(t, 1, a.calls)
		assert.Equal(t, 1, b.calls)
		assert.Equal(t, 1, c.calls)
	})

	t.Run("未声明 ExecutionMode 默认 Parallel", func(t *testing.T) {
		readA := &mockTool{name: "read_a", risk: tool.RiskReadOnly, meta: tool.ToolMeta{ReadOnly: true}}
		readB := &mockTool{name: "read_b", risk: tool.RiskReadOnly, meta: tool.ToolMeta{ReadOnly: true}}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{
				{ID: "1", Name: "read_a", Arguments: json.RawMessage(`{}`)},
				{ID: "2", Name: "read_b", Arguments: json.RawMessage(`{}`)},
			}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, readA, readB).WithGuard(ExposeGuard(nil))

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("read")})
		require.NoError(t, err)
	})

	t.Run("非只读未声明同样默认 Parallel", func(t *testing.T) {
		aEntered := make(chan struct{}, 1)
		bEntered := make(chan struct{}, 1)
		serialFallback := true // A 等到超时仍未见 B 进入，说明走了串行
		a := &mockTool{name: "exec_a", risk: tool.RiskExec, fn: func(json.RawMessage) tool.ToolResult {
			aEntered <- struct{}{}
			select {
			case <-bEntered:
				serialFallback = false
			case <-time.After(2 * time.Second):
			}
			return tool.ToolResult{Content: "a"}
		}}
		b := &mockTool{name: "exec_b", risk: tool.RiskExec, fn: func(json.RawMessage) tool.ToolResult {
			bEntered <- struct{}{}
			return tool.ToolResult{Content: "b"}
		}}
		p := &mockProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{
				{ID: "1", Name: "exec_a", Arguments: json.RawMessage(`{}`)},
				{ID: "2", Name: "exec_b", Arguments: json.RawMessage(`{}`)},
			}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}
		l := newTestLoop(t, p, a, b).WithGuard(ExposeGuard(nil))

		_, err := l.Run(context.Background(), []*llm.Message{llm.UserMessage("go")})
		require.NoError(t, err)
		assert.False(t, serialFallback, "非只读未声明工具应默认 Parallel：B 应在 A 阻塞期间启动")
	})
}

// === 测试辅助类型 ===

// memMessageQueue 内存版 MessageQueue，按 mode 行为。
type memMessageQueue struct {
	mu   sync.Mutex
	mode QueueMode
	buf  []*llm.Message
}

func (q *memMessageQueue) Drain() []*llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.buf) == 0 {
		return nil
	}
	switch q.mode {
	case QueueAll:
		out := q.buf
		q.buf = nil
		return out
	default:
		head := q.buf[0]
		q.buf = q.buf[1:]
		return []*llm.Message{head}
	}
}

func (q *memMessageQueue) Enqueue(m *llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.buf = append(q.buf, m)
}

func (q *memMessageQueue) HasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.buf) > 0
}

func (q *memMessageQueue) Mode() QueueMode { return q.mode }

// modelCapturingProvider 包装 mockProvider 记录每次请求的模型名。
type modelCapturingProvider struct {
	p        *mockProvider
	captured *[]string
}

func (m *modelCapturingProvider) Name() string { return m.p.Name() }
func (m *modelCapturingProvider) Kind() llm.ProviderKind { return m.p.Kind() }
func (m *modelCapturingProvider) Models(ctx context.Context) ([]llm.ModelInfo, error) {
	return m.p.Models(ctx)
}
func (m *modelCapturingProvider) Ping(ctx context.Context) error { return m.p.Ping(ctx) }

func (m *modelCapturingProvider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	*m.captured = append(*m.captured, req.Model)
	return m.p.Chat(ctx, req)
}

func (m *modelCapturingProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	*m.captured = append(*m.captured, req.Model)
	return m.p.Stream(ctx, req)
}

// seqTool 声明 ExecutionMode = Sequential 的测试工具。
type seqTool struct {
	name    string
	entered chan struct{} // 非空时 Execute 进入即非阻塞投递，供测试观测执行起点
	calls   int
	delay   chan struct{}
}

func (t *seqTool) Name() string { return t.name }
func (t *seqTool) Description() string { return "seq " + t.name }
func (t *seqTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.name,
		Description: t.Description(),
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}
func (t *seqTool) RiskLevel() tool.RiskLevel { return tool.RiskExec }
func (t *seqTool) ToolExecutionMode() tool.ExecutionMode { return tool.ExecutionSequential }

func (t *seqTool) Execute(_ context.Context, _ json.RawMessage) tool.ToolResult {
	t.calls++
	if t.entered != nil {
		select {
		case t.entered <- struct{}{}:
		default:
		}
	}
	if t.delay != nil {
		<-t.delay
	}
	return tool.ToolResult{Content: "ok:" + t.name}
}

// 确保 seqTool 满足 tool.Tool 接口
var _ tool.Tool = (*seqTool)(nil)