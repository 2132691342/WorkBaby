package llm

import "context"

// NewChunkStream 构造流式通道与 ctx 感知的发送函数。
// 发送在 ctx 取消时返回 false，生产者据此退出（消费者放弃时不挂死 goroutine 与上游连接）。
// 生产者需 defer close(ch) 并检查每次发送的返回值；buf <= 0 取默认 32。
func NewChunkStream(ctx context.Context, buf int) (chan StreamChunk, func(StreamChunk) bool) {
	if buf <= 0 {
		buf = 32
	}
	ch := make(chan StreamChunk, buf)
	send := func(c StreamChunk) bool {
		select {
		case ch <- c:
			return true
		case <-ctx.Done():
			return false
		}
	}
	return ch, send
}
