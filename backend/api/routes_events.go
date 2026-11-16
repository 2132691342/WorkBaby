package api

import (
	"net/http"
	"sync"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/sse"
)

// events 是 SSE 长连接：帧格式 { seq, event, data, sent_at }。
// seq 由 hub 统一发号、每帧 flush、心跳是注释行不占 seq——界面不漂全靠这三条。
func (s *Server) events(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	// 帧与心跳都写同一个 ResponseWriter，而它们来自两个 goroutine：
	// hub 的发送泵一个、这里的 ticker 一个。http.ResponseWriter 不支持并发写，
	// 交错起来会把一帧劈成两半，前端解析失败就静默丢帧。
	var writeMu sync.Mutex
	write := func(text string) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err := c.Writer.WriteString(text); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	// 先把当前 seq 告诉客户端：首屏拿到它再决定要不要补帧。
	openFrame := sse.Frame{Seq: s.hub.Seq(), Event: "stream:open", Data: map[string]any{}}
	if text, err := sse.Encode(openFrame); err == nil {
		if err := write(text); err != nil {
			return
		}
	}

	cancel, pumpDone := s.hub.Subscribe(queryInt64(c, "since"), func(frame sse.Frame) error {
		text, err := sse.Encode(frame)
		if err != nil {
			return nil
		}
		return write(text)
	})
	defer cancel()

	// 心跳跟着连接走：连接断开就退出循环，不留下空转的定时器。
	ticker := time.NewTicker(sse.Heartbeat(0))
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-pumpDone:
			// 泵已收摊（溢出/写失败）：结束连接让客户端带 since 重连，
			// 留着一条只有心跳的连接等于让界面静默失联。
			return
		case <-ticker.C:
			if err := write(sse.PingLine); err != nil {
				return
			}
		}
	}
}

// queryInt64 读一个 int64 查询参数；非数字或溢出当 0。
func queryInt64(c *gin.Context, key string) int64 {
	value, err := strconv.ParseInt(c.Query(key), 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
