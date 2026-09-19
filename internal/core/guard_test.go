package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// TestGuardChain 覆盖各护栏中间件的裁决语义：拒绝必须是 Refused 而非 Err（模型可改道）。
func TestGuardChain(t *testing.T) {
	t.Run("未暴露与未注册工具拒绝", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		h := Chain(ExposeGuard(map[string]bool{"other": true}))(Executor(ExecOptions{}))
		res := h(context.Background(), Call{ID: "1", Name: "echo", Args: json.RawMessage(`{}`), Tool: echo})

		assert.True(t, res.Refused, "拒绝不得带 Err，否则模型无法改道")
		assert.NoError(t, res.Err)
		assert.Equal(t, RefuseNotExposed, res.Meta["refused_reason"])
		assert.Equal(t, 0, echo.callCount())
	})

	t.Run("伪工具调用标记拒绝", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		// PolicyGuard 内置注入检测，无需独立 InjectionGuard。
		h := Chain(PolicyGuard(ModeDefault, nil, nil, nil))(Executor(ExecOptions{}))
		res := h(context.Background(), Call{ID: "1", Name: "echo", Args: json.RawMessage(`{"cmd":"<tool_call>"}`), Tool: echo})

		assert.True(t, res.Refused)
		assert.Equal(t, RefuseInjection, res.Meta["refused_reason"])
		assert.Equal(t, 0, echo.callCount())
	})

	t.Run("路径不受信任拒绝", func(t *testing.T) {
		write := &mockTool{name: "file_write", risk: tool.RiskWriteLocal}
		denyPath := func(_ context.Context, _ Call) (bool, string) { return false, "未登记目录" }
		h := Chain(PolicyGuard(ModeDefault, nil, nil, denyPath))(Executor(ExecOptions{}))
		res := h(context.Background(), Call{ID: "1", Name: "file_write", Args: json.RawMessage(`{"path":"/etc/passwd"}`), Tool: write})

		assert.True(t, res.Refused)
		assert.Equal(t, RefusePathTrust, res.Meta["refused_reason"])
		assert.Equal(t, 0, write.callCount())
	})

	t.Run("路径预检通过后进入审批门", func(t *testing.T) {
		execTool := &mockTool{name: "exec", risk: tool.RiskExec}
		allowPath := func(_ context.Context, _ Call) (bool, string) { return true, "" }
		h := Chain(PolicyGuard(ModeDefault, nil, &mockApprover{allow: true}, allowPath))(Executor(ExecOptions{}))
		res := h(context.Background(), Call{ID: "1", Name: "exec", Args: json.RawMessage(`{}`), Tool: execTool})

		assert.False(t, res.Refused)
		assert.Equal(t, 1, execTool.callCount())
	})

	t.Run("策略模式矩阵与审批", func(t *testing.T) {
		execTool := &mockTool{name: "exec", risk: tool.RiskExec}
		ctx := context.Background()
		cases := []struct {
			name    string
			mode    Mode
			risk    tool.RiskLevel
			approve bool
			refused bool
			reason  string
		}{
			{"default 模式执行需审批且被拒", ModeDefault, tool.RiskExec, false, true, RefuseApproval},
			{"default 模式执行经放行", ModeDefault, tool.RiskExec, true, false, ""},
			{"yolo 模式直接放行", ModeYolo, tool.RiskExec, false, false, ""},
			{"auto_edit 模式本地写放行", ModeAutoEdit, tool.RiskWriteLocal, false, false, ""},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				tl := &mockTool{name: execTool.name, risk: c.risk}
				h := Chain(PolicyGuard(c.mode, nil, &mockApprover{allow: c.approve}, nil))(Executor(ExecOptions{}))
				res := h(ctx, Call{ID: "1", Name: tl.name, Args: json.RawMessage(`{}`), Tool: tl})

				assert.Equal(t, c.refused, res.Refused)
				if c.refused {
					assert.Equal(t, c.reason, res.Meta["refused_reason"])
					assert.Equal(t, 0, tl.callCount())
				} else {
					assert.Equal(t, 1, tl.callCount())
				}
			})
		}
	})

	t.Run("自动编辑模式仍拦截执行类", func(t *testing.T) {
		execTool := &mockTool{name: "exec", risk: tool.RiskExec}
		h := Chain(PolicyGuard(ModeAutoEdit, nil, &mockApprover{allow: false}, nil))(Executor(ExecOptions{}))
		res := h(context.Background(), Call{ID: "1", Name: "exec", Args: json.RawMessage(`{}`), Tool: execTool})

		assert.True(t, res.Refused, "auto_edit 只放行只读与本地写")
		assert.Equal(t, 0, execTool.callCount())
	})

	t.Run("重复调用熔断", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		h := Chain(RepeatGuard(2, nil))(Executor(ExecOptions{}))
		ctx := context.Background()
		args := json.RawMessage(`{"b":1,"a":2}`)

		first := h(ctx, Call{ID: "1", Name: "echo", Args: args, Tool: echo})
		second := h(ctx, Call{ID: "2", Name: "echo", Args: args, Tool: echo})
		// 异序同参必须判为同一次调用（签名规范化）
		reordered := h(ctx, Call{ID: "3", Name: "echo", Args: json.RawMessage(`{"a":2,"b":1}`), Tool: echo})

		assert.False(t, first.Refused)
		assert.True(t, second.Refused, "连续第二次同参即熔断")
		assert.Equal(t, RefuseLoopGuard, second.Meta["refused_reason"])
		assert.True(t, reordered.Refused, "同参异序仍判重复")
		assert.Equal(t, 1, echo.callCount())
	})

	t.Run("已重用步骤不计入计数且不执行", func(t *testing.T) {
		echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
		store := &mockStepStore{m: map[string]string{}}
		h := Chain(RepeatGuard(2, store))(Executor(ExecOptions{}))
		ctx := context.Background()
		args := json.RawMessage(`{"a":1}`)

		// 第一次：正常执行并入库。
		h(ctx, Call{ID: "1", Name: "echo", Args: args, Tool: echo})
		assert.Equal(t, 1, echo.callCount())
		// 第二次：命中重用，不增计数、不重复执行（callCount 仍 1）。
		again := h(ctx, Call{ID: "2", Name: "echo", Args: args, Tool: echo})
		assert.False(t, again.Refused)
		assert.Equal(t, "reused", again.Meta["idempotent"])
		assert.Equal(t, 1, echo.callCount())
	})
}

