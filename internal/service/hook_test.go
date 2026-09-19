package service

// 用户钩子契约测试。
//
// 钩子写错的两种后果都难从现象反推：在错误的工具上生效、或完全静默失效。
// 因此只钉住这两类契约——匹配式（含写入期校验）与子进程协议（输出解析 / 事件名归一 /
// Stop 续跑）。排序等实现细节不在此列。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/domain"
)

// TestHookMatcher 匹配式三种写法与写入期校验。
//
// 空/* 匹配全部；只含字母数字下划线与竖线按精确名称名单；含其他字符按正则。
// 非法正则在写入期即拒绝——落库后到运行期才发现会让钩子静默失效。
func TestHookMatcher(t *testing.T) {
	t.Run("匹配语义", func(t *testing.T) {
		cases := []struct {
			name    string
			matcher string
			tool    string
			want    bool
		}{
			{"空匹配全部", "", "exec", true},
			{"星号匹配全部", "*", "write_file", true},
			{"名单命中", "Write|Edit|exec", "Edit", true},
			{"名单未命中", "Write|Edit", "exec", false},
			{"名单不是子串匹配", "Write", "write_file", false},
			{"正则命中", `^(write|edit)_.*$`, "write_file", true},
			{"正则未命中", `^(write|edit)_.*$`, "exec", false},
			{"非法正则视为不匹配", `([unclosed`, "exec", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, hookMatches(tc.matcher, tc.tool))
			})
		}
	})

	t.Run("写入期校验", func(t *testing.T) {
		require.NoError(t, checkMatcher(""))
		require.NoError(t, checkMatcher("*"))
		require.NoError(t, checkMatcher("Write|Edit"))
		require.NoError(t, checkMatcher(`^mcp__.*$`))
		require.Error(t, checkMatcher(`([unclosed`))
	})
}

// TestHookSubprocessContract 子进程协议：stdout 决策解析、历史事件名归一、Stop 续跑判定。
func TestHookSubprocessContract(t *testing.T) {
	t.Run("输出解析与退出码", func(t *testing.T) {
		cases := []struct {
			name       string
			stdout     string
			exitCode   int
			wantPerm   string
			wantReason string
			wantCtx    string
		}{
			{name: "空输出无意见", stdout: "", exitCode: 0},
			{name: "退出码 2 等价拒绝", stdout: "", exitCode: 2, wantPerm: "deny"},
			{name: "退出码 2 带理由", stdout: `{"reason":"not allowed"}`, exitCode: 2, wantPerm: "deny", wantReason: "not allowed"},
			{
				name:     "标准拒绝",
				stdout:   `{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"prod dir"}}`,
				exitCode: 0, wantPerm: "deny", wantReason: "prod dir",
			},
			{
				name:     "标准询问",
				stdout:   `{"hookSpecificOutput":{"permissionDecision":"ask"}}`,
				exitCode: 0, wantPerm: "ask",
			},
			{
				name:     "behavior 形态的拒绝",
				stdout:   `{"hookSpecificOutput":{"decision":{"behavior":"deny","message":"policy"}}}`,
				exitCode: 0, wantPerm: "deny", wantReason: "policy",
			},
			{
				name:     "附加上下文",
				stdout:   `{"hookSpecificOutput":{"additionalContext":"只改相关文件"}}`,
				exitCode: 0, wantCtx: "只改相关文件",
			},
			{name: "下划线别名兼容", stdout: `{"additional_context":"legacy"}`, exitCode: 0, wantCtx: "legacy"},
			{name: "噪声输出中取 JSON 行", stdout: "log line\n{\"additionalContext\":\"ok\"}\nmore log", exitCode: 0, wantCtx: "ok"},
			{name: "非 JSON 输出忽略", stdout: "Traceback (most recent call last)", exitCode: 0},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := parseHookOutput([]byte(tc.stdout))
				d.ExitCode = tc.exitCode
				perm, reason := d.ToolPermission()
				assert.Equal(t, tc.wantPerm, perm)
				assert.Equal(t, tc.wantReason, reason)
				assert.Equal(t, tc.wantCtx, d.ContextText())
			})
		}
	})

	t.Run("历史事件名归一", func(t *testing.T) {
		assert.Equal(t, domain.HookEventPreToolUse, domain.NormalizeHookEvent("before_tool"))
		assert.Equal(t, domain.HookEventPostToolUse, domain.NormalizeHookEvent("after_tool"))
		assert.Equal(t, domain.HookEventSessionStart, domain.NormalizeHookEvent("run_start"))
		assert.Equal(t, domain.HookEventStop, domain.NormalizeHookEvent("run_end"))
		assert.Equal(t, domain.HookEventPreToolUse, domain.NormalizeHookEvent("PreToolUse"))
		assert.False(t, isHookEvent("bogus_event"))
	})

	t.Run("Stop 续跑判定", func(t *testing.T) {
		block := func(v bool) *bool { return &v }
		cases := []struct {
			name   string
			dec    domain.HookDecision
			reason string
			want   bool
		}{
			{name: "block 带理由", dec: domain.HookDecision{Decision: "block", Reason: "还缺测试结果"}, reason: "还缺测试结果", want: true},
			{name: "block 无反馈不续跑", dec: domain.HookDecision{Decision: "block"}, want: false},
			{name: "退出码 2 带理由", dec: domain.HookDecision{ExitCode: 2, Reason: "补齐说明"}, reason: "补齐说明", want: true},
			{name: "continue 带上下文", dec: domain.HookDecision{Continue: block(true), AdditionalContext: "再检查一次"}, reason: "再检查一次", want: true},
			{name: "allow 不续跑", dec: domain.HookDecision{Decision: "allow"}, want: false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				feedback, block := stopFeedback(tc.dec)
				assert.Equal(t, tc.want, block)
				assert.Equal(t, tc.reason, feedback)
			})
		}
	})
}
