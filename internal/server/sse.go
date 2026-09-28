package server

import (
	"encoding/json"
	"sync"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// 每个客户端的缓冲与重放窗口：256 条足够覆盖一次断线重连。
const (
	clientBuffer = 256
	replayWindow = 256
)

// Hub 是 SSE 广播中心：按会话分发，慢客户端直接断连并提示对账。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	replay  map[string][]domain.Envelope
}

// Client 是一个 SSE 订阅者。
type Client struct {
	SessionID string
	ch        chan domain.Envelope
	done      chan struct{}
	once      sync.Once
}

// NewHub 构造广播中心。
func NewHub() *Hub {
	return &Hub{clients: map[*Client]bool{}, replay: map[string][]domain.Envelope{}}
}

// Subscribe 订阅一个会话的事件流。
func (h *Hub) Subscribe(sessionID string) *Client {
	c := &Client{SessionID: sessionID, ch: make(chan domain.Envelope, clientBuffer), done: make(chan struct{})}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	return c
}

// Unsubscribe 退订并关闭通道。
func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		c.once.Do(func() { close(c.done) })
	}
	h.mu.Unlock()
}

// Events 取出该客户端的事件通道。
func (c *Client) Events() <-chan domain.Envelope { return c.ch }

// Done 在客户端被移除时关闭。
func (c *Client) Done() <-chan struct{} { return c.done }

// Publish 把一个事件推给订阅该会话的全部客户端，并写入重放缓冲。
func (h *Hub) Publish(sessionID string, env domain.Envelope) {
	h.mu.Lock()
	buf := append(h.replay[sessionID], env)
	if len(buf) > replayWindow {
		buf = buf[len(buf)-replayWindow:]
	}
	h.replay[sessionID] = buf
	targets := make([]*Client, 0, 4)
	for c := range h.clients {
		if c.SessionID == sessionID {
			targets = append(targets, c)
		}
	}
	h.mu.Unlock()

	for _, c := range targets {
		select {
		case c.ch <- env:
		default:
			// 慢客户端：断连并提示前端拉快照对账，绝不阻塞内核。
			h.Unsubscribe(c)
			pkg.Warnf("sse: 会话 %s 的客户端消费过慢，已断开", sessionID)
		}
	}
}

// Replay 返回重放缓冲中晚于 after 的事件。
func (h *Hub) Replay(sessionID string, after int64) []domain.Envelope {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := []domain.Envelope{}
	for _, e := range h.replay[sessionID] {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out
}

// encodeEvent 把信封序列化成 SSE 帧。
func encodeEvent(env domain.Envelope) ([]byte, error) {
	raw, err := json.Marshal(env.Data)
	if err != nil {
		return nil, pkg.Wrap(2201, "序列化事件失败", err)
	}
	out := []byte("event: " + env.Event + "\n")
	out = append(out, []byte("id: "+itoa64(env.Seq)+"\n")...)
	out = append(out, []byte("data: ")...)
	out = append(out, raw...)
	out = append(out, '\n', '\n')
	return out, nil
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
