// 引擎守护测试：四项已落地修复的回归防线。
// S1 工具分片只补空字段（provider.go startCall）；S5 length 截断整批拒绝执行（loop.go failTruncatedCalls）；
// S2 工具全文落盘、折叠只发生在上下文、裁窗后的孤儿配对修复（loop.go / loop_context.go）；
// S6 上游失败白名单重试与成对退避事件（loop_retry.go）。
// 全部走真 HTTP + 真 SSE 分帧的 Runner.Run 全链路：私有函数不直接触碰，
// 断言只落在「会话落盘条目」与「假上游收到的请求体」这两个可观测面上。
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
)

// retryRecorder 记录退避事件的顺序日志：OnRetry/OnRetryEnd 必须成对出现，
// 顺序错了（先 end 后 retry、或漏掉 end）界面的重试进度就会卡死或闪烁。
type retryRecorder struct {
	mu  sync.Mutex
	log []string
}

// onRetry 记一次退避开始。
func (r *retryRecorder) onRetry(attempt, max int, delayMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, fmt.Sprintf("retry %d/%d %dms", attempt, max, delayMs))
}

// onRetryEnd 记一次退避结束。
func (r *retryRecorder) onRetryEnd() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, "end")
}

// events 取顺序日志副本。
func (r *retryRecorder) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.log...)
}

// guardUser 造一条纯文本用户消息。
func guardUser(text string) agent.Message {
	return agent.Message{Role: agent.RoleUser, Blocks: []agent.Block{{Kind: agent.KindText, Text: text}}}
}

// guardAssistantCalls 从会话落盘条目里取第一条助手消息的工具调用块。
// 读文件而不是读内存：会话是唯一真相，拼装结果必须在回放侧也成立。
func guardAssistantCalls(t *testing.T, session *agent.Session) []agent.Block {
	t.Helper()
	entries, err := agent.ReadEntries(session.File())
	if err != nil {
		t.Fatalf("读会话失败：%v", err)
	}
	for i := range entries {
		if entries[i].EntryMessage != nil && entries[i].EntryMessage.MessageRole == string(agent.RoleAssistant) {
			msg := agent.MessageFromDO(entries[i].EntryMessage)
			return msg.ToolCalls()
		}
	}
	t.Fatalf("会话里没有助手消息：%d 条", len(entries))
	return nil
}

// guardToolEntries 取会话落盘条目里全部 role=tool 的消息条目。
func guardToolEntries(t *testing.T, session *agent.Session) []domain.SessionEntryDO {
	t.Helper()
	entries, err := agent.ReadEntries(session.File())
	if err != nil {
		t.Fatalf("读会话失败：%v", err)
	}
	out := make([]domain.SessionEntryDO, 0, len(entries))
	for _, entry := range entries {
		if entry.EntryMessage != nil && entry.EntryMessage.MessageRole == string(agent.RoleTool) {
			out = append(out, entry)
		}
	}
	return out
}

// guardWireBody 是上游请求体里测试关心的那一小撮字段。
type guardWireBody struct {
	Messages []guardWireMsg `json:"messages"`
}

// guardWireMsg 是请求体里的一条消息。
type guardWireMsg struct {
	Role       string          `json:"role"`
	ToolCallId string          `json:"tool_call_id"`
	ToolCalls  []guardWireCall `json:"tool_calls"`
}

// guardWireCall 是请求体里的一次工具调用。
type guardWireCall struct {
	Id string `json:"id"`
}

// guardDecodeBody 把假上游收到的请求体解成可断言的形状。
func guardDecodeBody(t *testing.T, body string) guardWireBody {
	t.Helper()
	var parsed guardWireBody
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("上游请求体不是合法 JSON：%v", err)
	}
	return parsed
}

