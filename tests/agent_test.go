// 原生引擎：流式分片拼装、工具准入、上下文裁剪。
// 这条链路的价值在「分片怎么被拼回来」与「哪些工具此刻能跑」——
// 两处都是拼错一次就静默出错的地方，所以用真的 HTTP + 真的 SSE 分帧来测。
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
)

// sseServer 是一个假的 OpenAI 兼容端点：按脚本吐分片。
// 用真的 HTTP + 真的 SSE 分帧，而不是塞一个假 Streamer——
// 要测的正是「分片怎么被拼回来」，把解析绕过去等于没测。
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
			c.text.WriteString(delta)
			c.mu.Unlock()
		},
		OnThinking: func(delta string) {
			c.mu.Lock()
			c.thinking.WriteString(delta)
			c.mu.Unlock()
		},
		OnToolStart: func(callId, name string, args string) {
			c.mu.Lock()
			c.tools = append(c.tools, name)
			c.mu.Unlock()
		},
		OnError: func(message string) {
			c.mu.Lock()
			c.errors = append(c.errors, message)
			c.mu.Unlock()
		},
		OnDone: func(aborted bool) {
			c.mu.Lock()
			c.done = true
			c.aborted = aborted
			c.mu.Unlock()
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

// TestEngineAssemblesStreamFragments 断言引擎最核心的一条链路：
// 正文分片拼成完整回复，工具入参分片拼成合法 JSON，两者不互相踩掉；
// 落盘的那份与内存里看到的一致（会话是唯一真相）。
func TestEngineAssemblesStreamFragments(t *testing.T) {
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
}

// TestEngineToolBoundary 断言工具的作用域边界：路径越界读不到、文件工具的往返正确。
// 越界是安全边界——模型的路径来自对话内容，不能成为逃出工作区的通道。
func TestEngineToolBoundary(t *testing.T) {
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
}
