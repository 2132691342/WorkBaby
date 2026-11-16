// 引擎层：轮次循环、分片拼装、工具执行、上下文组装、上游重试、计划模式闸门。
// 这一层拼错一次就静默出错，且只在「上游分片 → 引擎 → 会话落盘」这一段闭合，
// 所以一律用真 HTTP + 真 SSE 分帧来测，而不是塞一个假 Streamer；
// 回归防线（分片补空缺、截断拒绝、落盘全文、孤儿修复、退避重试）也钉在这里。
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
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/service/parser"
)

// retryRecorder 记录退避事件的顺序日志：OnRetry/OnRetryEnd 必须成对出现，
// 顺序错了界面的重试进度就会卡死或闪烁。
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
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"reasoning_content":"先想想"}}]}`,
			`{"choices":[{"delta":{"content":"我来看看"}}]}`,
			`{"choices":[{"delta":{"content":"目录。"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ls","arguments":"{\"path\""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\".\"}"}}]}}]}`,
			`{"choices":[{"delta":{"content":"  还有话"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7}}`,
		})

		runner, session, _ := newRunner(t, server.server.URL)
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

	// 上游 SSE 的 error 帧必须把原话带到失败面：丢掉它，用户只看到「失败了」，
	// 日志里也是空的，排查从零开始。
	t.Run("上游错误原话透传", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"error":{"message":"账户余额不足，请先充值再试"}}`,
		})
		runner, _, _ := newRunner(t, server.server.URL)
		recorder := &collect{}
		runner.Run(context.Background(), agent.Message{
			Role:   agent.RoleUser,
			Blocks: []agent.Block{{Kind: agent.KindText, Text: "你好"}},
		}, recorder.handler())

		if !recorder.done {
			t.Fatal("上游报错后运行也必须收尾")
		}
		if len(recorder.errors) == 0 {
			t.Fatal("上游报错应触发 OnError")
		}
		joined := strings.Join(recorder.errors, " | ")
		if !strings.Contains(joined, "账户余额不足") {
			t.Fatalf("错误原话该透传到失败面，实际 %q", joined)
		}
	})

	// 工具入参容错：schema 声明的一切参数都是 string，模型照办发 "20"，
	// 只认 float64 的话它拿到的永远是缺省值且没有任何提示。
	t.Run("工具入参容错", func(t *testing.T) {
		workspace := t.TempDir()
		writeFile(t, filepath.Join(workspace, "a.txt"), "第一行\n")
		registry := agent.NewRegistry(func() string { return workspace })
		ctx := context.Background()

		// 数字字符串按数字用：limit 传 "1" 不该回缺省 8 条。
		if out := registry.Run(ctx, "read", `{"path":"a.txt","start":"1","end":"1"}`); out.IsError {
			t.Fatalf("数字字符串形态的行号应被接受：%s", out.Text)
		}
		// 布尔字符串同理：ignore_case="true" 要真的开。
		writeFile(t, filepath.Join(workspace, "b.txt"), "Needle\n")
		upper := registry.Run(ctx, "grep", `{"pattern":"needle","ignore_case":"true"}`)
		if upper.IsError || !strings.Contains(upper.Text, "b.txt") {
			t.Fatalf("布尔字符串应被当成 true：%s", upper.Text)
		}
		lower := registry.Run(ctx, "grep", `{"pattern":"NEEDLE","ignore_case":"false"}`)
		if lower.IsError {
			t.Fatalf("布尔字符串 false 不该让工具失败：%s", lower.Text)
		}
		if strings.Contains(lower.Text, "b.txt") {
			t.Fatalf("ignore_case=false 时不该命中大小写不同的内容：%s", lower.Text)
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

func TestEngineGuard(t *testing.T) {
	// 同 index 的分片再次带 id/name（非空）时只补空缺，不重建块：
	// 重建会把已拼进去的入参前缀整段清掉，拼出来的 JSON 缺头少尾。
	t.Run("分片重复头字段不清入参", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_frag","function":{"name":"write","arguments":"{\"path\":\"guard.txt\",\"content\":\"前"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_dup","function":{"name":"edit","arguments":"半段"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"后段\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		// 拼装出的调用照常执行一轮，第二轮必须纯文本收尾：假上游回放同一套
		// 工具脚本的话，模型会一直要工具直到撞上 24 步上限。
		server.followUpChunks = textReplyChunks("收尾")

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

	// 变体：name 空串的首片只带 id + 入参前缀，name 由最后一片补：
	// 首片登记的块不能因为 name 为空被丢弃，后到的 name 也不能踩掉已拼的入参。
	t.Run("name迟到最后一片参数不丢", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_late","function":{"name":"","arguments":"{\"path\":\"b"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":".txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		server.followUpChunks = textReplyChunks("收尾")

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

	// finish_reason=length 时入参可能是半截 JSON，整批拒绝执行：
	// 每个调用落一条 IsError 的 tool 结果（正文说清「截断」），随后照常回到模型收尾。
	t.Run("length截断整批拒绝执行", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_t1","function":{"name":"write","arguments":"{\"path\":\"guard.txt\",\"content\":\"半"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_t2","function":{"name":"write","arguments":"{\"path\":\"other.txt\",\"content\":\"x\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"length"}]}`,
		})
		server.followUpChunks = textReplyChunks("已重新发过，收尾")

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

	// 全文落盘、折叠只发生在组装上下文：落盘条目正文长度等于原文、
	// 含中段标记；第二次上游请求体里是折叠文本（含「已折叠」、中段标记不在）。
	t.Run("落盘全文与上下文折叠分离", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_big","function":{"name":"read","arguments":"{\"path\":\"big.txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		})
		server.followUpChunks = textReplyChunks("读完了")

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

	// 裁窗留下的缺口必须修平：没有宿主的孤儿 tool 消息丢掉，
	// 悬空 tool_call 补一条 IsError 的合成结果；严格端点才不会整条请求拒掉。
	t.Run("裁窗后孤儿配对修复", func(t *testing.T) {
		server := newScriptServer(t, textReplyChunks("收尾"))
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

	// 建流失败（503）在白名单内，退避后重发并成功；
	// OnRetry/OnRetryEnd 成对、attempt 从 1 数、首档退避 2000ms。只造 1 次失败控制时长。
	t.Run("瞬时失败退避后成功", func(t *testing.T) {
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
			for _, chunk := range textReplyChunks("重试成功了") {
				fmt.Fprintf(w, "data: %s\n\n", chunk)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		defer server.Close()

		runner, _, _ := newRunner(t, server.URL)
		runner.RetryBaseDelay = 10 * time.Millisecond
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
		want := []string{"retry 1/3 10ms", "end"}
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

	// 401 是一票否决：重发改变不了结果，只烧额度；原话必须透传到失败面。
	t.Run("鉴权失败一票否决不重试", func(t *testing.T) {
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

	// 200 + error 帧走流内 retryUpstreamError：限流类原话可重试，
	// 第二次正常脚本收尾；错误那步不落盘、不报 OnError。
	t.Run("流内error帧限流重试", func(t *testing.T) {
		server := newScriptServer(t, []string{
			`{"error":{"message":"rate limit exceeded, try again"}}`,
		})
		server.followUpChunks = textReplyChunks("限流后恢复了")

		runner, session, _ := newRunner(t, server.server.URL)
		recorder := &collect{}
		runner.RetryBaseDelay = 10 * time.Millisecond
		retries := &retryRecorder{}
		handler := recorder.handler()
		handler.OnRetry = retries.onRetry
		handler.OnRetryEnd = retries.onRetryEnd
		runner.Run(context.Background(), guardUser("你好"), handler)

		bodies := server.requestBodies()
		if len(bodies) != 2 {
			t.Fatalf("error 帧后该重发一次，实际 %d 次请求", len(bodies))
		}
		want := []string{"retry 1/3 10ms", "end"}
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
