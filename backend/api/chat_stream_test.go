// 对话流链路：发消息 → SSE 送达 → 断线重放。
// 跨进程边界验证「前端真的能收到事件」——内核与落库都没问题时，界面仍可能一直转圈。
package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/factory"
	"WorkBaby/backend/llm/llmtest"
)

// sseFrame 是一条被解析出来的 SSE 帧。
type sseFrame struct {
	Event string
	ID    string
	Data  map[string]any
}

// subscribeSSE 订阅会话事件流，逐帧解析；lastID 非空时带 Last-Event-ID 重连。
// 读到 EOF 或出错时关闭通道，测试侧靠它判断流是否提前断掉。
func subscribeSSE(t *testing.T, base, sessionID, lastID string) (<-chan sseFrame, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/events?session_id="+sessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}

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

// 对话流链路：发消息 → SSE 送达 → 断线重放，跨进程边界验证「前端真的能收到事件」。
func TestChatStreamChain(t *testing.T) {
	base := newBootedServer(t)

	// 假实现必须早于建服务注入：Upsert 只在 factory 有 override 时才放行
	// 「test」这种非内置类型，晚一步整条链路会在第一步被拒。
	var script llm.Streamer
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

	// POST 发消息后，SSE 必须依次送达 start → delta → done，
	// 且 delta 的正文拼接起来等于模型回复、done 带得上 token 用量。
	t.Run("SSE 送达 start delta done", func(t *testing.T) {
		s := llmtest.New(llm.Message{Content: "你好，我是 WorkBaby"})
		s.Usage = &llm.Usage{Input: 11, Output: 7, Total: 18, LatencyMs: 42}
		script = s

		frames, stop := subscribeSSE(t, base, sessID, "")
		defer stop()

		code, body := call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sessID, "content": "在吗",
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":0`) {
			t.Fatalf("发送失败: %d %s", code, body)
		}

		got := drainUntil(t, frames, "chat:start", "chat:delta", "chat:done")

		// 前端把 data 当信封用：data.event 决定路由，data.data 才是载荷。
		// 只断言 `event:` 行会漏掉「data 里没回带 event」，界面会一个字都不显示。
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
	})

	// 断线重连：带 Last-Event-ID 回来必须补上缺的那几帧，否则用户切走一次
	// 就永久丢一段回复（界面上正文停在半句，点重连也没反应）。
	// 另起一个会话：同会话的旧事件会一起重放，断言会被上一轮干扰。
	t.Run("按 Last-Event-ID 重放缺失事件", func(t *testing.T) {
		script = llmtest.New(llm.Message{Content: "重放我"})

		code, body := call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		sid := unwrapID(t, body)

		frames, stop := subscribeSSE(t, base, sid, "")
		if _, body := call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sid, "content": "hi",
		}); !strings.Contains(body, `"code":0`) {
			t.Fatalf("发送失败: %s", body)
		}
		got := drainUntil(t, frames, "chat:done")
		stop()

		seqOf := func(f sseFrame) int64 {
			v, _ := f.Data["seq"].(float64)
			return int64(v)
		}
		last := seqOf(got[len(got)-1])
		if last < 3 {
			t.Skipf("本轮只发出 %d 帧，构不出重放窗口", last)
		}

		// 假装断了 2 帧：重连后必须先把这两帧补回来。
		replay, stopReplay := subscribeSSE(t, base, sid, strconv.FormatInt(last-2, 10))
		defer stopReplay()
		again := drainUntil(t, replay, "chat:done")
		if first := seqOf(again[0]); first <= last-2 {
			t.Fatalf("重连后没补上缺失事件：首帧 seq=%d，应 > %d", first, last-2)
		}
	})
}
