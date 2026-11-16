// 聊天运行时：队列语义、思考档接线、发送失败面、提问卡全链路。
// 这四件事都被界面直接依赖，且都跨「接口 → 编排 → 引擎 → 上游 → SSE」多层，
// 所以一律用真上游 + 真会话文件来测。
package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
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

func TestChatFlow(t *testing.T) {
	// 队列语义：插话进本轮、follow_up 在本轮收尾后另起一轮、清空两条一起清掉。
	t.Run("队列语义", func(t *testing.T) {
		t.Run("插话被清掉后不进下一轮", func(t *testing.T) {
			h := newHarness(t)
			cap := &frameCapture{}
			cancel, _ := h.app.Hub.Subscribe(0, cap.sink)
			defer cancel()
			script := newScriptServer(t, replyChunks("收到"))
			bindWorkspaceAndModel(t, h, script.server.URL)

			// 起一轮，趁第一轮没结束排进一条插话。
			env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "第一句"})
			h.mustOK(env, code, "send", nil)
			env, code = h.call("POST", "/chat/steer", domain.ChatSendREQ{Message: "被清掉的插话"})
			h.mustOK(env, code, "steer", nil)

			out := h.app.Chat.ClearQueue()
			if len(out.Steering) != 1 || out.Steering[0] != "被清掉的插话" {
				t.Fatalf("清空应返回被清掉的文本，实际 %+v", out)
			}
			// 收尾后的下一轮请求体里不该再出现被清掉的话。
			waitIdle(t, h)
			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "第二句"})
			h.mustOK(env, code, "send-2", nil)
			waitIdle(t, h)

			bodies := script.requestBodies()
			if len(bodies) != 2 {
				t.Fatalf("上游应只收到 2 次请求，实际 %d", len(bodies))
			}
			if strings.Contains(bodies[1], "被清掉的插话") {
				t.Fatalf("被清掉的插话不该进下一轮上下文，实际 %s", bodies[1])
			}
			// 广播过一次空队：界面的两条队据此归零。
			last := cap.of("chat:queue")
			if len(last) == 0 {
				t.Fatal("清空后应广播一次 chat:queue")
			}
			raw, _ := json.Marshal(last[len(last)-1].Data)
			if !strings.Contains(string(raw), `"follow_up"`) {
				t.Fatalf("清空广播应带两条队，实际 %s", raw)
			}
		})

		t.Run("follow_up 在本轮收尾后另起一轮", func(t *testing.T) {
			h := newHarness(t)
			script := newScriptServer(t, replyChunks("好"))
			bindWorkspaceAndModel(t, h, script.server.URL)

			env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "开场"})
			h.mustOK(env, code, "send", nil)
			env, code = h.call("POST", "/chat/followup", domain.ChatSendREQ{Message: "追加的问题"})
			h.mustOK(env, code, "followup", nil)

			// 两条队都该出现：follow_up 进追加队，不混进插话队。
			env, code = h.call("GET", "/chat/state", nil)
			h.mustOK(env, code, "state", nil)
			var st domain.ChatStateRESP
			if err := json.Unmarshal(env.Data, &st); err != nil {
				t.Fatalf("解析 state 失败：%v", err)
			}
			if len(st.Queued.FollowUp) != 1 || st.Queued.FollowUp[0] != "追加的问题" {
				t.Fatalf("follow_up 应在追加队里，实际 %+v", st.Queued)
			}
			if len(st.Queued.Steering) != 0 {
				t.Fatalf("follow_up 不该混进插话队，实际 %+v", st.Queued)
			}

			// 本轮收尾后自动派发：上游共收到 2 次请求，第二次带着追加的话。
			waitIdle(t, h)
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if len(script.requestBodies()) >= 2 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			bodies := script.requestBodies()
			if len(bodies) != 2 {
				t.Fatalf("follow_up 应自动再起一轮（共 2 次请求），实际 %d", len(bodies))
			}
			if !strings.Contains(bodies[1], "追加的问题") {
				t.Fatalf("第二轮上下文应带追加的话，实际 %s", bodies[1])
			}
		})
	})

	// 思考档接线：切档写回设置、非法档位拒绝且不落盘、
	// 进请求体（low 原样、off 不发字段）。
	t.Run("思考档接线", func(t *testing.T) {
		h := newHarness(t)
		script := newScriptServer(t, replyChunks("好"))
		bindWorkspaceAndModel(t, h, script.server.URL)

		// 切到 low：写进设置台账。
		if err := h.app.Chat.SetThinking("", "low"); err != nil {
			t.Fatalf("切思考档失败：%v", err)
		}
		if got, _ := h.store.Get(service.KeyThinkingLevel); got != "low" {
			t.Fatalf("思考档应写回设置，实际 %q", got)
		}
		// 非法档位拒绝且不落盘。
		if err := h.app.Chat.SetThinking("", "ultra"); err == nil {
			t.Fatal("未知思考档应被拒绝")
		}
		if got, _ := h.store.Get(service.KeyThinkingLevel); got != "low" {
			t.Fatalf("被拒的切档不该落盘，实际 %q", got)
		}

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
		h.mustOK(env, code, "send", nil)
		waitIdle(t, h)
		bodies := script.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
		}
		var wire map[string]any
		if err := json.Unmarshal([]byte(bodies[0]), &wire); err != nil {
			t.Fatalf("解析请求体失败：%v", err)
		}
		if wire["reasoning_effort"] != "low" {
			t.Fatalf("low 档应原样进请求体，实际 %v", wire["reasoning_effort"])
		}

		// off：字段整个不发，不是发个 "off" 过去。
		if err := h.app.Chat.SetThinking("", "off"); err != nil {
			t.Fatalf("切到 off 失败：%v", err)
		}
		script.resetBodies()
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "再来"})
		h.mustOK(env, code, "send-2", nil)
		waitIdle(t, h)
		bodies = script.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("上游应再收 1 次请求，实际 %d", len(bodies))
		}
		wire = map[string]any{}
		if err := json.Unmarshal([]byte(bodies[0]), &wire); err != nil {
			t.Fatalf("解析请求体失败：%v", err)
		}
		if _, exists := wire["reasoning_effort"]; exists {
			t.Fatalf("off 档不该发 reasoning_effort，实际 %v", wire["reasoning_effort"])
		}
	})

	// 输出上限与新会话默认思考档：两个设置都必须真的到达上游请求体，
	// 只写进台账不回读的设置在界面上看起来生效了，实际没有。
	t.Run("模型参数接线", func(t *testing.T) {
		t.Run("max_tokens 进请求体", func(t *testing.T) {
			h := newHarness(t)
			script := newScriptServer(t, replyChunks("好"))
			env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
			h.mustOK(env, code, "workspace", nil)
			env, code = h.call("POST", "/providers/upsert", domain.ProviderUpsertREQ{
				ProviderName: "up", BaseUrl: script.server.URL, ApiKey: "k",
				Models: []domain.ProviderModelEntryVO{{ModelId: "m", ContextWindow: 100000, MaxTokens: 4096}},
			})
			h.mustOK(env, code, "upsert", nil)
			if err := h.app.Chat.SetModel("", "up", "m"); err != nil {
				t.Fatalf("选模型失败：%v", err)
			}

			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			h.mustOK(env, code, "send", nil)
			waitIdle(t, h)

			bodies := script.requestBodies()
			if len(bodies) != 1 {
				t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
			}
			var wire map[string]any
			if err := json.Unmarshal([]byte(bodies[0]), &wire); err != nil {
				t.Fatalf("解析请求体失败：%v", err)
			}
			got, ok := wire["max_tokens"].(float64)
			if !ok || int(got) != 4096 {
				t.Fatalf("模型定义的 max_tokens 应进请求体（4096），实际 %v", wire["max_tokens"])
			}
		})

		t.Run("新会话默认思考档生效", func(t *testing.T) {
			h := newHarness(t)
			script := newScriptServer(t, replyChunks("好"))
			bindWorkspaceAndModel(t, h, script.server.URL)

			// 写默认档：这条键以前只写不读，新会话仍按 medium 发。
			env, code := h.call("POST", "/models/defaults", domain.ModelDefaultSaveREQ{
				DefaultProvider: "up", DefaultModel: "m", DefaultThinkingLevel: "off",
			})
			h.mustOK(env, code, "写默认档", nil)
			if got, _ := h.store.Get(service.KeyThinkingLevel); got != "" {
				t.Fatalf("当前档不该被默认档写坏，实际 %q", got)
			}

			// 新建会话：没有当前档时落到默认档。
			env, code = h.call("POST", "/chat/new", nil)
			h.mustOK(env, code, "新建会话", nil)
			env, code = h.call("GET", "/chat/state", nil)
			var st domain.ChatStateRESP
			h.mustOK(env, code, "state", &st)
			if st.ThinkingLevel != "off" {
				t.Fatalf("新会话应从默认档起步（off），实际 %q", st.ThinkingLevel)
			}

			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			h.mustOK(env, code, "send", nil)
			waitIdle(t, h)
			bodies := script.requestBodies()
			if len(bodies) != 1 {
				t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
			}
			var wire map[string]any
			if err := json.Unmarshal([]byte(bodies[0]), &wire); err != nil {
				t.Fatalf("解析请求体失败：%v", err)
			}
			if _, exists := wire["reasoning_effort"]; exists {
				t.Fatalf("默认档 off 不该发 reasoning_effort，实际 %v", wire["reasoning_effort"])
			}
		})
	})

	// 中止要把被作废的排队话术还回来：用户刚打的字不能无声消失。
	t.Run("中止交回排队话术", func(t *testing.T) {
		h := newHarness(t)
		script := newScriptServer(t, replyChunks("慢一点"))
		bindWorkspaceAndModel(t, h, script.server.URL)

		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "开场"})
		h.mustOK(env, code, "send", nil)
		env, code = h.call("POST", "/chat/steer", domain.ChatSendREQ{Message: "插一句话"})
		h.mustOK(env, code, "steer", nil)
		env, code = h.call("POST", "/chat/followup", domain.ChatSendREQ{Message: "追加一句"})
		h.mustOK(env, code, "followup", nil)

		env, code = h.call("POST", "/chat/stop", nil)
		var discarded domain.QueuedMessagesVO
		h.mustOK(env, code, "stop", &discarded)
		if len(discarded.Steering) != 1 || discarded.Steering[0] != "插一句话" {
			t.Fatalf("中止应回被作废的插话，实际 %+v", discarded)
		}
		if len(discarded.FollowUp) != 1 || discarded.FollowUp[0] != "追加一句" {
			t.Fatalf("中止应回被作废的追加话，实际 %+v", discarded)
		}
		waitIdle(t, h)
	})

	// 发送失败面：没配模型与坏指针都回可行动的 3001（并自愈清指针），
	// 真上游连不通时发送照常受理、运行期出 chat:error 且收尾不悬挂。
	t.Run("发送失败面", func(t *testing.T) {
		t.Run("没配模型与坏指针", func(t *testing.T) {
			h := newHarness(t)
			env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
			h.mustOK(env, code, "workspace", nil)

			// 没选模型：用户该被告知「去哪做」，而不是一句含糊的失败。
			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			if env.Code != pkg.CodeNoProvider {
				t.Fatalf("没选模型应回 %q，实际 %q（msg=%q）", pkg.CodeNoProvider, env.Code, env.Message)
			}
			if !strings.Contains(env.Message, "模型页") {
				t.Fatalf("错误信息该指明去模型页，实际 %q", env.Message)
			}

			// 活跃指针指向已删模型：先造一个再删掉，模拟用户配置过期。
			if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
				ProviderName: "gone", BaseUrl: "https://x.example/v1", ApiKey: "k",
				Models: []domain.ProviderModelEntryVO{{ModelId: "m"}},
			}); err != nil {
				t.Fatalf("建服务商失败：%v", err)
			}
			if _, err := h.app.Models.Delete("gone"); err != nil {
				t.Fatalf("删服务商失败：%v", err)
			}
			if err := h.store.Save(map[string]string{
				service.KeyActiveProvider: "gone", service.KeyActiveModel: "m",
			}); err != nil {
				t.Fatalf("预置坏指针失败：%v", err)
			}
			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			if env.Code != pkg.CodeNoProvider {
				t.Fatalf("坏指针应回 %q，实际 %q（msg=%q）", pkg.CodeNoProvider, env.Code, env.Message)
			}
			if got, _ := h.store.Get(service.KeyActiveProvider); got != "" {
				t.Fatalf("坏指针应在报错时被清掉，实际 %q", got)
			}
		})

		t.Run("上游连不通", func(t *testing.T) {
			h := newHarness(t)
			cap := &frameCapture{}
			cancel, _ := h.app.Hub.Subscribe(0, cap.sink)
			defer cancel()
			env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
			h.mustOK(env, code, "workspace", nil)

			// 一个立刻关闭的端口：连接必被拒，是「上游死了」的确定性假上游。
			dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			dead.Close()
			if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
				ProviderName: "dead", BaseUrl: dead.URL, ApiKey: "k",
				Models: []domain.ProviderModelEntryVO{{ModelId: "m", ContextWindow: 100000}},
			}); err != nil {
				t.Fatalf("建服务商失败：%v", err)
			}
			if err := h.app.Chat.SetModel("", "dead", "m"); err != nil {
				t.Fatalf("选模型失败：%v", err)
			}

			env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "你好"})
			h.mustOK(env, code, "chat/send", nil)
			var resp domain.ChatSendRESP
			if err := json.Unmarshal(env.Data, &resp); err != nil {
				t.Fatalf("解析发送出参失败：%v", err)
			}
			if resp.Disposition != domain.DispositionStarted || resp.RunId == "" {
				t.Fatalf("死上游的发送该照常受理，实际 %+v", resp)
			}

			// 运行失败走 chat:error；收尾必须在几秒内到达，界面不能停在「正在回答」。
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if !h.app.Chat.Busy() {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if h.app.Chat.Busy() {
				t.Fatal("上游连不通时运行应在几秒内收尾，实际 3 秒后仍在跑")
			}
			// 错误必须真的广播出去，且带可读原因：只有收尾没有 chat:error，
			// 界面就只知道「停了」，不知道「为什么停」。
			errFrames := cap.of("chat:error")
			if len(errFrames) == 0 {
				t.Fatal("上游连不通应广播 chat:error")
			}
			message, _ := errFrames[0].Data["message"].(string)
			if strings.TrimSpace(message) == "" {
				t.Fatalf("chat:error 应带可读原因，实际 %v", errFrames[0].Data)
			}
		})
	})

	// 提问卡链路：模型调 ask 工具 → 引擎阻塞等待 → 广播 chat:ask →
	// 界面经 /chat/answer 应答 → 那一轮继续往下走。任何一环断了都是「模型卡住不动」。
	t.Run("提问卡链路", func(t *testing.T) {
		t.Run("提问广播与应答", func(t *testing.T) {
			h := newHarness(t)
			cap := &frameCapture{}
			cancel, _ := h.app.Hub.Subscribe(0, cap.sink)
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

		t.Run("待答清单与广播一致", func(t *testing.T) {
			h := newHarness(t)
			cap := &frameCapture{}
			cancel, _ := h.app.Hub.Subscribe(0, cap.sink)
			defer cancel()
			script := askScriptServer(t)
			bindWorkspaceAndModel(t, h, script.server.URL)

			env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "帮我做件事"})
			h.mustOK(env, code, "send", nil)
			askFrames := cap.waitFrames(t, "chat:ask", 1)
			askID, _ := askFrames[0].Data["ask_id"].(string)

			// 待答清单必须与广播那一帧逐字段一致：它是「刷新后补回卡片」唯一的数据来源，
			// 路由把它导出到 /chat/state 之前，这条先把清单本身钉住。
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
	})
}
