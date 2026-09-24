package service

// Agent 装配链路测试：内核接线（护栏与事件映射接通）+ 自定义档案生命周期对注册表的同步。

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
)

// ===== 测试替身 =====

// stubProvider 按调用序返回预设响应的测试 Provider。
type stubProvider struct {
	mu    sync.Mutex
	turns []llm.ChatResponse
	calls int
}

func (p *stubProvider) Name() string                                    { return "stub" }
func (p *stubProvider) Kind() llm.ProviderKind                          { return "openai" }
func (p *stubProvider) Models(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (p *stubProvider) Ping(context.Context) error                      { return nil }

func (p *stubProvider) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.calls
	p.calls++
	if i < len(p.turns) {
		r := p.turns[i]
		return &r, nil
	}
	return &llm.ChatResponse{}, nil
}

func (p *stubProvider) Stream(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	p.mu.Lock()
	i := p.calls
	p.calls++
	var resp llm.ChatResponse
	if i < len(p.turns) {
		resp = p.turns[i]
	}
	p.mu.Unlock()

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
		for _, tc := range resp.ToolCalls {
			c := tc
			if !send(llm.StreamChunk{ToolCall: &c}) {
				return
			}
		}
		sr := resp.StopReason
		_ = send(llm.StreamChunk{FinishReason: &sr})
	}()
	return out, nil
}

// stubTool 测试工具；err 非空时 Execute 恒定失败（模拟"工具持续失败"场景）。
type stubTool struct {
	mu    sync.Mutex
	name  string
	risk  tool.RiskLevel
	calls int
	err   string
}

func (t *stubTool) Name() string        { return t.name }
func (t *stubTool) Description() string { return "stub " + t.name }

func (t *stubTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: t.name, Description: "stub", Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (t *stubTool) RiskLevel() tool.RiskLevel { return t.risk }

func (t *stubTool) Execute(context.Context, json.RawMessage) tool.ToolResult {
	t.mu.Lock()
	t.calls++
	t.mu.Unlock()
	if t.err != "" {
		return tool.ToolResult{Err: pkg.New(4006, t.err, "")}
	}
	return tool.ToolResult{Content: "ok:" + t.name}
}

func (t *stubTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// ===== 内核装配接线 =====

// TestCoreLoopAssembly 装配层接线：真实护栏链 + 事件映射在装配后是否接通。
func TestCoreLoopAssembly(t *testing.T) {
	t.Run("跑通一次带工具的 ReAct 并映射事件", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		echo := &stubTool{name: "echo", risk: tool.RiskReadOnly}
		require.NoError(t, svc.tools.Registry().Register(echo))

		prov := &stubProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}

		var mu sync.Mutex
		var names []string
		svc.bus.Subscribe(event.MatchPrefix("chat:"), func(name string, _ any) {
			mu.Lock()
			defer mu.Unlock()
			names = append(names, name)
		})

		mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		loop := svc.newCoreLoop(coreLoopSpec{
			Ses:            &domain.ChatSessionDO{ID: ses.ID},
			RunID:          "RUN_1",
			AssistantMsgID: "MSG_1",
			Def:            core.Agent(core.AgentDefault),
			Provider:       prov,
			Model:          "stub-model",
			ToolNames:      []string{"echo"},
			Sink:           core.FuncSink(mapper.handle),
		})

		out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, core.ReasonEndTurn, out.Reason)
		assert.Equal(t, 1, echo.callCount())
		assert.Contains(t, names, "chat:tool")
		assert.Contains(t, names, "chat:tool-result")
		assert.Contains(t, names, "chat:stream")
	})

	t.Run("未暴露工具被护栏拒绝且不执行", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		echo := &stubTool{name: "echo", risk: tool.RiskReadOnly}
		require.NoError(t, svc.tools.Registry().Register(echo))

		prov := &stubProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "adapted"}, StopReason: "end_turn"},
		}}

		var mu sync.Mutex
		var refused []bool
		svc.bus.Subscribe(event.MatchPrefix("chat:tool-result"), func(_ string, payload any) {
			mu.Lock()
			defer mu.Unlock()
			if m, ok := payload.(map[string]any); ok {
				if v, ok := m["refused"].(bool); ok {
					refused = append(refused, v)
				}
			}
		})

		mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		loop := svc.newCoreLoop(coreLoopSpec{
			Ses:            &domain.ChatSessionDO{ID: ses.ID},
			RunID:          "RUN_1",
			AssistantMsgID: "MSG_1",
			Def:            core.Agent(core.AgentDefault),
			Provider:       prov,
			Model:          "stub-model",
			ToolNames:      []string{"other"}, // echo 未暴露
			Sink:           core.FuncSink(mapper.handle),
		})

		out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, 0, echo.callCount(), "未暴露的工具绝不能被执行")
		assert.Equal(t, core.ReasonEndTurn, out.Reason, "拒绝不是故障：模型可改道后正常收尾")
		require.Len(t, refused, 1)
		assert.True(t, refused[0], "拒绝回执必须带 refused 标记，前端展示「已拒绝」而非错误")
	})
}