// TestCompressKeepsToolPairing 压缩的硬约束：绝不制造孤儿 tool 或空 assistant。
func TestCompressKeepsToolPairing(t *testing.T) {
	msgs := []*llm.Message{
		llm.SystemMessage("system"),
		llm.UserMessage("第一轮"),
		llm.AssistantMessage("开始", []llm.ToolCall{
			{ID: "t1", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"path":"a"}`}},
			{ID: "t2", Type: "function", Function: llm.FunctionCall{Name: "exec", Arguments: `{"cmd":"ls"}`}},
		}),
		llm.ToolMessage("t1", "file_read", "AAA"),
		llm.ToolMessage("t2", "exec", "BBB"),
		llm.UserMessage("第二轮"),
		llm.AssistantMessage("结论", nil),
	}

	t.Run("预算充足时不改动", func(t *testing.T) {
		out, info := MicroCompressor{}.Compress(msgs, 1_000_000)
		assert.Len(t, out, len(msgs))
		assert.Equal(t, 0, info.Removed)
	})

	t.Run("超预算折叠后配对完整", func(t *testing.T) {
		out, info := MicroCompressor{}.Compress(msgs, 1)
		require.NotEmpty(t, out)
		assert.Greater(t, info.Removed, 0)
		assertNoOrphanTools(t, out)
		assert.Equal(t, "结论", out[len(out)-1].Content, "最后一轮永不丢")
	})
}

// assertNoOrphanTools 断言每条带 tool_calls 的 assistant 其后都有等量 tool 结果。
func assertNoOrphanTools(t *testing.T, msgs []*llm.Message) {
	t.Helper()
	for i, m := range msgs {
		if m.Role != llm.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		want := make(map[string]bool, len(m.ToolCalls))
		for _, c := range m.ToolCalls {
			want[c.ID] = true
		}
		got := make(map[string]bool)
		for j := i + 1; j < len(msgs) && msgs[j].Role == llm.RoleTool; j++ {
			got[msgs[j].ToolCallID] = true
		}
		for id := range want {
			assert.True(t, got[id], "tool_call %s 缺少配对结果（孤儿 tool）", id)
		}
	}
}

// mockApprover 测试用审批门。
type mockApprover struct {
	allow bool
	seen  []string
}

func (a *mockApprover) Approve(_ context.Context, call Call, risk string) bool {
	a.seen = append(a.seen, call.Name+":"+risk)
	return a.allow
}

// mockStepStore 测试用步骤记忆。
type mockStepStore struct{ m map[string]string }

func (s *mockStepStore) Load(k string) (string, bool) {
	v, ok := s.m[k]
	return v, ok
}

func (s *mockStepStore) Store(k, v string) { s.m[k] = v }

// TestExploreReadOnlyPolicy 只读探索 Agent 的暴露集不得含任何写工具（安全不变量）。
func TestExploreReadOnlyPolicy(t *testing.T) {
	defs := []llm.ToolDefinition{
		{Name: "file_read"}, {Name: "file_write"}, {Name: "file_edit"},
		{Name: "exec"}, {Name: "file_grep"}, {Name: "websearch"},
	}
	names := Agent(AgentExplore).ExposedNames(defs)

	assert.ElementsMatch(t, []string{"file_read", "file_grep", "websearch"}, names)
	for _, banned := range []string{"file_write", "file_edit", "exec"} {
		assert.NotContains(t, names, banned, "只读探索不允许出现写工具")
	}
}
