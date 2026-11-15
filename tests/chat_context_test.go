// 上下文读数与压缩：占用估算、首屏总装、水位提示、压缩换上下文。
// 压缩是唯一会改写「模型看到什么」的动作，读数是界面唯一的容量指示，
// 两者的边界都跨引擎与编排两层，所以真上游 + 真会话文件来测。
package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// TestChatContextReadout 断言上下文读数的两条链路：
// 首屏总装的读数与计数、水位提示的触发条件。
func TestChatContextReadout(t *testing.T) {
	t.Run("首屏读数与计数", func(t *testing.T) {
		h := newHarness(t)
		script := newScriptServer(t, replyChunks("回答"))
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
		h.mustOK(env, code, "send", nil)
		waitIdle(t, h)

		env, code = h.call("GET", "/chat/state", nil)
		h.mustOK(env, code, "state", nil)
		var st domain.ChatStateRESP
		if err := json.Unmarshal(env.Data, &st); err != nil {
			t.Fatalf("解析 state 失败：%v", err)
		}
		if st.ContextUsage == nil || st.ContextUsage.UsageTokens == nil {
			t.Fatalf("context_usage 应有读数，实际 %+v", st.ContextUsage)
		}
		if st.ContextUsage.ContextWindow != 100000 {
			t.Fatalf("context_window 应取模型定义，实际 %d", st.ContextUsage.ContextWindow)
		}
		if st.Stats == nil {
			t.Fatal("stats 不该为空")
		}
		if st.Stats.UserMessages != 1 || st.Stats.AssistantMessages != 1 {
			t.Fatalf("消息计数不对：%+v", st.Stats)
		}
		if st.Stats.TotalMessages != 2 {
			t.Fatalf("消息总数应为 2，实际 %d", st.Stats.TotalMessages)
		}
		// 低水位不给提示。
		if st.ContextWarn != "" {
			t.Fatalf("低水位不该出提示，实际 %q", st.ContextWarn)
		}
	})

	t.Run("水位提示", func(t *testing.T) {
		h := newHarness(t)
		script := newScriptServer(t, []string{
			`{"choices":[{"delta":{"content":"好"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":90000,"completion_tokens":1000}}`,
		})
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
		h.mustOK(env, code, "send", nil)
		waitIdle(t, h)

		// 9 万 / 10 万 = 0.9：过八成，自动压缩默认开 → 提示。
		env, code = h.call("GET", "/chat/state", nil)
		h.mustOK(env, code, "state", nil)
		var st domain.ChatStateRESP
		_ = json.Unmarshal(env.Data, &st)
		if st.ContextUsage == nil || st.ContextUsage.UsagePercent == nil {
			t.Fatalf("水位应有读数，实际 %+v", st.ContextUsage)
		}
		if *st.ContextUsage.UsagePercent <= 0.8 {
			t.Fatalf("水位应过八成，实际 %f", *st.ContextUsage.UsagePercent)
		}
		if st.ContextWarn == "" {
			t.Fatal("过八成且自动压缩开着应给水位提示")
		}

		// 关掉自动压缩：同水位不再提示。
		if err := h.app.Chat.SetAutoCompaction("", false); err != nil {
			t.Fatalf("关自动压缩失败：%v", err)
		}
		env, code = h.call("GET", "/chat/state", nil)
		h.mustOK(env, code, "state-2", nil)
		st = domain.ChatStateRESP{}
		_ = json.Unmarshal(env.Data, &st)
		if st.ContextWarn != "" {
			t.Fatalf("关了自动压缩不该提示，实际 %q", st.ContextWarn)
		}
	})
}

