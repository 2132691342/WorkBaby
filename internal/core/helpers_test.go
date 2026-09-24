// 内核测试共享替身：Provider / Tool / Sink / 步骤记忆 / 检查点的内存实现。

package core

import (
	"context"
	"encoding/json"
	"sync"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// mockProvider 按轮返回预设响应；记录调用次数与收到的请求。
type mockProvider struct {
	mu        sync.Mutex
	turns     []llm.ChatResponse
	calls     int
	requests  []*llm.ChatRequest
	streamErr error
	// chunkErr 非空时在流尾追加一个错误 chunk，模拟「生成中途上游报错」。
	chunkErr error
}

func (m *mockProvider) Name() string                                    { return "mock" }
func (m *mockProvider) Kind() llm.ProviderKind                          { return "openai" }
func (m *mockProvider) Models(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (m *mockProvider) Ping(context.Context) error                      { return nil }

// Chat 非流式：按调用序取一条响应。
func (m *mockProvider) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.calls
	m.calls++
	if i < len(m.turns) {
		r := m.turns[i]
		return &r, nil
	}
	return &llm.ChatResponse{}, nil
}

// Stream 按调用序取一条响应并拆成增量 chunk。
func (m *mockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	m.mu.Lock()
	m.requests = append(m.requests, req)
	i := m.calls
	m.calls++
	err := m.streamErr
	var resp llm.ChatResponse
	if i < len(m.turns) {
		resp = m.turns[i]
	}
	chunkErr := m.chunkErr
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make(chan llm.StreamChunk)
	send := func(c llm.StreamChunk) bool {
		select {
		case out <- c:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		defer close(out)
		if resp.Message.Content != "" {
			if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Content: resp.Message.Content}}) {
				return
			}
		}
		if resp.Message.Thinking != "" {
			if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Thinking: resp.Message.Thinking}}) {
				return
			}
		}
		for _, tc := range resp.ToolCalls {
			c := tc
			if !send(llm.StreamChunk{ToolCall: &c}) {
				return
			}
		}
		u := resp.Usage
		if !send(llm.StreamChunk{FinalUsage: &u}) {
			return
		}
		sr := resp.StopReason
		_ = send(llm.StreamChunk{FinishReason: &sr})
		if chunkErr != nil {
			_ = send(llm.StreamChunk{Err: chunkErr})
		}
	}()
	return out, nil
}

// callCount 已发起的模型请求次数。
func (m *mockProvider) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// toolNamesOf 最近一次请求暴露给模型的工具名。
func (m *mockProvider) toolNamesOf() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.requests[0].Tools))
	for _, t := range m.requests[0].Tools {
		names = append(names, t.Name)
	}
	return names
}

// mockTool 测试工具；fn 为空时返回固定成功结果。
type mockTool struct {
	mu    sync.Mutex
	name  string
	risk  tool.RiskLevel
	meta  tool.ToolMeta
	fn    func(args json.RawMessage) tool.ToolResult
	calls int
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool " + t.name }

// Schema 暴露给模型的描述；参数用最小合法 object。
func (t *mockTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.name,
		Description: "mock tool " + t.name,
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func (t *mockTool) RiskLevel() tool.RiskLevel { return t.risk }

// Meta 声明式元信息，供只读并行裁决与路径校验使用。
func (t *mockTool) Meta() tool.ToolMeta { return t.meta }

func (t *mockTool) Execute(_ context.Context, args json.RawMessage) tool.ToolResult {
	t.mu.Lock()
	t.calls++
	t.mu.Unlock()
	if t.fn != nil {
		return t.fn(args)
	}
	return tool.ToolResult{Content: "ok:" + t.name}
}

// callCount 执行次数。
func (t *mockTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// captureSink 收集事件的测试 Sink。
type captureSink struct {
	mu     sync.Mutex
	events []Event
}

func (s *captureSink) Emit(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

// kinds 按序返回事件类型。
func (s *captureSink) kinds() []EventKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]EventKind, 0, len(s.events))
	for _, e := range s.events {
		out = append(out, e.Kind)
	}
	return out
}

// count 统计某类事件出现次数。
func (s *captureSink) count(k EventKind) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// memStepStore 内存步骤记忆，测试幂等复用。
type memStepStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemStepStore() *memStepStore { return &memStepStore{data: map[string]string{}} }

func (s *memStepStore) Load(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *memStepStore) Store(key, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = content
}

// memCheckpoints 内存检查点存储。
type memCheckpoints struct {
	mu   sync.Mutex
	last *Checkpoint
}

func (s *memCheckpoints) Append(cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = cp
	return nil
}

func (s *memCheckpoints) LoadLast(string) (*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last, nil
}

func (s *memCheckpoints) Cleanup(string, int) error { return nil }

// newTestLoop 构造带 mock 工具与默认执行器的 Loop。
func newTestLoop(t interface{ Helper() }, p llm.Provider, tools ...*mockTool) *Loop {
	t.Helper()
	reg := tool.NewRegistry()
	for _, tl := range tools {
		if err := reg.Register(tl); err != nil {
			panic("register mock tool failed: " + err.Error())
		}
	}
	return New(p, reg, Config{Model: "mock-model"})
}
