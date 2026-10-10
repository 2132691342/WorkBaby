// 跨进程边界的端到端链路：真实 HTTP 栈下的装配完整性、跨源预检、帮助文档、
// SSE 送达与断线重放。内核与落库都没问题时，界面仍可能一直转圈。
package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/llm/factory"
	"WorkBaby/backend/llm/llmtest"
	"WorkBaby/backend/runtime/runtimetest"
	"WorkBaby/backend/server"
)

// bootServer 在干净家目录起一套完整装配 + 真实 HTTP 栈。
// 运行时标记先预置好：配置 / DB / 服务 / 工具注册全链路照常跑，只跳过归档解压。
func bootServer(t *testing.T) (string, *api.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
	runtimetest.SeedMarkers(t, home)

	h := api.New("test")
	if err := h.Startup(context.Background()); err != nil {
		t.Fatalf("首次启动必须成功: %v", err)
	}
	t.Cleanup(h.Shutdown)

	// 装配缺任一样东西，界面都是空窗口 + 接口全 404。
	if h.Svc == nil || h.Repo == nil || h.Registry == nil || h.Skills == nil || h.Cfg.MasterKey == "" {
		t.Fatal("服务容器没有装配完整（Svc/Repo/Registry/Skills/MasterKey），界面会空着且接口全 404")
	}
	for _, name := range []string{"read", "write"} {
		if _, ok := h.Registry.Get(name); !ok {
			t.Fatalf("%s 工具没有注册，模型会突然找不到文件能力", name)
		}
	}
	if h.ServerPort() != 0 {
		t.Fatal("端口应由后续握手阶段写入，Startup 阶段应为 0（早一步广播则前端所有接口 404）")
	}

	srv := server.New(h)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("HTTP 服务启动失败: %v", err)
	}
	t.Cleanup(srv.Stop)
	return fmt.Sprintf("http://127.0.0.1:%d/api/v1", port), h
}

// call 发一个带 WebView2 来源的请求，返回状态码与响应体。
func call(t *testing.T, base, method, path string, body any) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "http://wails.localhost")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "TRANSPORT-ERROR: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// unwrapID 从统一响应信封里取 data.id：会话与 provider 都靠它串起后续步骤。
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

// sseFrame 是一条被解析出来的 SSE 帧。
type sseFrame struct {
	Event string
	ID    string
	Data  map[string]any
}

// subscribeSSE 订阅会话事件流，逐帧解析；lastID 非空时带 Last-Event-ID 重连。
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
	resp, err := (&http.Client{}).Do(req)
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
	var got []sseFrame
	deadline := time.After(8 * time.Second)
	for len(want) > 0 {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("事件流提前关闭，只收到 %v", names(got))
			}
			got = append(got, f)
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

