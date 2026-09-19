package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/event"
	"github.com/gin-gonic/gin"
)

// SSEHub 把 event.Bus 的 chat:* / pet:* / app:* / task:* 事件桥接到 SSE 长连接，
// 客户端按 (scope, runID) 过滤；每条事件带 run 内单调 seq，重连时按 Last-Event-ID 重放，
// 缓冲被覆盖则推 chat:gap 由前端转全量回补。
type SSEHub struct {
	bus     *event.Bus
	log     *event.RunEventLog
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	closed  bool
}

// sseClient 一条 SSE 连接；done 由 close 保证只关一次（Serve 的 defer 与 hub.Close 都会调）。
type sseClient struct {
	scope string // chat | pet | app
	runID string // 空 = 全部 run
	ch    chan sseMsg
	done  chan struct{}
	once  sync.Once
}

// close 幂等关闭连接；channel 不关闭（交由 GC 回收），避免向已关闭 channel 发送 panic。
func (c *sseClient) close() { c.once.Do(func() { close(c.done) }) }

type sseMsg struct {
	seq  int64
	name string
	data string
}

// NewSSEHub 构造 hub 并订阅 event.Bus。log 可为 nil（退化为无重放）。
func NewSSEHub(bus *event.Bus, log *event.RunEventLog) *SSEHub {
	h := &SSEHub{
		bus:     bus,
		log:     log,
		clients: make(map[*sseClient]struct{}),
	}
	// 桥接 chat:*/pet:*/app:*/task:* 四类事件（前端订阅范围）。
	// task:* 必须在此登记：后台任务面板按 scope=task 订阅，缺了这一行任务中心收不到任何实时变化。
	h.bus.Subscribe(event.MatchPrefix("chat:"), h.onEvent)
	h.bus.Subscribe(event.MatchPrefix("pet:"), h.onEvent)
	h.bus.Subscribe(event.MatchPrefix("app:"), h.onEvent)
	h.bus.Subscribe(event.MatchPrefix("task:"), h.onEvent)
	return h
}

func (h *SSEHub) onEvent(name string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	runID := extractRunID(payload)
	msg := sseMsg{seq: extractSeq(payload), name: name, data: string(data)}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.scope != "" && !strings.HasPrefix(name, c.scope+":") {
			continue
		}
		if c.runID != "" && c.runID != runID {
			continue
		}
		select {
		case c.ch <- msg:
		default:
			// 慢客户端：关闭连接触发浏览器重连（EventSource 自带 Last-Event-ID 重放），
			// 不做丢帧——丢帧会让前端永久停在「正在输入」。
			c.close()
		}
	}
}

// extractRunID 从事件载荷中提取订阅键（run_id）；无 run 归属的事件返回空串，只按 scope 过滤。
func extractRunID(payload any) string {
	if m, ok := payload.(map[string]any); ok {
		if v, ok := m["run_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// extractSeq 取出事件序号（service emit 时由 RunEventLog 注入）；无序号返回 0。
func extractSeq(payload any) int64 {
	m, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	switch v := m["seq"].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

// lastEventSeq 解析断线重连的起点序号：优先标准 Last-Event-ID 头，回退 query。
func lastEventSeq(c *gin.Context) int64 {
	raw := c.GetHeader("Last-Event-ID")
	if raw == "" {
		raw = c.Query("last_event_id")
	}
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Serve 处理 GET /api/v1/events?scope=chat&runId={runId}。
func (h *SSEHub) Serve(c *gin.Context) {
	scope := c.DefaultQuery("scope", "chat")
	runID := c.Query("runId")
	afterSeq := lastEventSeq(c)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	client := &sseClient{
		scope: scope,
		runID: runID,
		ch:    make(chan sseMsg, 256),
		done:  make(chan struct{}),
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		h.mu.Unlock()
		client.close()
	}()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		Fail(c, fmt.Errorf("streaming unsupported"))
		return
	}

	// 重放（afterSeq 之后补齐）：首连时 afterSeq=0，等价于补发该 run 已缓存的全部事件。
	//
	// 首连也必须重放：run 在 POST /chat/stream 返回时就已在 goroutine 里开跑，
	// 「HTTP 响应 → SSE 握手」窗口内发出的 chat:stream.start 乃至 chat:done
	// 若不补回，前端会永远等不到终态。
	if runID != "" && h.log != nil {
		events, covered := h.log.Replay(runID, afterSeq)
		if !covered {
			fmt.Fprintf(c.Writer, "event: chat:gap\ndata: {\"run_id\":%q,\"last_seq\":%d}\n\n", runID, afterSeq)
		}
		for _, e := range events {
			writeEvent(c.Writer, e.Seq, e.Name, e.Data)
		}
		flusher.Flush()
	}

	// 主动通知前端已就绪（消费方以此确认连接建立）
	fmt.Fprintf(c.Writer, "event: sse-ready\ndata: {\"scope\":%q}\n\n", scope)
	flusher.Flush()

	// 心跳：30s 一次具名 ping 事件（可靠性套件）。
	// 具名事件（而非注释帧）让前端 EventSource 可见，watchdog 据此区分「连接活着但空闲」与「假死」。
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-client.done:
			return
		case msg := <-client.ch:
			writeEvent(c.Writer, msg.seq, msg.name, msg.data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(c.Writer, "event: ping\ndata: {\"t\":%d}\n\n", time.Now().UnixMilli())
			flusher.Flush()
		}
	}
}

// writeEvent 输出一帧；seq>0 时带 id，供浏览器重连时回传。
func writeEvent(w http.ResponseWriter, seq int64, name, data string) {
	if seq > 0 {
		fmt.Fprintf(w, "id: %d\n", seq)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
}

// Close 关闭全部连接（Shutdown 时调用）；幂等，可安全重复调用。
func (h *SSEHub) Close() {
	h.mu.Lock()
	clients := make([]*sseClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = make(map[*sseClient]struct{})
	h.closed = true
	h.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
}
