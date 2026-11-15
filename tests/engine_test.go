// 引擎层：分片拼装、工具轮次闭环、工具作用域、待办往返、计划模式闸门。
// 这一层拼错一次就静默出错，且只在「上游分片 → 引擎 → 会话落盘」这一段闭合，
// 所以一律用真 HTTP + 真 SSE 分帧来测，而不是塞一个假 Streamer。
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/service/parser"
)

// sseServer 是一个按脚本吐分片的 OpenAI 兼容端点。
func sseServer(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

// collect 是测试里的 Handler 记录器。
type collect struct {
	mu       sync.Mutex
	text     strings.Builder
	thinking strings.Builder
	tools    []string
	errors   []string
	done     bool
	aborted  bool
}

// handler 组装一个把事件记进 collect 的 Handler。
func (c *collect) handler() agent.Handler {
	return agent.Handler{
		OnText: func(delta string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.text.WriteString(delta)
		},
		OnThinking: func(delta string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.thinking.WriteString(delta)
		},
		OnToolStart: func(callId, name string, args string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.tools = append(c.tools, name)
		},
		OnError: func(message string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.errors = append(c.errors, message)
		},
		OnDone: func(aborted bool) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.done = true
			c.aborted = aborted
		},
	}
}

// newRunner 造一个绑到临时工作区的运行器。
func newRunner(t *testing.T, baseUrl string) (*agent.Runner, *agent.Session, string) {
	t.Helper()
	workspace := t.TempDir()
	store := agent.NewStore(t.TempDir(), func() string { return workspace })
	session, err := store.Create(workspace)
	if err != nil {
		t.Fatalf("建会话失败：%v", err)
	}
	runner := &agent.Runner{
		Streamer: agent.NewOpenAIStreamer(),
		Registry: agent.NewRegistry(func() string { return workspace }),
		Session:  session,
		Model: agent.ModelRef{
			ProviderName: "test", ModelId: "m", BaseUrl: baseUrl, ApiKey: "k",
		},
	}
	return runner, session, workspace
}

// stubTool 是测试用的占位工具：只关心名字，不关心行为。
type stubTool struct {
	// name 工具名
	name string
}