// ===== 失败改道与错误摘要 =====

// TestAdaptiveGuardInServiceChain 同工具连续失败达阈值时内核必须注入改道提示：
// 模型要看见「已失败 N 次 + 候选替代路径」，否则会死磕同一调用直到耗尽预算。
func TestAdaptiveGuardInServiceChain(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, _ := seedSession(t, svc, msgRepo, ctx)

	failing := &stubTool{name: "file_read", risk: tool.RiskReadOnly, err: "read failed"}
	require.NoError(t, svc.tools.Registry().Register(failing))

	// 4 轮各传不同参数（绕开 RepeatGuard 的同参熔断），第 5 轮收尾
	call := func(id, path string) llm.NormalizedToolCall {
		return llm.NormalizedToolCall{ID: id, Name: "file_read", Arguments: json.RawMessage(`{"path":"` + path + `"}`)}
	}
	prov := &stubProvider{turns: []llm.ChatResponse{
		{ToolCalls: []llm.NormalizedToolCall{call("t1", "a")}, StopReason: "tool_use"},
		{ToolCalls: []llm.NormalizedToolCall{call("t2", "b")}, StopReason: "tool_use"},
		{ToolCalls: []llm.NormalizedToolCall{call("t3", "c")}, StopReason: "tool_use"},
		{ToolCalls: []llm.NormalizedToolCall{call("t4", "d")}, StopReason: "tool_use"},
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "改道"}, StopReason: "end_turn"},
	}}

	mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
	loop := svc.newCoreLoop(coreLoopSpec{
		Ses: &domain.ChatSessionDO{ID: ses.ID}, RunID: "RUN_1", AssistantMsgID: "MSG_1",
		Def: core.Agent(core.AgentDefault), Provider: prov, Model: "stub-model",
		ToolNames: []string{"file_read"}, Sink: core.FuncSink(mapper.handle),
	})

	out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("read")})
	require.NoError(t, err)
	assert.Equal(t, core.ReasonEndTurn, out.Reason, "失败不是故障：模型改道后应正常收尾")

	var hinted *llm.Message
	var toolCount int
	for _, m := range out.Messages {
		if m.Role != llm.RoleTool {
			continue
		}
		toolCount++
		if strings.Contains(m.Content, "[guard]") {
			hinted = m
		}
	}
	require.GreaterOrEqual(t, toolCount, 3, "改道阈值 3：至少 3 次失败才触发")
	require.NotNil(t, hinted, "达阈值后必须有 tool 消息含 [guard] 改道提示")
	assert.Contains(t, hinted.Content, "file_read", "提示应含工具名")
	assert.Contains(t, hinted.Content, "delegate_task", "只读类改道文案应列出 explore 子 Agent 选项")
}

