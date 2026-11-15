// 提问卡全链路：模型调 ask 工具 → 引擎阻塞等待 → 服务端广播 chat:ask →
// 界面经 /chat/answer 应答 → 引擎那一轮继续往下走。
// 这是唯一一条跨「引擎工具 → service 编排 → SSE → HTTP 回填」的阻塞式链路，
// 任何一环断了都是「模型卡住不动」，所以整条钉死。
package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
)

// askScriptServer 是会调 ask 工具的假上游：第一次请求发工具调用，
// 之后回纯文本——工具跑完后引擎要再调一次模型，那次就该收尾了。
func askScriptServer(t *testing.T) *scriptServer {
	t.Helper()
	s := newScriptServer(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_ask","function":{"name":"ask","arguments":"{\"method\":\"confirm\",\"title\":\"要继续吗\",\"message\":\"是否执行这个操作\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	})
	s.followUpChunks = []string{
		`{"choices":[{"delta":{"content":"好的，继续。"}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	return s
}

// TestAskQuestionChain 断言提问卡的三条契约：
// 提问广播出去且带完整字段、应答后引擎那一轮继续、待答清单能补回卡片。
func TestAskQuestionChain(t *testing.T) {
	t.Run("提问广播与应答", func(t *testing.T) {
		h := newHarness(t)
		cap := &frameCapture{}
		cancel := h.app.Hub.Subscribe(0, cap.sink)
		defer cancel()
		script := askScriptServer(t)
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "帮我做件事"})
		h.mustOK(env, code, "send", nil)

		// 提问帧必须广播出来，字段要全：界面照这些字段画卡片。
		askFrames := cap.waitFrames(t, "chat:ask", 1)
		askID, _ := askFrames[0].Data["ask_id"].(string)
		if !strings.HasPrefix(askID, "ASK_") {
			t.Fatalf("提问 ID 该带 ASK_ 前缀，实际 %q", askID)
		}
		if got, _ := askFrames[0].Data["ask_method"].(string); got != "confirm" {
			t.Fatalf("提问形态该是 confirm，实际 %q", got)
		}
		if got, _ := askFrames[0].Data["ask_title"].(string); got != "要继续吗" {
			t.Fatalf("提问标题该带上，实际 %q", got)
		}
		if got, _ := askFrames[0].Data["ask_message"].(string); got != "是否执行这个操作" {
			t.Fatalf("提问正文该带上，实际 %q", got)
		}

		// 引擎在等回答时这一轮还没结束：提问是阻塞式的。
		if !h.app.Chat.Busy() {
			t.Fatal("提问等待期间引擎该处于运行态")
		}

		// 应答：确认。
		env, code = h.call("POST", "/chat/answer", domain.ChatAnswerREQ{
			SessionFile: "", AskId: askID, Confirmed: new(true),
		})
		h.mustOK(env, code, "answer", nil)

		// 应答后引擎继续往下走并收尾。
		waitIdle(t, h)
		if len(script.requestBodies()) < 2 {
			t.Fatalf("应答后引擎该再发一次上游请求，实际 %d 次", len(script.requestBodies()))
		}
		// 回答内容要进下一轮上下文：模型得知道自己被问了什么。
		last := script.requestBodies()[len(script.requestBodies())-1]
		if !strings.Contains(last, "用户回答：是") {
			t.Fatalf("回答该进下一轮上下文，实际 %s", last)
		}
	})

	t.Run("待答清单能补回卡片", func(t *testing.T) {
		h := newHarness(t)
		cap := &frameCapture{}
		cancel := h.app.Hub.Subscribe(0, cap.sink)
		defer cancel()
		script := askScriptServer(t)
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "帮我做件事"})
		h.mustOK(env, code, "send", nil)
		askFrames := cap.waitFrames(t, "chat:ask", 1)
		askID, _ := askFrames[0].Data["ask_id"].(string)

		// 界面刷新时靠待答清单把卡片补回来：这是「刷新后提问卡不见了」的解药。
		pending := h.app.Chat.AskPending()
		if len(pending) != 1 {
			t.Fatalf("待答清单该有 1 条，实际 %d 条", len(pending))
		}
		if pending[0].AskId != askID {
			t.Fatalf("待答清单的 ID 该与广播一致，实际 %q", pending[0].AskId)
		}
		if pending[0].AskTitle != "要继续吗" {
			t.Fatalf("待答清单该带标题，实际 %q", pending[0].AskTitle)
		}

		// 应答完就该从清单里消失：留着会让界面补回一张已经答过的卡。
		env, code = h.call("POST", "/chat/answer", domain.ChatAnswerREQ{
			SessionFile: "", AskId: askID, Confirmed: new(false),
		})
		h.mustOK(env, code, "answer", nil)
		waitIdle(t, h)
		if pending := h.app.Chat.AskPending(); len(pending) != 0 {
			t.Fatalf("答完后待答清单该清空，实际 %d 条", len(pending))
		}
	})

	t.Run("已作废的提问回退成普通回复", func(t *testing.T) {
		h := newHarness(t)
		env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
		h.mustOK(env, code, "workspace", nil)

		// 提问超时或中止后作答：用户说的话不该凭空消失，
		// 要作为一条普通用户消息落进会话。
		env, code = h.call("POST", "/chat/answer", domain.ChatAnswerREQ{
			SessionFile: "", AskId: "ASK_不存在", Value: "事后补的一句",
		})
		h.mustOK(env, code, "answer", nil)

		env, code = h.call("GET", "/chat/state", nil)
		h.mustOK(env, code, "state", nil)
		var st domain.ChatStateRESP
		if err := json.Unmarshal(env.Data, &st); err != nil {
			t.Fatalf("解析 state 失败：%v", err)
		}
		found := false
		for _, turn := range st.History {
			for _, block := range turn.TurnBlocks {
				if strings.Contains(block.BlockText, "事后补的一句") {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("作废提问的回答该落成一条用户消息")
		}
	})
}
