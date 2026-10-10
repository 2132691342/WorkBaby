package server

import (
	"encoding/json"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 每个客户端的缓冲与重放窗口：256 条足够覆盖一次断线重连。
const (
	clientBuffer = 256
	replayWindow = 256
	// deltaCoalesce 连续 delta 的合并窗口：流式正文每 token 一条，不合流会把前端压成慢消费者。
	deltaCoalesce = 80 * time.Millisecond
	// replayGrace：重连是「先断后连」，立刻清就再也重放不出来。
	replayGrace = 2 * time.Minute
)

// Hub 是 SSE 广播中心：按会话分发，慢客户端直接断连并提示对账。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	replay  map[string][]domain.Envelope
	pend    map[string]*domain.Envelope // 会话内待合并的 delta；非 delta 事件到达时先冲刷保序
	timers  map[string]*time.Timer      // 会话的 delta 合流定时器
	sweeps  map[string]*time.Timer      // 会话无订阅者后的延迟回收定时器
}

// Client 是一个 SSE 订阅者。
type Client struct {
	SessionID string
	ch        chan domain.Envelope
	done      chan struct{}
	once      sync.Once
	// sendMu 保证同一个客户端的投递顺序：publishNow 是「读通道 → 挤 delta → 回灌」
	// 三步，两个并发发布者同时在上面操作会让回灌把后来的事件插到前面去，
	// 挤掉判据也随之失真。发送永远是非阻塞的，持锁不会卡住调用链。
	sendMu sync.Mutex
}

// NewHub 构造广播中心。
func NewHub() *Hub {
	return &Hub{
		clients: map[*Client]bool{},
		replay:  map[string][]domain.Envelope{},
		pend:    map[string]*domain.Envelope{},
		timers:  map[string]*time.Timer{},
		sweeps:  map[string]*time.Timer{},
	}
}

// Subscribe 订阅一个会话的事件流。
func (h *Hub) Subscribe(sessionID string) *Client {
	c := &Client{SessionID: sessionID, ch: make(chan domain.Envelope, clientBuffer), done: make(chan struct{})}
	h.mu.Lock()
	h.clients[c] = true
	// 重连回来了：撤掉延迟回收，否则这段重放窗口会在补帧过程中被清掉。
	if t, ok := h.sweeps[sessionID]; ok {
		t.Stop()
		delete(h.sweeps, sessionID)
	}
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
	h.scheduleSweep(c.SessionID)
	h.mu.Unlock()
}

// DropSession 立刻丢弃一个会话的全部缓冲（删会话时用，不留宽限期）。
func (h *Hub) DropSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.sweeps[sessionID]; ok {
		t.Stop()
		delete(h.sweeps, sessionID)
	}
	h.clear(sessionID)
}

// scheduleSweep 在最后一个订阅者离开后挂一张延迟回收的定时器（调用方持有 h.mu）。
func (h *Hub) scheduleSweep(sessionID string) {
	for c := range h.clients {
		if c.SessionID == sessionID {
			return
		}
	}
	if _, ok := h.sweeps[sessionID]; ok {
		return
	}
	h.sweeps[sessionID] = time.AfterFunc(replayGrace, func() {
		h.DropSession(sessionID)
	})
}

// clear 清掉一个会话的重放窗口 / 待合并 delta / 合流定时器（调用方持有 h.mu）。
func (h *Hub) clear(sessionID string) {
	delete(h.replay, sessionID)
	if t, ok := h.timers[sessionID]; ok {
		t.Stop()
		delete(h.timers, sessionID)
	}
	delete(h.pend, sessionID)
}

// Events 取出该客户端的事件通道。
func (c *Client) Events() <-chan domain.Envelope { return c.ch }

// Done 在客户端被移除时关闭。
func (c *Client) Done() <-chan struct{} { return c.done }

// Publish 推送给订阅该会话的全部客户端并写入重放缓冲。
// 连续的 chat:delta 按 80ms 窗口合流，非 delta 事件到达时先冲刷挂起的 delta 再发自己。
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
	var flush *domain.Envelope
	if pend := h.pend[sessionID]; pend != nil {
		if pd, same := pend.Data.(domain.DeltaData); same && pd.EntryID == d.EntryID && pd.Kind == d.Kind {
			// 同一条目同一形态：正文拼进去（Data 是值，必须回写）
			pd.Delta += d.Delta
			pend.Data = pd
			h.mu.Unlock()
			return
		}
		// 形态切换（思考 ↔ 正文）或换了条目：旧的一并取走。「取旧 + 装新」必须同一次持锁——
		// 中间放锁的话，并发 Publish 写进来的 pend 会被本 goroutine 覆盖，那段正文永久丢。
		flush = pend
	}
	e := env
	h.pend[sessionID] = &e
	h.armTimer(sessionID)
	h.mu.Unlock()
	if flush != nil {
		h.publishNow(sessionID, *flush)
	}
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
	// 定时器只停不删：下一段 delta 还要用它，删掉就得为每个 token 重新分配一张。
	if t, ok := h.timers[sessionID]; ok {
		t.Stop()
	}
	return pend
}

// armTimer 给会话挂上或重置合流定时器（调用方持有 h.mu）：会话只留一张、按需 Reset，
// 每 token 新起一张 AfterFunc 等于每 token 一次分配。
func (h *Hub) armTimer(sessionID string) {
	if t, ok := h.timers[sessionID]; ok {
		t.Reset(deltaCoalesce)
		return
	}
	h.timers[sessionID] = time.AfterFunc(deltaCoalesce, func() { h.FlushDelta(sessionID) })
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
		c.sendMu.Lock()
		select {
		case c.ch <- env:
		default:
			// 缓冲满：先挤掉积压的 delta（正文可由快照对账恢复），
			// 给关键事件腾位子。挤完还塞不进才断开。
			if drainDeltas(c.ch) {
				// 回灌时关键事件被并发写入挤掉：done / error 丢了前端就永远停在
				// 「运行中」，必须断开走快照对账，不能静默吞掉。
				h.Unsubscribe(c)
				pkg.Warnf("sse: 会话 %s 的客户端缓冲拥塞且回灌失败，已断开（等待对账）", sessionID)
				c.sendMu.Unlock()
				continue
			}
			select {
			case c.ch <- env:
			default:
				h.Unsubscribe(c)
				pkg.Warnf("sse: 会话 %s 的客户端持续消费过慢，已断开（等待对账）", sessionID)
			}
		}
		c.sendMu.Unlock()
	}
}

// drainDeltas 把通道里积压的 chat:delta 挤到只剩最后一条，返回是否有非 delta
// 事件在回灌时被挤掉。delta 可由快照与重放恢复；done / error 不行，调用方据此断开。
func drainDeltas(ch chan domain.Envelope) (lost bool) {
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
			// 回灌一律非阻塞：此刻通道刚被本函数读空，正常单写者场景都塞得回；
			// 并发写入挤满时宁可丢 delta（可由快照对账恢复），也不能卡住调用链。
			for _, ev := range kept {
				select {
				case ch <- ev:
				default:
					lost = true
				}
			}
			if last != nil {
				select {
				case ch <- *last:
				default:
				}
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
// data 放完整信封而不是只有载荷：前端直接按 `env.event` 路由、`env.data` 取载荷。
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