// TestSummarizeToolErrorInLoop 长栈错误必须摘要后回填：原样回填会把上下文撑爆
// （连续几次失败就能吃掉整轮预算），但关键错误与工具名必须保留。
func TestSummarizeToolErrorInLoop(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, _ := seedSession(t, svc, msgRepo, ctx)

	long := "Traceback (most recent call last):\n  File \"<string>\", line 1\n    import pptx\n" +
		"ModuleNotFoundError: No module named 'pptx'\n" + strings.Repeat("at frame x\n", 30)
	failing := &stubTool{name: "exec", risk: tool.RiskExec, err: long}
	require.NoError(t, svc.tools.Registry().Register(failing))

	prov := &stubProvider{turns: []llm.ChatResponse{
		{ToolCalls: []llm.NormalizedToolCall{
			{ID: "t1", Name: "exec", Arguments: json.RawMessage(`{"command":"python"}`)},
		}, StopReason: "tool_use"},
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
	}}

	mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
	loop := svc.newCoreLoop(coreLoopSpec{
		Ses: &domain.ChatSessionDO{ID: ses.ID}, RunID: "RUN_1", AssistantMsgID: "MSG_1",
		Def: core.Agent(core.AgentDefault), Provider: prov, Model: "stub-model",
		ToolNames: []string{"exec"}, Sink: core.FuncSink(mapper.handle),
	})

	out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("run")})
	require.NoError(t, err)
	assert.Equal(t, core.ReasonEndTurn, out.Reason)

	var found bool
	for _, m := range out.Messages {
		if m.Role != llm.RoleTool {
			continue
		}
		found = true
		assert.NotContains(t, m.Content, "at frame x", "长栈必须截断，原始 30 行不应进上下文")
		assert.Contains(t, m.Content, "(truncated", "超长应出现截断标记")
		assert.Contains(t, m.Content, "ModuleNotFoundError", "关键错误必须保留")
		assert.Contains(t, m.Content, "工具执行失败", "首行固定为工具名提示")
	}
	assert.True(t, found, "必须至少落一条 tool 消息")
}

// ===== 自定义档案 =====

// TestAgentProfileLifecycle 校验 upsert → 注册表同步 → 启停 → 删除全链路。
func TestAgentProfileLifecycle(t *testing.T) {
	gdb := newChatOpsTestDB(t)
	svc := NewAgentProfileService(repo.NewAgentProfileRepo(gdb))
	ctx := t.Context()

	enabled := true
	created, err := svc.Upsert(ctx, &domain.AgentProfileREQ{
		Name:         "translator",
		Description:  "中英翻译",
		SystemPrompt: "你是翻译助手，只输出译文。",
		ToolsAllow:   []string{"webfetch"},
		MaxTurns:     6,
		Enabled:      &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, "translator", created.Name)
	require.Equal(t, []string{"webfetch"}, created.ToolsAllow)

	// 注册表已同步：core.Agent 命中自定义定义
	def := core.Agent("translator")
	require.Equal(t, "translator", def.Name)
	require.Equal(t, "你是翻译助手，只输出译文。", def.Persona)
	require.Equal(t, 6, def.Budget.MaxTurns)
	found := false
	for _, d := range core.AllAgents() {
		if d.Name == "translator" {
			found = true
		}
	}
	require.True(t, found)

	// 内置保留名拒绝（内置已收敛为 default / explore）
	_, err = svc.Upsert(ctx, &domain.AgentProfileREQ{Name: core.AgentExplore})
	require.Error(t, err)

	// 非法名拒绝
	_, err = svc.Upsert(ctx, &domain.AgentProfileREQ{Name: "Bad Name!"})
	require.Error(t, err)

	// 停用后从注册表摘除
	require.NoError(t, svc.SetEnabled(ctx, "translator", false))
	require.Equal(t, "default", core.Agent("translator").Name)

	// 重新启用即回归注册表
	require.NoError(t, svc.SetEnabled(ctx, "translator", true))
	require.Equal(t, "translator", core.Agent("translator").Name)

	// 删除后回退 default
	require.NoError(t, svc.Delete(ctx, "translator"))
	require.Equal(t, "default", core.Agent("translator").Name)
	_, err = svc.List(ctx)
	require.NoError(t, err)
}