// TestChatCompaction 断言压缩的三条契约：
// 手动压缩换掉上下文但真相不删、没配模型给可行动错误、自动压缩按水位与开关触发。
func TestChatCompaction(t *testing.T) {
	t.Run("手动压缩换掉上下文", func(t *testing.T) {
		h := newHarness(t)
		script := newSummaryServer(t, replyChunks("好的"))
		bindWorkspaceAndModel(t, h, script.server.URL)

		// 造两轮历史。
		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "压缩前的第一句"})
		h.mustOK(env, code, "send-1", nil)
		waitIdle(t, h)
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "压缩前的第二句"})
		h.mustOK(env, code, "send-2", nil)
		waitIdle(t, h)

		// 手动压缩：上游收到一次带「对话整理助手」的总结调用。
		env, code = h.call("POST", "/chat/compact", map[string]string{"session_file": "", "instructions": ""})
		h.mustOK(env, code, "compact", nil)
		bodies := script.requestBodies()
		if len(bodies) != 3 {
			t.Fatalf("应有两轮对话 + 一次压缩总结共 3 次请求，实际 %d", len(bodies))
		}
		if !strings.Contains(bodies[2], compactionSystemDigest) {
			t.Fatalf("压缩应发一次总结调用，实际 %s", bodies[2])
		}
		if !strings.Contains(bodies[2], "压缩前的第一句") {
			t.Fatalf("总结调用应带压缩前原文，实际 %s", bodies[2])
		}

		// 压缩后的下一轮：请求体含摘要引导与最新这句，不含压缩前原文。
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "压缩后的一句话"})
		h.mustOK(env, code, "send-3", nil)
		waitIdle(t, h)
		bodies = script.requestBodies()
		if len(bodies) != 4 {
			t.Fatalf("压缩后应再有一次对话请求，实际 %d", len(bodies))
		}
		next := bodies[3]
		if !strings.Contains(next, "以下是此前对话的摘要") || !strings.Contains(next, "用户问了目录结构") {
			t.Fatalf("压缩后的上下文应以摘要引导开头，实际 %s", next)
		}
		if !strings.Contains(next, "压缩后的一句话") {
			t.Fatalf("压缩后的消息该带上，实际 %s", next)
		}
		if strings.Contains(next, "压缩前的第一句") || strings.Contains(next, "压缩前的第二句") {
			t.Fatalf("压缩前原文不该再进上下文，实际 %s", next)
		}

		// 原文仍在会话历史里：压缩不删真相。
		env, code = h.call("GET", "/chat/state", nil)
		h.mustOK(env, code, "state", nil)
		var st domain.ChatStateRESP
		if err := json.Unmarshal(env.Data, &st); err != nil {
			t.Fatalf("解析 state 失败：%v", err)
		}
		found := false
		for _, turn := range st.History {
			for _, block := range turn.TurnBlocks {
				if strings.Contains(block.BlockText, "压缩前的第一句") {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("压缩前原文应仍在会话历史里（真相不删）")
		}
	})

	t.Run("没配模型给可行动错误", func(t *testing.T) {
		h := newHarness(t)
		env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
		h.mustOK(env, code, "workspace", nil)
		env, code = h.call("POST", "/chat/compact", map[string]string{})
		if env.Code != pkg.CodeNoProvider {
			t.Fatalf("没配模型压缩应回 %q，实际 %q（msg=%q）", pkg.CodeNoProvider, env.Code, env.Message)
		}
		if !strings.Contains(env.Message, "模型页") {
			t.Fatalf("错误应指引去模型页，实际 %q", env.Message)
		}
	})

	t.Run("自动压缩按水位触发", func(t *testing.T) {
		h := newHarness(t)
		// 对话回短文本 + 一帧大用量把水位顶到 0.9；总结调用回摘要。
		chunks := []string{
			`{"choices":[{"delta":{"content":"好"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":90000,"completion_tokens":1000}}`,
		}
		script := newSummaryServer(t, chunks)
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "把水位顶上去"})
		h.mustOK(env, code, "send-1", nil)
		waitIdle(t, h)

		// 下一句发送前应先压缩：请求顺序 = 总结调用 → 对话。
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "触发自动压缩的一句"})
		h.mustOK(env, code, "send-2", nil)
		// 压缩总结先发、对话后发：等齐 3 次（首句对话 + 总结 + 触发句的对话）再断言顺序。
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && len(script.requestBodies()) < 3 {
			time.Sleep(10 * time.Millisecond)
		}
		waitIdle(t, h)
		bodies := script.requestBodies()
		if len(bodies) < 3 {
			t.Fatalf("应有首句对话 + 压缩总结 + 触发句对话共 3 次请求，实际 %d", len(bodies))
		}
		if !strings.Contains(bodies[1], compactionSystemDigest) {
			t.Fatalf("第二轮对话前应先出总结调用，实际第 2 次请求 = %s", bodies[1])
		}
		if strings.Contains(bodies[2], compactionSystemDigest) {
			t.Fatalf("第 3 次应是正常对话而非又一次总结，实际 %s", bodies[2])
		}

		// 开关关掉：同水位不再压缩，只多一次对话请求。
		if err := h.app.Chat.SetAutoCompaction("", false); err != nil {
			t.Fatalf("关自动压缩失败：%v", err)
		}
		before := len(script.requestBodies())
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "开关关后的第一句"})
		h.mustOK(env, code, "send-3", nil)
		waitIdle(t, h)
		after := script.requestBodies()
		if len(after) != before+1 {
			t.Fatalf("关开关后应只多一次对话请求，实际 %d → %d", before, len(after))
		}
		for _, body := range after[before:] {
			if strings.Contains(body, compactionSystemDigest) {
				t.Fatalf("开关关后不该有总结调用，实际 %s", body)
			}
		}
	})
}