// Spec 工具声明。
func (t *stubTool) Spec() agent.ToolSpec {
	return agent.ToolSpec{
		Name:        t.name,
		Description: "测试用占位工具",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

// Run 执行：恒成功。
func (t *stubTool) Run(_ context.Context, _ *agent.Registry, _ map[string]any) agent.Result {
	return agent.Result{Text: "ok"}
}

func TestEngine(t *testing.T) {
	// 分片拼装：正文分片拼成完整回复，工具入参分片拼成合法 JSON，两者不互相踩掉；
	// 落盘的那份与内存里看到的一致（会话是唯一真相）。
	t.Run("分片拼装", func(t *testing.T) {
		server := sseServer(t, []string{
			`{"choices":[{"delta":{"reasoning_content":"先想想"}}]}`,
			`{"choices":[{"delta":{"content":"我来看看"}}]}`,
			`{"choices":[{"delta":{"content":"目录。"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ls","arguments":"{\"path\""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\".\"}"}}]}}]}`,
			`{"choices":[{"delta":{"content":"  还有话"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
		})
		defer server.Close()

		runner, session, _ := newRunner(t, server.URL)
		runner.Model.ContextWindow = 0
		recorder := &collect{}
		// 把工具集清空：模型要不到工具就走完一轮，只测分片拼装这一件事。
		runner.Registry = agent.NewRegistry(func() string { return t.TempDir() })
		runner.Run(context.Background(), agent.Message{
			Role:   agent.RoleUser,
			Blocks: []agent.Block{{Kind: agent.KindText, Text: "看看当前目录"}},
		}, recorder.handler())

		text := recorder.text.String()
		// 工具块之前的正文、工具块之后续写的正文，两段都必须保住。
		if !strings.Contains(text, "我来看看") || !strings.Contains(text, "目录。") {
			t.Fatalf("正文分片丢了：%q", text)
		}
		if !strings.Contains(text, "还有话") {
			t.Fatalf("工具调用之后的正文被吞了：%q", text)
		}
		if !strings.Contains(recorder.thinking.String(), "先想想") {
			t.Fatalf("思考分片丢了：%q", recorder.thinking.String())
		}
		if !recorder.done {
			t.Fatal("运行没有收尾")
		}

		// 落盘的是同一份内容：会话是唯一真相，内存与文件不能各说各的。
		entries, err := agent.ReadEntries(session.File())
		if err != nil {
			t.Fatalf("读会话失败：%v", err)
		}
		// 助手消息后面跟着它的工具结果：找第一条助手消息，不是找最后一条。
		var assistant *domain.SessionEntryDO
		for i := range entries {
			if entries[i].EntryMessage != nil && entries[i].EntryMessage.MessageRole == string(agent.RoleAssistant) {
				assistant = &entries[i]
				break
			}
		}
		if assistant == nil {
			t.Fatalf("会话里没有助手消息：%d 条", len(entries))
		}
		msg := agent.MessageFromDO(assistant.EntryMessage)
		calls := msg.ToolCalls()
		if len(calls) != 1 {
			t.Fatalf("工具调用应为 1 个，实际 %d", len(calls))
		}
		if calls[0].ToolName != "ls" {
			t.Fatalf("工具名拼错：%q", calls[0].ToolName)
		}
		// 入参分两片到达，拼起来必须是合法 JSON。
		var parsed map[string]any
		if err := json.Unmarshal([]byte(calls[0].ToolArgs), &parsed); err != nil {
			t.Fatalf("工具入参没拼成合法 JSON：%q（%v）", calls[0].ToolArgs, err)
		}
		if parsed["path"] != "." {
			t.Fatalf("工具入参内容不对：%v", parsed)
		}
	})

	// 工具轮次闭环：工具跑过一次后引擎要再调一次模型，工具结果进了下一轮上下文，
	// 模型据此给出最终回复并收尾；超长输出折叠后才进上下文，折叠事实随结果落盘。
	t.Run("工具轮次闭环", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":\"a.txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		server.followUpChunks = []string{
			`{"choices":[{"delta":{"content":"读完了"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}

		runner, session, workspace := newRunner(t, server.server.URL)
		// 远超折叠上限的输出：模型只该拿到头尾与标记，全文不给它。
		writeFile(t, filepath.Join(workspace, "a.txt"), strings.Repeat("内容行\n", 2000))

		recorder := &collect{}
		runner.Run(context.Background(), agent.Message{
			Role:   agent.RoleUser,
			Blocks: []agent.Block{{Kind: agent.KindText, Text: "读文件"}},
		}, recorder.handler())

		// 两次请求：第一次要工具，第二次拿结果说最终话。
		bodies := server.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("该发 2 次上游请求（要工具 + 收结果），实际 %d 次", len(bodies))
		}
		if len(recorder.tools) != 1 || recorder.tools[0] != "read" {
			t.Fatalf("该跑一次 read 工具，实际 %v", recorder.tools)
		}
		if recorder.text.String() != "读完了" {
			t.Fatalf("模型该据工具结果给出最终回复，实际 %q", recorder.text.String())
		}
		if !recorder.done {
			t.Fatal("这一轮该正常收尾")
		}

		// 喂回模型的那一份是折叠后的：有折叠标记、头尾仍在，正文远少于原文。
		fed := bodies[1]
		if !strings.Contains(fed, "已折叠") {
			t.Fatalf("超长工具输出该折叠后再进上下文：%d 字", len([]rune(fed)))
		}
		if !strings.Contains(fed, "内容行") {
			t.Fatalf("折叠后头尾内容还得在：%s", fed[:200])
		}
		// 原文 2000 行「内容行」，折叠只留头 3000 字与尾 1000 字：正文行数是原文的零头。
		if got := strings.Count(fed, "内容行"); got == 0 || got > 1200 {
			t.Fatalf("折叠后喂进去的正文该远少于原文 2000 行，实际 %d 行", got)
		}

		// 工具结果要落进会话：模型下一轮是从会话文件读历史的；
		// 折叠的事实（tidied / original_chars）跟着落盘，回放时界面才知道这段被折过。
		entries, err := agent.ReadEntries(session.File())
		if err != nil {
			t.Fatalf("读会话失败：%v", err)
		}
		var toolReply *domain.SessionEntryDO
		for i := range entries {
			if entries[i].EntryMessage != nil && entries[i].EntryMessage.MessageRole == string(agent.RoleTool) {
				toolReply = &entries[i]
			}
		}
		if toolReply == nil {
			t.Fatal("工具结果该落一条进会话")
		}
		if toolReply.EntryMessage.MessageDetails["tidied"] != true {
			t.Fatalf("折叠事实该随结果落盘，实际 %v", toolReply.EntryMessage.MessageDetails)
		}
		// 折叠阈值是 4000 字：原文字数超阈值才说明真被折过，且这个数要记进台账。
		if chars, _ := toolReply.EntryMessage.MessageDetails["original_chars"].(float64); chars <= 4000 {
			t.Fatalf("原始字数该超折叠阈值并记下来，实际 %v", toolReply.EntryMessage.MessageDetails["original_chars"])
		}
	})

	// 工具作用域：路径越界读不到——模型的路径来自对话内容，
	// 不能成为逃出工作区的通道。
	t.Run("工具作用域", func(t *testing.T) {
		workspace := t.TempDir()
		registry := agent.NewRegistry(func() string { return workspace })
		ctx := context.Background()

		out := registry.Run(ctx, "read", `{"path":"../../../etc/passwd"}`)
		if !out.IsError {
			t.Fatalf("越界读取竟然成功了：%q", out.Text)
		}
		if !strings.Contains(out.Text, "工作区") {
			t.Fatalf("错误信息没有说明越界：%q", out.Text)
		}

		if result := registry.Run(ctx, "write", `{"path":"a.txt","content":"第一行\n第二行\n"}`); result.IsError {
			t.Fatalf("写入失败：%s", result.Text)
		}
		read := registry.Run(ctx, "read", `{"path":"a.txt"}`)
		if read.IsError || !strings.Contains(read.Text, "第二行") {
			t.Fatalf("读取不符合预期：%s", read.Text)
		}
		// 行号必须带上：编辑与差异都靠它锚定。
		if !strings.Contains(read.Text, "2\t第二行") {
			t.Fatalf("读取没有带行号：%s", read.Text)
		}
		edit := registry.Run(ctx, "edit", `{"path":"a.txt","old_text":"第二行","new_text":"改过了"}`)
		if edit.IsError {
			t.Fatalf("编辑失败：%s", edit.Text)
		}
		if diff, ok := edit.Details["diff"].(string); !ok || !strings.Contains(diff, "改过了") {
			t.Fatalf("差异没有回传：%v", edit.Details)
		}
	})

	// 待办往返：清单整份替换、状态与现在进行时保留、全量表随结果落盘，
	// 回放层据此派生界面要的行。
	t.Run("待办往返", func(t *testing.T) {
		workspace := t.TempDir()
		registry := agent.NewRegistry(func() string { return workspace })

		result := registry.Run(context.Background(), "todo", `{"tasks":[
		{"subject":"先把接口写好","status":"in_progress","activeForm":"正在写接口"},
		{"subject":"再写测试","status":"pending"}
	]}`)
		if result.IsError {
			t.Fatalf("待办工具失败：%s", result.Text)
		}
		// 全量表必须跟着结果走：回放层只靠它复原清单。
		raw, ok := result.Details["tasks"].([]any)
		if !ok || len(raw) != 2 {
			t.Fatalf("待办清单没有随结果落盘：%v", result.Details)
		}
		first, ok := raw[0].(map[string]any)
		if !ok {
			t.Fatalf("待办项形状不对：%v", raw[0])
		}
		if first["subject"] != "先把接口写好" {
			t.Fatalf("待办项内容不对：%v", first)
		}
		if first["status"] != "in_progress" {
			t.Fatalf("待办状态不该丢：%v", first)
		}
		if first["activeForm"] != "正在写接口" {
			t.Fatalf("现在进行时不该丢：%v", first)
		}
		// 任务号按顺序编号：模型不认得旧编号，重排比保留更可预期。
		if id, ok := first["id"].(int); !ok || id != 1 {
			t.Fatalf("任务号该从 1 顺排，实际 %v", first["id"])
		}

		// 整份替换：再调一次，旧任务不该还在。
		result = registry.Run(context.Background(), "todo", `{"tasks":[{"subject":"只做这一件","status":"completed"}]}`)
		if result.IsError {
			t.Fatalf("待办工具失败：%s", result.Text)
		}
		raw, _ = result.Details["tasks"].([]any)
		if len(raw) != 1 {
			t.Fatalf("清单该被整份替换，实际 %d 项", len(raw))
		}

		// 清单能跨「落盘 → 回放」闭合：活动分支上最近一次调用就是当前待办。
		store := agent.NewStore(t.TempDir(), func() string { return workspace })
		session, err := store.Create(workspace)
		if err != nil {
			t.Fatalf("建会话失败：%v", err)
		}
		if _, err := session.AppendMessage(agent.Message{
			Role:      agent.RoleTool,
			ToolName:  "todo",
			Blocks:    []agent.Block{{Kind: agent.KindText, Text: result.Text}},
			Details:   result.Details,
			CreatedAt: 1,
		}); err != nil {
			t.Fatalf("写工具结果失败：%v", err)
		}
		todos := parser.TodosFromFile(session.File())
		if len(todos) != 1 {
			t.Fatalf("回放该派生出一条待办，实际 %d 条", len(todos))
		}
		if todos[0].TodoSubject != "只做这一件" {
			t.Fatalf("待办内容不对：%q", todos[0].TodoSubject)
		}
		if todos[0].TodoStatus != "completed" {
			t.Fatalf("待办状态不对：%q", todos[0].TodoStatus)
		}
	})

	// 计划模式闸门：写工具被拦、读工具放行、未登记的工具默认按「会改」处理，
	// 且系统提示写明被拦的是哪些。
	t.Run("计划模式闸门", func(t *testing.T) {
		workspace := t.TempDir()
		registry := agent.NewRegistry(func() string { return workspace })

		// 默认不拦：自由态下写工具照常可用。
		write := registry.Run(context.Background(), "write", `{"path":"a.txt","content":"x"}`)
		if write.IsError {
			t.Fatalf("自由态不该拦写工具：%s", write.Text)
		}

		registry.Policy.SetReadOnly("SES_test", true)

		blocked := registry.Run(context.Background(), "write", `{"path":"b.txt","content":"y"}`)
		if !blocked.IsError {
			t.Fatal("只读态下写工具必须被拦下")
		}
		if !strings.Contains(blocked.Text, "计划模式") {
			t.Fatalf("拦截文案要说清是计划模式：%s", blocked.Text)
		}
		if blocked.Details["blocked_by"] != "plan_mode" {
			t.Fatalf("拦截详情要标出原因：%v", blocked.Details)
		}

		// 读工具不受影响：只读模式拦的是改，不是看。
		read := registry.Run(context.Background(), "read", `{"path":"a.txt"}`)
		if read.IsError {
			t.Fatalf("只读态下读工具不该被拦：%s", read.Text)
		}

		// 未登记的工具名默认按写工具处理：漏放一个写工具，计划模式就名存实亡。
		registry.Add(&stubTool{name: "deploy_prod"})
		if out := registry.Run(context.Background(), "deploy_prod", `{}`); !out.IsError {
			t.Fatal("未登记的工具在只读态下必须被拦下")
		}

		// 模型得知道为什么写工具会报错，否则它只会当成故障反复重试。
		restricted := agent.SystemPrompt(registry, t.TempDir())
		if !strings.Contains(restricted, "计划模式") {
			t.Fatalf("只读态的系统提示要说清计划模式：\n%s", restricted)
		}
		if !strings.Contains(restricted, "write") {
			t.Fatalf("系统提示该点名被拦的工具：\n%s", restricted)
		}

		// 退出计划模式后恢复自由。
		registry.Policy.SetReadOnly("SES_test", false)
		again := registry.Run(context.Background(), "write", `{"path":"c.txt","content":"z"}`)
		if again.IsError {
			t.Fatalf("退出只读态后写工具该能用：%s", again.Text)
		}
	})
}
