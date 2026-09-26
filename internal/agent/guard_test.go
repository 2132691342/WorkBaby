// 护栏中间件链测试：拒绝语义（Refused 非 Err）、权限矩阵、熔断与失败改道注入。

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
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
			{"yolo 模式不可逆仍询问", ModeYolo, tool.RiskDestructive, false, true, RefuseApproval},
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

	t.Run("AdaptiveLoopGuard 同工具连续失败注入改道提示", func(t *testing.T) {
		failing := &mockTool{
			name: "exec", risk: tool.RiskExec,
			fn: func(_ json.RawMessage) tool.ToolResult {
				return tool.ToolResult{Err: pkg.Wrap(4006, "exec failed", errors.New("exit status 1"))}
			},
		}
		// threshold=2 让测试更紧凑；只验证「计数+提示+」语义
		h := Chain(AdaptiveLoopGuard(2))(Executor(ExecOptions{}))
		ctx := context.Background()
		args := json.RawMessage(`{"command":"python","args":["-c","print"]}`)

		// 1) 第一次失败：记 1，未达阈值，原文回填
		r1 := h(ctx, Call{ID: "1", Name: "exec", Args: args, Tool: failing})
		assert.Error(t, r1.Err)
		assert.Equal(t, "1", r1.Meta["same_failure_count"])
		assert.NotContains(t, r1.Content, "[guard]", "未达阈值不应附加 hint")

		// 2) 第二次失败：达阈值，附加改道提示 + Meta 标记
		r2 := h(ctx, Call{ID: "2", Name: "exec", Args: args, Tool: failing})
		assert.Error(t, r2.Err)
		assert.Equal(t, "2", r2.Meta["same_failure_count"])
		assert.Equal(t, "1", r2.Meta["adaptive_hint"], "达标后 Meta 必须打标记")
		assert.Contains(t, r2.Content, "[guard]", "达标后必须追加 hint")
		assert.Contains(t, r2.Content, "exec", "hint 必须含工具名")
		assert.Contains(t, r2.Content, "检查命令参数", "exec 类提示核对参数")

		// 3) 任意一次成功：streak 清零，下次失败重新计数
		ok := &mockTool{name: "exec", risk: tool.RiskExec}
		h2 := Chain(AdaptiveLoopGuard(2))(Executor(ExecOptions{}))
		_ = h2(ctx, Call{ID: "3", Name: "exec", Args: args, Tool: ok})
		r3 := h2(ctx, Call{ID: "4", Name: "exec", Args: args, Tool: failing})
		assert.Equal(t, "1", r3.Meta["same_failure_count"], "成功后失败计数归零")
	})

	t.Run("summarizeToolError 长栈截断", func(t *testing.T) {
		// 模拟 python 那种 30+ 行的栈：原样回填会把上下文撑爆
		var sb strings.Builder
		for i := 0; i < 30; i++ {
			sb.WriteString("at frame ")
			sb.WriteString(strconv.Itoa(i))
			sb.WriteString(" of 30\n")
		}
		long := sb.String()
		out := summarizeToolError("exec", errors.New(long))
		assert.Contains(t, out, "工具执行失败: exec", "首行固定为工具名")
		assert.Contains(t, out, "(truncated", "超过 stackLines 必须截断")
		assert.Less(t, len(out), len(long), "截断后必须比原文短")
	})

}

// TestRepeatGuardStepReuse 覆盖步骤记忆复用语义：检查点恢复的键续跑直接复用（不执行、不耗熔断计数），
// 本 run 新记的结果只持久化不复用（防「写后重读」拿到旧值）。
func TestRepeatGuardStepReuse(t *testing.T) {
	restored := NewMapSteps(map[string]string{`echo|{"a":1}`: "cached-result"})
	echo := &mockTool{name: "echo", risk: tool.RiskReadOnly}
	call := Call{ID: "1", Name: "echo", Args: json.RawMessage(`{"a":1}`), Tool: echo}

	res := Chain(RepeatGuard(2, restored))(Executor(ExecOptions{}))(context.Background(), call)
	assert.False(t, res.Refused)
	assert.Equal(t, "cached-result", res.Content)
	assert.Equal(t, "reused", res.Meta["idempotent"])
	assert.Equal(t, 0, echo.callCount(), "恢复键命中直接复用，不重放副作用")

	fresh := NewMapSteps(nil)
	h := Chain(RepeatGuard(3, fresh))(Executor(ExecOptions{}))
	assert.False(t, h(context.Background(), call).Refused)
	assert.False(t, h(context.Background(), Call{ID: "2", Name: "echo", Args: json.RawMessage(`{"a":1}`), Tool: echo}).Refused)
	assert.Equal(t, 2, echo.callCount(), "本 run 新记不复用，必须真执行")
	assert.Len(t, fresh.Snapshot(), 1, "新记结果随检查点持久化")
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
