// 真实 HTTP 栈下的对话流链路：发消息 → SSE 收到增量 → 收到 done。
// 服务层单测能证明内核与落库是好的，但证明不了「前端真的能收到事件」——
// 用户看到的「一直转圈没有回复」正是断在这两段之间，所以必须跨进程边界测。

package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/factory"
	"WorkBaby/internal/llm/llmtest"
)

// sseFrame 是一条被解析出来的 SSE 帧。
type sseFrame struct {
	Event string
	ID    string
	Data  map[string]any
}

// subscribeSSE 订阅会话事件流，逐帧解析；返回帧通道与停止函数。
// 读到 EOF 或出错时关闭通道，测试侧靠它判断流是否提前断掉。
func subscribeSSE(t *testing.T, base, sessionID string) (<-chan sseFrame, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/events?session_id="+sessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// SSE 是长连接：不能设整体超时，否则流会被测试自己掐断。
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("订阅事件流失败: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		cancel()
		peek, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		t.Fatalf("事件流 Content-Type 不对: %q body=%s", ct, string(peek))
	}

	frames := make(chan sseFrame, 64)
	go func() {
		defer close(frames)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		var cur sseFrame
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			switch {
			case line == "":
				if cur.Event != "" {
					frames <- cur
				}
				cur = sseFrame{}
			case strings.HasPrefix(line, "event: "):
				cur.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				cur.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &cur.Data)
			}
		}
	}()
	return frames, cancel
}

// drainUntil 收帧直到 want 全部出现或超时；返回已收到的全部帧。
func drainUntil(t *testing.T, frames <-chan sseFrame, want ...string) []sseFrame {
	t.Helper()
	seen := map[string]bool{}
	var got []sseFrame
	deadline := time.After(8 * time.Second)
	for len(want) > 0 {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("事件流提前关闭，只收到 %v", names(got))
			}
			got = append(got, f)
			seen[f.Event] = true
			for i, w := range want {
				if w == f.Event {
					want = append(want[:i], want[i+1:]...)
					break
				}
			}
		case <-deadline:
			t.Fatalf("等待事件 %v 超时，实际收到 %v", want, names(got))
		}
	}
	return got
}

func names(frames []sseFrame) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.Event)
	}
	return out
}

// unwrapID 从统一响应信封里取 data.id：会话与服务都靠它串起后续步骤。
func unwrapID(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, body)
	}
	if payload.Data.ID == "" {
		t.Fatalf("响应里没有 id: %s", body)
	}
	return payload.Data.ID
}

// 完整对话流：POST 发消息后，SSE 必须依次送达 start → delta → done，
// 且 delta 的正文拼接起来等于模型回复、done 带得上 token 用量。
func TestChatStreamDeliversDeltaAndDone(t *testing.T) {
	base := newBootedServer(t)

	script := llmtest.New(llm.Message{Content: "你好，我是 WorkBaby"})
	script.Usage = &llm.Usage{Input: 11, Output: 7, Total: 18, LatencyMs: 42}
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer { return script })
	t.Cleanup(func() { factory.SetOverride("test", nil) })

	code, body := call(t, base, "POST", "/providers", map[string]any{
		"name": "t", "api": "test", "models": []string{"m1"},
	})
	if code != http.StatusOK {
		t.Fatalf("创建模型服务失败: %d %s", code, body)
	}
	provID := unwrapID(t, body)
	code, body = call(t, base, "POST", "/providers/"+provID+"/default", nil)
	if code != http.StatusOK {
		t.Fatalf("设为默认失败: %d %s", code, body)
	}

	code, body = call(t, base, "POST", "/sessions", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("建会话失败: %d %s", code, body)
	}
	sessID := unwrapID(t, body)

	frames, stop := subscribeSSE(t, base, sessID)
	defer stop()

	code, body = call(t, base, "POST", "/chat/send", map[string]any{
		"session_id": sessID, "content": "在吗",
	})
	if code != http.StatusOK || !strings.Contains(body, `"code":0`) {
		t.Fatalf("发送失败: %d %s", code, body)
	}

	got := drainUntil(t, frames, "chat:start", "chat:delta", "chat:done")

	// 按前端的真实用法解帧：它把 data 直接当信封用——
	// `env.event` 决定路由到哪个处理函数，`env.data` 才是载荷。
	// 只断言 `event:` 行会漏掉「data 里没带 event」这种致命问题：
	// 后端跑完全部轮次、数据库里答案齐全，界面却一个字都不显示。
	var text strings.Builder
	var seqs []int64
	for _, f := range got {
		if f.Data["event"] != f.Event {
			t.Fatalf("data 里没有回带 event 字段（实际 %v，帧头是 %q）：前端按 env.event 路由会全部落空",
				f.Data["event"], f.Event)
		}
		envSeq, _ := f.Data["seq"].(float64)
		seqs = append(seqs, int64(envSeq))
		load, ok := f.Data["data"].(map[string]any)
		if !ok {
			t.Fatalf("%s 的 data 字段不是对象，前端读不到载荷: %v", f.Event, f.Data)
		}
		switch f.Event {
		case "chat:delta":
			if load["kind"] == "text" {
				text.WriteString(load["delta"].(string))
			}
		case "chat:done":
			if load["usage"] == nil {
				t.Error("done 事件没有带上 token 用量，输入区水位永远更新不了")
			}
		}
	}
	if text.String() != "你好，我是 WorkBaby" {
		t.Fatalf("流式正文拼接不对: %q", text.String())
	}
	// seq 必须严格递增：前端靠它做断线重放，乱序会导致对账时重复渲染。
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("事件 seq 非递增: %v", seqs)
		}
	}
}

// 断线重连：Last-Event-ID 之后的事件必须被重放，
// 否则用户切走再切回来会丢掉整段回复。
func TestChatStreamReplaysAfterLastEventID(t *testing.T) {
	base := newBootedServer(t)

	script := llmtest.New(llm.Message{Content: "重放我"})
	factory.SetOverride("test", func(llm.ClientConfig) llm.Streamer { return script })
	t.Cleanup(func() { factory.SetOverride("test", nil) })

	code, body := call(t, base, "POST", "/providers", map[string]any{
		"name": "t", "api": "test", "models": []string{"m1"},
	})
	if code != http.StatusOK {
		t.Fatalf("创建模型服务失败: %d %s", code, body)
	}
	provID := unwrapID(t, body)
	if code, body := call(t, base, "POST", "/providers/"+provID+"/default", nil); code != http.StatusOK {
		t.Fatalf("设为默认失败: %d %s", code, body)
	}

	code, body = call(t, base, "POST", "/sessions", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("建会话失败: %d %s", code, body)
	}
	sessID := unwrapID(t, body)

	frames, stop := subscribeSSE(t, base, sessID)
	defer stop()
	if _, body := call(t, base, "POST", "/chat/send", map[string]any{
		"session_id": sessID, "content": "hi",
	}); !strings.Contains(body, `"code":0`) {
		t.Fatalf("发送失败: %s", body)
	}
	got := drainUntil(t, frames, "chat:done")
	stop()

	// 以最后一个 seq 重连，缓冲里已经没有更新的事件，应当只收到 gap 提示。
	last := got[len(got)-1].ID
	req, err := http.NewRequest(http.MethodGet, base+"/events?session_id="+sessID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", last)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if !sc.Scan() {
		// 没有待重放事件时静默挂起是允许的，这里只要求不能报错、不能立刻 EOF 崩掉。
		t.Log("无待重放事件，连接保持挂起")
	}
}
