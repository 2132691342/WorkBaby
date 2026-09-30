package server

import (
	"encoding/json"
	"sync"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// 每个客户端的缓冲与重放窗口：256 条足够覆盖一次断线重连。
const (
	clientBuffer = 256
	replayWindow = 256
	// deltaCoalesce 连续 delta 的合并窗口：流式正文每 token 一条事件，
	// 一秒能灌几百条，前端消费不过来就被判慢消费者断连——
	// 这是「回复过程中界面卡死」的共同根源。80ms 合并一次对观感无影响。
	deltaCoalesce = 80 * time.Millisecond
)

// Hub 是 SSE 广播中心：按会话分发，慢客户端直接断连并提示对账。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	replay  map[string][]domain.Envelope
	pend    map[string]*domain.Envelope // 会话内待合并的 delta；非 delta 事件到达时先冲刷保序
	timers  map[string]*time.Timer
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
	return &Hub{
		clients: map[*Client]bool{},
		replay:  map[string][]domain.Envelope{},
		pend:    map[string]*domain.Envelope{},
		timers:  map[string]*time.Timer{},
	}
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
// 连续的 chat:delta 先在会话内合并（80ms 窗口），非 delta 事件到达时
// 先冲刷挂起的 delta 再发，保证顺序。seq 由 Emitter 分配，合并不破坏
// 断线重放的一致性——重放里少的是中间增量，正文语义等价。
func (h *Hub) Publish(sessionID string, env domain.Envelope) {
	if env.Event != domain.EventChatDelta {
		// 非 delta：先冲刷挂起的 delta（保序），再发自己
		h.mu.Lock()
		pend := h.takePending(sessionID)
		h.mu.Unlock()
		if pend != nil {
			h.publishNow(sessionID, *pend)
		}
		h.publishNow(sessionID, env)
		return
	}

	d, ok := env.Data.(domain.DeltaData)
	if !ok {
		h.publishNow(sessionID, env)
		return
	}

	h.mu.Lock()
	if pend := h.pend[sessionID]; pend != nil {
		if pd, same := pend.Data.(domain.DeltaData); same && pd.EntryID == d.EntryID && pd.Kind == d.Kind {
			// 同一条目同一形态：正文拼进去（Data 是值，必须回写）
			pd.Delta += d.Delta
			pend.Data = pd
			h.mu.Unlock()
			return
		}
		// 形态切换（思考 ↔ 正文）或换了条目：先冲刷旧的
		delete(h.pend, sessionID)
		h.stopTimer(sessionID)
		h.mu.Unlock()
		h.publishNow(sessionID, *pend)
		h.mu.Lock()
	}
	e := env
	h.pend[sessionID] = &e
	h.timers[sessionID] = time.AfterFunc(deltaCoalesce, func() { h.FlushDelta(sessionID) })
	h.mu.Unlock()
}

// FlushDelta 把会话挂起的合并 delta 发出去；定时器与事件冲刷共用。
func (h *Hub) FlushDelta(sessionID string) {
	h.mu.Lock()
	pend := h.takePending(sessionID)
	h.mu.Unlock()
	if pend != nil {
		h.publishNow(sessionID, *pend)
	}
}

// takePending 取走并清掉会话的挂起 delta（调用方持有 h.mu）。
func (h *Hub) takePending(sessionID string) *domain.Envelope {
	pend := h.pend[sessionID]
	delete(h.pend, sessionID)
	h.stopTimer(sessionID)
	return pend
}

func (h *Hub) stopTimer(sessionID string) {
	if t, ok := h.timers[sessionID]; ok {
		t.Stop()
		delete(h.timers, sessionID)
	}
}

// publishNow 真正投递：写重放缓冲 + 推给所有订阅者。
func (h *Hub) publishNow(sessionID string, env domain.Envelope) {
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
			// 缓冲满：先挤掉积压的 delta（正文可由快照对账恢复），
			// 给关键事件腾位子。挤完还塞不进才断开。
			drainDeltas(c.ch)
			select {
			case c.ch <- env:
			default:
				h.Unsubscribe(c)
				pkg.Warnf("sse: 会话 %s 的客户端持续消费过慢，已断开（等待对账）", sessionID)
			}
		}
	}
}

// drainDeltas 把通道里积压的 chat:delta 挤到只剩最后一条。
// delta 可由快照与重放恢复，done / error 丢了前端就永远停在「运行中」。
func drainDeltas(ch chan domain.Envelope) {
	var last *domain.Envelope
	kept := make([]domain.Envelope, 0, 16)
	for {
		select {
		case ev := <-ch:
			if ev.Event == domain.EventChatDelta {
				last = &ev
				continue
			}
			kept = append(kept, ev)
		default:
			for _, ev := range kept {
				ch <- ev
			}
			if last != nil {
				ch <- *last
			}
			return
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
//
// data 里必须放**完整信封**而不是只有载荷：前端的 SSE 监听器拿到帧后
// 直接把 data 当信封用（`env.event` 决定路由、`env.data` 才是载荷）。
// 只发载荷的话 env.event 恒为 undefined，事件会被整条静默丢弃——
// 后端跑完 3 轮、数据库里答案齐全，界面却一个字都不显示。
func encodeEvent(env domain.Envelope) ([]byte, error) {
	raw, err := json.Marshal(env)
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