func TestEngineGuard(t *testing.T) {
	// S1：同 index 的分片再次带 id/name（非空）时只补空缺，不重建块——
	// 重建会把已拼进去的入参前缀整段清掉，拼出来的 JSON 缺头少尾。
	t.Run("S1_分片重复头字段不清入参", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_frag","function":{"name":"write","arguments":"{\"path\":\"guard.txt\",\"content\":\"前"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_dup","function":{"name":"edit","arguments":"半段"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"后段\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		// 拼装出的调用照常执行一轮，第二轮必须纯文本收尾：假上游回放同一套
		// 工具脚本的话，模型会一直要工具直到撞上 24 步上限。
		server.followUpChunks = replyChunks("收尾")

		runner, session, _ := newRunner(t, server.server.URL)
		// 注册表换一份独立临时工作区：拼装出的 write 落在测试目录，
		// 不碰会话工作区——这一组只测拼装这一件事。
		runner.Registry = agent.NewRegistry(func() string { return t.TempDir() })
		recorder := &collect{}
		runner.Run(context.Background(), guardUser("拼分片"), recorder.handler())

		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("这一轮该无错收尾：done=%v errors=%v", recorder.done, recorder.errors)
		}
		calls := guardAssistantCalls(t, session)
		if len(calls) != 1 {
			t.Fatalf("三片同 index 该归并成 1 个调用，实际 %d", len(calls))
		}
		// 「只补空缺」不是「后到覆盖」：非空头字段再来一遍也不能改写既有值。
		if calls[0].CallId != "call_frag" || calls[0].ToolName != "write" {
			t.Fatalf("头字段拼错：id=%q name=%q", calls[0].CallId, calls[0].ToolName)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(calls[0].ToolArgs), &parsed); err != nil {
			t.Fatalf("入参前缀被重复头字段清掉了：%q（%v）", calls[0].ToolArgs, err)
		}
		if parsed["path"] != "guard.txt" || parsed["content"] != "前半段后段" {
			t.Fatalf("三段入参该完整拼合，实际 %v", parsed)
		}
	})

	// S1 变体：name 空串的首片只带 id + 入参前缀，name 由最后一片补——
	// 首片登记的块不能因为 name 为空被丢弃，后到的 name 也不能踩掉已拼的入参。
	t.Run("S1_name迟到最后一片参数不丢", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_late","function":{"name":"","arguments":"{\"path\":\"b"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":".txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		server.followUpChunks = replyChunks("收尾")

		runner, session, _ := newRunner(t, server.server.URL)
		// 同上：独立临时工作区，第二轮纯文本收尾，只测拼装。
		runner.Registry = agent.NewRegistry(func() string { return t.TempDir() })
		recorder := &collect{}
		var toolNames []string
		handler := recorder.handler()
		handler.OnToolStart = func(_, name, _ string) { toolNames = append(toolNames, name) }
		runner.Run(context.Background(), guardUser("补名字"), handler)


		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("这一轮该无错收尾：done=%v errors=%v", recorder.done, recorder.errors)
		}
		// name 空串的首片不触发 OnToolStart：执行必须等 name 由最后一片补齐。
		if len(toolNames) != 1 || toolNames[0] != "read" {
			t.Fatalf("该等 name 补齐后执行一次 read，实际 %v", toolNames)
		}
		calls := guardAssistantCalls(t, session)
		if len(calls) != 1 {
			t.Fatalf("name 迟到的调用不能丢：实际 %d 个调用", len(calls))
		}
		if calls[0].CallId != "call_late" || calls[0].ToolName != "read" {
			t.Fatalf("后到的 name 该补进空缺：id=%q name=%q", calls[0].CallId, calls[0].ToolName)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(calls[0].ToolArgs), &parsed); err != nil {
			t.Fatalf("name 补齐时入参被清了：%q（%v）", calls[0].ToolArgs, err)
		}
		if parsed["path"] != "b.txt" {
			t.Fatalf("两片入参该拼成完整路径，实际 %v", parsed)
		}
	})

	// S5：finish_reason=length 时入参可能是半截 JSON，整批拒绝执行——
	// 每个调用落一条 IsError 的 tool 结果（正文说清「截断」），随后照常回到模型收尾。
	t.Run("S5_length截断整批拒绝执行", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_t1","function":{"name":"write","arguments":"{\"path\":\"guard.txt\",\"content\":\"半"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_t2","function":{"name":"write","arguments":"{\"path\":\"other.txt\",\"content\":\"x\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"length"}]}`,
		})
		server.followUpChunks = replyChunks("已重新发过，收尾")

		runner, session, workspace := newRunner(t, server.server.URL)
		recorder := &collect{}
		handler := recorder.handler()
		var toolResults []agent.Result
		handler.OnToolEnd = func(_, _ string, result agent.Result, _ int64) {
			toolResults = append(toolResults, result)
		}
		runner.Run(context.Background(), guardUser("写文件"), handler)

		// 工具没有真的执行：两个目标文件都不该存在（第二个入参完整也整批拒绝）。
		for _, name := range []string{"guard.txt", "other.txt"} {
			if _, err := os.Stat(filepath.Join(workspace, name)); !os.IsNotExist(err) {
				t.Fatalf("截断批次的 write 不该落笔：%s（err=%v）", name, err)
			}
		}
		// 每个调用各落一条 IsError 的结果，正文带「截断」让模型知道该重发。
		toolEntries := guardToolEntries(t, session)
		if len(toolEntries) != 2 {
			t.Fatalf("两个调用该各落一条结果，实际 %d 条", len(toolEntries))
		}
		wantIds := []string{"call_t1", "call_t2"}
		for i, entry := range toolEntries {
			msg := entry.EntryMessage
			if msg.MessageToolCallId != wantIds[i] {
				t.Fatalf("第 %d 条结果该配对 %s，实际 %q", i, wantIds[i], msg.MessageToolCallId)
			}
			if !msg.MessageIsError {
				t.Fatalf("截断拒绝的结果必须 IsError：%+v", msg)
			}
			resultMsg := agent.MessageFromDO(msg)
			text := resultMsg.Text()
			if !strings.Contains(text, "截断") {
				t.Fatalf("结果正文该说清是截断：%q", text)
			}
		}
		if len(toolResults) != 2 {
			t.Fatalf("OnToolEnd 该对每个调用各报一次，实际 %d", len(toolResults))
		}
		for _, result := range toolResults {
			if !result.IsError || !strings.Contains(result.Text, "截断") {
				t.Fatalf("事件里的结果也该是截断错误：%+v", result)
			}
		}
		// 截断不是终局：还会再调一次上游，错误结果喂回去，模型据此收尾。
		bodies := server.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("截断后该再调一次上游收尾，实际 %d 次请求", len(bodies))
		}
		if !strings.Contains(bodies[1], "截断") {
			t.Fatal("截断的错误结果该进第二次请求的上下文")
		}
		if recorder.text.String() != "已重新发过，收尾" {
			t.Fatalf("第二轮该正常收尾，实际正文 %q", recorder.text.String())
		}
		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("这一轮该无错收尾：done=%v errors=%v", recorder.done, recorder.errors)
		}
	})

	// S2a：全文落盘、折叠只发生在组装上下文——落盘条目正文长度等于原文、
	// 含中段标记；第二次上游请求体里是折叠文本（含「已折叠」、中段标记不在）。
	t.Run("S2_落盘全文与上下文折叠分离", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_big","function":{"name":"read","arguments":"{\"path\":\"big.txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		server.followUpChunks = replyChunks("读完了")

		runner, session, workspace := newRunner(t, server.server.URL)
		// 中段塞一个唯一标记：折叠只留头 3000 尾 1000 字，标记必须落在被折掉的区间。
		const middleMarker = "中段唯一标记ZQX"
		head := strings.Repeat("头部内容行\n", 300)
		middle := strings.Repeat("填充", 1500) + middleMarker + strings.Repeat("填充", 1500)
		tail := strings.Repeat("尾部内容行\n", 300)
		writeFile(t, filepath.Join(workspace, "big.txt"), head+middle+"\n"+tail)

		recorder := &collect{}
		runner.Run(context.Background(), guardUser("读大文件"), recorder.handler())

		toolEntries := guardToolEntries(t, session)
		if len(toolEntries) != 1 {
			t.Fatalf("该落一条工具结果，实际 %d 条", len(toolEntries))
		}
		entry := toolEntries[0].EntryMessage
		if entry.MessageDetails["tidied"] != true {
			t.Fatalf("折叠事实该随结果落盘：%v", entry.MessageDetails)
		}
		originalChars, _ := entry.MessageDetails["original_chars"].(float64)
		if originalChars <= 4000 {
			t.Fatalf("原文该超折叠阈值 4000 字：%v", originalChars)
		}
		// 落盘正文是全文：长度等于折叠前字数，中段标记与头尾都在，且没有折叠话术。
		entryMsg := agent.MessageFromDO(entry)
		text := entryMsg.Text()
		if got := len([]rune(text)); got != int(originalChars) {
			t.Fatalf("落盘正文长度该等于原文 %d，实际 %d（落盘就折叠了）", int(originalChars), got)
		}
		if !strings.Contains(text, middleMarker) {
			t.Fatal("落盘全文该含中段标记：全文永久丢失就是这里出的错")
		}
		if !strings.Contains(text, "头部内容行") || !strings.Contains(text, "尾部内容行") {
			t.Fatal("落盘全文头尾都该在")
		}
		if strings.Contains(text, "已折叠") {
			t.Fatalf("落盘正文不该是折叠文本：%d 字", len([]rune(text)))
		}
		// 直接验落盘块形态：text 块里存的就是全文，不靠 MessageFromDO 的转译。
		blocks, ok := entry.MessageContent.([]any)
		if !ok || len(blocks) == 0 {
			t.Fatalf("工具结果该以内容块数组落盘：%T", entry.MessageContent)
		}
		block, _ := blocks[0].(map[string]any)
		if block["type"] != string(domain.SessionContentText) {
			t.Fatalf("首块该是 text 块：%v", block)
		}
		if raw, _ := block["text"].(string); !strings.Contains(raw, middleMarker) {
			t.Fatal("落盘 text 块本身该含中段全文")
		}

		// 喂回模型的那份是折叠文本：有「已折叠」、头尾还在、中段标记不给模型。
		bodies := server.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("该发 2 次上游请求，实际 %d 次", len(bodies))
		}
		fed := bodies[1]
		if !strings.Contains(fed, "已折叠") {
			t.Fatal("超长工具输出该折叠后再进上下文")
		}
		if strings.Contains(fed, middleMarker) {
			t.Fatal("中段内容不该进上下文：折叠只发生在落盘之外")
		}
		if !strings.Contains(fed, "头部内容行") || !strings.Contains(fed, "尾部内容行") {
			t.Fatal("折叠后头尾内容还得在")
		}
		if recorder.text.String() != "读完了" || !recorder.done {
			t.Fatalf("这一轮该正常收尾：%q done=%v", recorder.text.String(), recorder.done)
		}
	})

	// S2b：裁窗留下的缺口必须修平——没有宿主的孤儿 tool 消息丢掉，
	// 悬空 tool_call 补一条 IsError 的合成结果；严格端点才不会整条请求拒掉。
	t.Run("S2_裁窗后孤儿配对修复", func(t *testing.T) {
		server := newScriptServer(t, replyChunks("收尾"))
		runner, session, _ := newRunner(t, server.server.URL)

		// 造缺口：早段超长发言（必被裁掉）、悬空调用的 assistant、两条孤儿 tool。
		if _, err := session.AppendMessage(guardUser("早段填充标记" + strings.Repeat("旧", 900))); err != nil {
			t.Fatalf("写会话失败：%v", err)
		}
		if _, err := session.AppendMessage(agent.Message{
			Role: agent.RoleAssistant,
			Blocks: []agent.Block{
				{Kind: agent.KindText, Text: "要个工具"},
				{Kind: agent.KindToolCall, CallId: "call_big", ToolName: "read", ToolArgs: `{"path":"x.txt"}`},
			},
		}); err != nil {
			t.Fatalf("写会话失败：%v", err)
		}
		if _, err := session.AppendMessage(agent.Message{
			Role: agent.RoleTool, ToolCallId: "call_small", ToolName: "read",
			Blocks: []agent.Block{{Kind: agent.KindText, Text: "小结果"}},
		}); err != nil {
			t.Fatalf("写会话失败：%v", err)
		}
		if _, err := session.AppendMessage(agent.Message{
			Role: agent.RoleTool, ToolCallId: "call_ghost", ToolName: "read",
			Blocks: []agent.Block{{Kind: agent.KindText, Text: "孤儿结果"}},
		}); err != nil {
			t.Fatalf("写会话失败：%v", err)
		}

		// 精确控制预算：budget = window - len(system)/3 - 1024 = 100 token。
		// 早段发言约 300+ token 必被裁；其余合计约 45 token，assistant 必保留。
		systemChars := len(agent.SystemPrompt(runner.Registry, session.Cwd()))
		runner.Model.ContextWindow = 1024 + systemChars/3 + 100

		recorder := &collect{}
		runner.Run(context.Background(), guardUser("现在说最后一句话"), recorder.handler())

		bodies := server.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("纯文本收尾该只有 1 次请求，实际 %d", len(bodies))
		}
		body := bodies[0]
		if strings.Contains(body, "早段填充标记") {
			t.Fatal("小窗口该把早段超长发言裁掉：trimToWindow 没生效")
		}
		if !strings.Contains(body, "现在说最后一句话") {
			t.Fatal("最后一条用户消息永远不能裁")
		}
		if strings.Contains(body, "call_ghost") || strings.Contains(body, "call_small") {
			t.Fatalf("没有宿主的孤儿 tool 消息必须丢掉：%s", body)
		}

		parsed := guardDecodeBody(t, body)
		hosted := map[string]bool{}
		answered := map[string]bool{}
		for _, msg := range parsed.Messages {
			for _, call := range msg.ToolCalls {
				hosted[call.Id] = true
			}
			if msg.Role == "tool" {
				answered[msg.ToolCallId] = true
			}
		}
		for i, msg := range parsed.Messages {
			if msg.Role == "tool" && !hosted[msg.ToolCallId] {
				t.Fatalf("第 %d 条 tool 消息没有宿主 assistant：%q", i, msg.ToolCallId)
			}
		}
		for id := range hosted {
			if !answered[id] {
				t.Fatalf("tool_call %q 没有配对结果：严格端点会整条拒掉", id)
			}
		}
		// 悬空调用补的是紧随其后的合成错误结果，模型据此知道要重发。
		if !hosted["call_big"] {
			t.Fatal("预算内的 assistant 不该被裁：场景失去了悬空调用")
		}
		if !strings.Contains(body, "这次调用没有留下结果") {
			t.Fatal("悬空 tool_call 该补一条合成错误结果")
		}
		for i, msg := range parsed.Messages {
			for _, call := range msg.ToolCalls {
				if call.Id != "call_big" {
					continue
				}
				if i+1 >= len(parsed.Messages) || parsed.Messages[i+1].Role != "tool" || parsed.Messages[i+1].ToolCallId != "call_big" {
					t.Fatalf("合成结果该紧跟宿主 assistant：%+v", parsed.Messages)
				}
			}
		}
		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("修平后的请求该正常收尾：done=%v errors=%v", recorder.done, recorder.errors)
		}
	})

	// S6a：建流失败（503）在白名单内，退避后重发并成功；
	// OnRetry/OnRetryEnd 成对、attempt 从 1 数、首档退避 2000ms。只造 1 次失败控制时长。
	t.Run("S6_瞬时失败退避后成功", func(t *testing.T) {
		var mu sync.Mutex
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			requests++
			n := requests
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, "service unavailable")
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range replyChunks("重试成功了") {
				fmt.Fprintf(w, "data: %s\n\n", chunk)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		defer server.Close()

		runner, _, _ := newRunner(t, server.URL)
		recorder := &collect{}
		retries := &retryRecorder{}
		handler := recorder.handler()
		handler.OnRetry = retries.onRetry
		handler.OnRetryEnd = retries.onRetryEnd
		runner.Run(context.Background(), guardUser("你好"), handler)

		mu.Lock()
		gotRequests := requests
		mu.Unlock()
		if gotRequests != 2 {
			t.Fatalf("1 次失败 + 1 次成功该共 2 次请求，实际 %d", gotRequests)
		}
		want := []string{"retry 1/3 2000ms", "end"}
		events := retries.events()
		if len(events) != len(want) || events[0] != want[0] || events[1] != want[1] {
			t.Fatalf("退避事件该成对且 attempt=1、延迟 2000ms：%v", events)
		}
		if recorder.text.String() != "重试成功了" {
			t.Fatalf("重试后该拿到正常回复，实际 %q", recorder.text.String())
		}
		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("重试成功不该报错：done=%v errors=%v", recorder.done, recorder.errors)
		}
	})

	// S6b：401 是一票否决——重发改变不了结果，只烧额度；原话必须透传到失败面。
	t.Run("S6_鉴权失败一票否决不重试", func(t *testing.T) {
		var mu sync.Mutex
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			requests++
			mu.Unlock()
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"unauthorized: invalid api key"}}`)
		}))
		defer server.Close()

		runner, _, _ := newRunner(t, server.URL)
		recorder := &collect{}
		retries := &retryRecorder{}
		handler := recorder.handler()
		handler.OnRetry = retries.onRetry
		handler.OnRetryEnd = retries.onRetryEnd
		runner.Run(context.Background(), guardUser("你好"), handler)

		mu.Lock()
		gotRequests := requests
		mu.Unlock()
		if gotRequests != 1 {
			t.Fatalf("401 不该重试：只该收到 1 次请求，实际 %d", gotRequests)
		}
		if len(retries.events()) != 0 {
			t.Fatalf("401 不该有退避事件：%v", retries.events())
		}
		if len(recorder.errors) != 1 {
			t.Fatalf("该恰有一条失败原话，实际 %v", recorder.errors)
		}
		if !strings.Contains(recorder.errors[0], "401") || !strings.Contains(recorder.errors[0], "invalid api key") {
			t.Fatalf("OnError 该带上游原话，实际 %q", recorder.errors[0])
		}
		if !recorder.done {
			t.Fatal("失败也必须收尾")
		}
	})

	// S6c：200 + error 帧走流内 retryUpstreamError——限流类原话可重试，
	// 第二次正常脚本收尾；错误那步不落盘、不报 OnError。
	t.Run("S6_流内error帧限流重试", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"error":{"message":"rate limit exceeded, try again"}}`,
		})
		server.followUpChunks = replyChunks("限流后恢复了")

		runner, session, _ := newRunner(t, server.server.URL)
		recorder := &collect{}
		retries := &retryRecorder{}
		handler := recorder.handler()
		handler.OnRetry = retries.onRetry
		handler.OnRetryEnd = retries.onRetryEnd
		runner.Run(context.Background(), guardUser("你好"), handler)

		bodies := server.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("error 帧后该重发一次，实际 %d 次请求", len(bodies))
		}
		want := []string{"retry 1/3 2000ms", "end"}
		events := retries.events()
		if len(events) != len(want) || events[0] != want[0] || events[1] != want[1] {
			t.Fatalf("流内重试也该成对退避：%v", events)
		}
		if recorder.text.String() != "限流后恢复了" {
			t.Fatalf("重试后该成功，实际 %q", recorder.text.String())
		}
		if !recorder.done || len(recorder.errors) != 0 {
			t.Fatalf("重试成功不该把 error 帧报给用户：done=%v errors=%v", recorder.done, recorder.errors)
		}
		// 失败那步不落盘：会话里只有用户消息与成功的助手消息。
		entries, err := agent.ReadEntries(session.File())
		if err != nil {
			t.Fatalf("读会话失败：%v", err)
		}
		for _, entry := range entries {
			if entry.EntryMessage != nil && entry.EntryMessage.MessageStopReason == "error" {
				t.Fatalf("被重试救回的失败不该落盘：%+v", entry.EntryMessage)
			}
		}
	})
}