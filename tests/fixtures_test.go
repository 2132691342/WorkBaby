// 共享测试装具：假上游、帧捕获与文件助手。
// 这些是多个主题文件共用的脚手架，放在这里只写一份。
package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/sse"
)

// frameCapture 收订阅到的帧：广播入队与出队是异步的，断言前要等泵送达。
type frameCapture struct {
	mu     sync.Mutex
	frames []sse.Frame
}

// sink 把一帧记下来。
func (c *frameCapture) sink(frame sse.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

// of 取某个事件的所有帧。
func (c *frameCapture) of(event string) []sse.Frame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sse.Frame, 0, len(c.frames))
	for _, f := range c.frames {
		if f.Event == event {
			out = append(out, f)
		}
	}
	return out
}

// waitFrames 等某个事件攒够 n 帧；超时即失败。
func (c *frameCapture) waitFrames(t *testing.T, event string, n int) []sse.Frame {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.of(event); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等事件 %s ×%d 超时，实收 %d 帧", event, n, len(c.of(event)))
	return nil
}

// scriptServer 起一个按脚本吐 SSE 分片的假上游，并记下收到的请求体。
// summaryChunks 非空时，请求体带压缩系统提示的改走那套分片——压缩总结与
// 正常对话共用一个假上游，省掉第二份实现。
// followUpChunks 非空时，第一次请求之后都改走那套分片——「先要工具、
// 拿到结果再收尾」的场景用它。
type scriptServer struct {
	server         *httptest.Server
	mu             sync.Mutex
	bodies         []string
	chunks         []string
	summaryChunks  []string
	followUpChunks []string
}

// newScriptServer 造一个假上游：每个请求吐完 chunks 后结束（不再续轮）。
func newScriptServer(t *testing.T, chunks []string) *scriptServer {
	t.Helper()
	s := &scriptServer{chunks: chunks}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = readFull(r, raw)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(raw))
		count := len(s.bodies)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range s.replyChunks(string(raw), count) {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(s.server.Close)
	return s
}

// newSummaryServer 造一个两用假上游：压缩总结走摘要脚本，正常对话走对话脚本。
func newSummaryServer(t *testing.T, chatChunks []string) *scriptServer {
	t.Helper()
	s := newScriptServer(t, chatChunks)
	s.summaryChunks = []string{
		`{"choices":[{"delta":{"content":"用户问了目录结构，助手作答。"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	return s
}

// replyChunks 按请求体与请求次数挑该回哪套分片。
func (s *scriptServer) replyChunks(body string, count int) []string {
	if len(s.summaryChunks) > 0 && strings.Contains(body, compactionSystemDigest) {
		return s.summaryChunks
	}
	if len(s.followUpChunks) > 0 && count > 1 {
		return s.followUpChunks
	}
	return s.chunks
}

// requestBodies 取到目前为止的请求体副本。
func (s *scriptServer) requestBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.bodies...)
}

// resetBodies 清掉已记的请求体，用于切换场景。
func (s *scriptServer) resetBodies() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = nil
}

// compactionSystemDigest 用于识别总结调用的片段：取压缩系统提示里稳定的词。
const compactionSystemDigest = "对话整理助手"

// readFull 把请求体读满 buf。
func readFull(r *http.Request, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Body.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// replyChunks 是一轮「纯文本回复、不要工具」的脚本。
func replyChunks(text string) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, text),
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
}

// bindWorkspaceAndModel 给 harness 备好工作区、服务商与选中模型。
func bindWorkspaceAndModel(t *testing.T, h *harness, base string) {
	t.Helper()
	env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
	h.mustOK(env, code, "workspace", nil)
	env, code = h.call("POST", "/providers/upsert", domain.ProviderUpsertREQ{
		ProviderName: "up", BaseUrl: base, ApiKey: "k",
		Models: []domain.ProviderModelEntryVO{{ModelId: "m", ContextWindow: 100000}},
	})
	h.mustOK(env, code, "upsert", nil)
	if err := h.app.Chat.SetModel("", "up", "m"); err != nil {
		t.Fatalf("选模型失败：%v", err)
	}
}

// waitIdle 等当前运行收尾；超时即失败。
func waitIdle(t *testing.T, h *harness) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !h.app.Chat.Busy() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("3 秒后运行仍未收尾")
}

// writeFile 写一个文件，父目录不存在就建。
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}
