// 引擎的流式拼装：正文分片、思考分片、工具入参分片与用量。
// 用真的 HTTP + 真的 SSE 分帧来测，而不是塞一个假 Streamer——
// 要测的正是「分片怎么被拼回来」，把解析绕过去等于没测。
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
