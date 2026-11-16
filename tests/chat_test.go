package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			script := newScriptServer(t, textReplyChunks("收到"))
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
			script := newScriptServer(t, textReplyChunks("好"))
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
		script := newScriptServer(t, textReplyChunks("好"))
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
			script := newScriptServer(t, textReplyChunks("好"))
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
			script := newScriptServer(t, textReplyChunks("好"))
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
		script := newScriptServer(t, textReplyChunks("慢一点"))
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

func TestChatContext(t *testing.T) {
	// 上下文读数：首屏总装的读数与计数、水位提示的触发条件。
	t.Run("上下文读数", func(t *testing.T) {
		t.Run("首屏读数与计数", func(t *testing.T) {
			h := newHarness(t)
			script := newScriptServer(t, textReplyChunks("回答"))
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
			script := newSummaryServer(t, textReplyChunks("好的"))
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