// 从零启动到事件送达的完整链路：装配（bootServer 内断言）→ 预检 → 帮助文档 → SSE 流与重放。
func TestHTTPChain(t *testing.T) {
	base, _ := bootServer(t)

	// Go 的 http 客户端不发预检，只有模拟浏览器才测得出来（症状：打包版所有 POST 报 Network Error）。
	t.Run("跨源预检放行 POST", func(t *testing.T) {
		req, err := http.NewRequest("OPTIONS", base+"/skills", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "http://wails.localhost")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("预检传输失败: %v", err)
		}
		defer resp.Body.Close()
		if acao := resp.Header.Get("Access-Control-Allow-Origin"); resp.StatusCode != http.StatusNoContent || acao != "http://wails.localhost" {
			t.Fatalf("预检被拦：status=%d ACAO=%q（POST 会全部变成 Network Error）", resp.StatusCode, acao)
		}
	})

	t.Run("帮助文档可读且不越界", func(t *testing.T) {
		code, body := call(t, base, "GET", "/docs", nil)
		if code != http.StatusOK || !strings.Contains(body, `"getting-started"`) {
			t.Fatalf("帮助目录读不到: %d %s", code, body)
		}
		var list struct {
			Code int `json:"code"`
			Data []struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			t.Fatalf("帮助目录解析失败: %v", err)
		}
		if len(list.Data) < 2 || list.Data[0].Name != "getting-started" || list.Data[0].Title == "" {
			t.Fatalf("帮助目录不完整（首篇须是快速上手且带标题）: %+v", list.Data)
		}

		code, body = call(t, base, "GET", "/docs/getting-started", nil)
		if code != http.StatusOK || !strings.Contains(body, "快速上手") {
			t.Fatalf("帮助正文读不到: %d %s", code, body)
		}

		// 穿越尝试只要能拿到文档内容就算失守：路由层 404 与业务层错误码都算拦下。
		code, body = call(t, base, "GET", "/docs/..%2Fconfig", nil)
		if code == http.StatusOK && strings.Contains(body, `"code":0`) {
			t.Fatalf("非法文档名没有被拦下: %d %s", code, body)
		}
	})

	// 事件信封的形态（data 里回带 event 与 seq）必须与前端解析约定一致，
	// 只断言 `event:` 行会漏掉界面一个字都不显示的情况。
	t.Run("SSE 送达与断线重放", func(t *testing.T) {
		// 假实现必须早于建服务注入：Upsert 只在 factory 有 override 时才放行「test」这种类型。
		script := llmtest.New(llm.Message{Content: "你好，我是 WorkBaby"})
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

		script = llmtest.New(llm.Message{Content: "你好，我是 WorkBaby"})
		script.Usage = &llm.Usage{Input: 11, Output: 7, Total: 18, LatencyMs: 42}

		code, body = call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		sessID := unwrapID(t, body)

		frames, stop := subscribeSSE(t, base, sessID, "")
		defer stop()

		code, body = call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sessID, "content": "在吗",
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":0`) {
			t.Fatalf("发送失败: %d %s", code, body)
		}
		got := drainUntil(t, frames, "chat:start", "chat:delta", "chat:done")

		// 前端把 data 当信封用：data.event 决定路由，data.data 才是载荷。
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
				// 用量会被服务层按「输入含缓存」归一，所以只断言口径不钉死脚本值。
				usage, ok := load["usage"].(map[string]any)
				total, _ := usage["total"].(float64)
				if !ok || usage["output"] != float64(7) || total <= 0 {
					t.Fatalf("done 事件的用量不对（output 应为 7、total 为正，实际 %v）：输入区水位与仪表盘都靠它", load["usage"])
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

		// 断线重连：带 Last-Event-ID 回来必须补上缺的那几帧，否则切走一次就永久丢一段回复。
		script = llmtest.New(llm.Message{Content: "重放我"})
		code, body = call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		sid := unwrapID(t, body)

		frames2, stop2 := subscribeSSE(t, base, sid, "")
		if _, body := call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sid, "content": "hi",
		}); !strings.Contains(body, `"code":0`) {
			t.Fatalf("发送失败: %s", body)
		}
		got = drainUntil(t, frames2, "chat:done")
		stop2()

		seqOf := func(f sseFrame) int64 {
			v, _ := f.Data["seq"].(float64)
			return int64(v)
		}
		// 帧数不足说明一轮连 start/delta/done 都没凑齐，是链路退化而不是"跳过"：
		// 用 Fatal 而不是 Skip，避免重放这一段在无人知晓的情况下永远不执行。
		last := seqOf(got[len(got)-1])
		if last < 3 {
			t.Fatalf("本轮只发出 %d 帧（<start/delta/done>），断线重放无从验证", last)
		}
		replay, stopReplay := subscribeSSE(t, base, sid, strconv.FormatInt(last-2, 10))
		defer stopReplay()
		again := drainUntil(t, replay, "chat:done")
		if first := seqOf(again[0]); first <= last-2 {
			t.Fatalf("重连后没补上缺失事件：首帧 seq=%d，应 > %d", first, last-2)
		}
	})
}
