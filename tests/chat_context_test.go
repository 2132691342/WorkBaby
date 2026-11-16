// 上下文与流式呈现：占用读数、水位提示、压缩换上下文、块表状态机。
// 压缩是唯一会改写「模型看到什么」的动作，读数是界面唯一的容量指示，
// 块表是界面「此刻该显示什么」的唯一答案——三者都跨引擎与编排两层。
package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

// newStream 造一对（翻译器 + 帧捕获）：每段场景各自独立，互不串帧。
func newStream(t *testing.T) (*service.Stream, *frameCapture) {
	t.Helper()
	hub := sse.New(64)
	cap := &frameCapture{}
	cancel, _ := hub.Subscribe(0, cap.sink)
	t.Cleanup(cancel)
	return service.NewStream(hub), cap
}

func TestChatContext(t *testing.T) {
	// 上下文读数：首屏总装的读数与计数、水位提示的触发条件。
	t.Run("上下文读数", func(t *testing.T) {
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

		t.Run("用量分项有真值", func(t *testing.T) {
			h := newHarness(t)
			script := newScriptServer(t, []string{
				`{"choices":[{"delta":{"content":"回答"}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
				`{"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":40}}}`,
			})
			bindWorkspaceAndModel(t, h, script.server.URL)

			env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			h.mustOK(env, code, "send", nil)
			waitIdle(t, h)

			env, code = h.call("GET", "/chat/state", nil)
			var st domain.ChatStateRESP
			h.mustOK(env, code, "state", &st)
			if st.Stats == nil {
				t.Fatal("stats 不该为空")
			}
			// 分项为零是假数据：界面按输入 / 输出 / 缓存分开显示，只给合计等于三项全报 0。
			if st.Stats.InputTokens != 120 {
				t.Fatalf("input_tokens 应为 120，实际 %d", st.Stats.InputTokens)
			}
			if st.Stats.OutputTokens != 30 {
				t.Fatalf("output_tokens 应为 30，实际 %d", st.Stats.OutputTokens)
			}
			if st.Stats.CacheReadTokens != 40 {
				t.Fatalf("cache_read_tokens 应为 40，实际 %d", st.Stats.CacheReadTokens)
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
	})

	// 上下文压缩：手动压缩换掉上下文但真相不删、没配模型给可行动错误、
	// 自动压缩按水位与开关触发。
	t.Run("上下文压缩", func(t *testing.T) {
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
	})

	// 流式块表：增量按块累加、工具块从出现到收尾、落盘消息权威覆盖但保留工具读数、
	// 用量透传、失败与收尾都收得住。
	t.Run("流式块表", func(t *testing.T) {
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
	})
}
