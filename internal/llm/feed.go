// Feed 是流式适配器共用的事件喂送器：ctx 感知发送 + 上游空闲看门狗。
// 三个协议适配器都经它发事件，杜绝「缓冲满静默丢事件」与「上游停滞永久挂起」。
package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"WorkBaby/internal/pkg"
)

// StreamIdleTimeout 上游空闲超时：这么久没有收到任何数据就掐断连接。
// 代理停滞、上游不回包时，底层 scanner 会无限等下去，表现为「莫名卡住」。
// var 是为了测试能把超时调短，不用真等两分钟。
var StreamIdleTimeout = 120 * time.Second

// Feed 包装一条事件通道。Send 在 ctx 取消前阻塞而不是丢弃，
// 消费方（内核）保证读到通道关闭，所以不会死锁。
type Feed struct {
	ctx      context.Context
	cancel   context.CancelFunc
	events   chan Event
	activity chan struct{}
	timedOut atomic.Bool
	once     sync.Once
}

// NewFeed 构造喂送器；ctx 是调用方（内核）的取消信号。
func NewFeed(ctx context.Context, buf int) *Feed {
	sctx, cancel := context.WithCancel(ctx)
	return &Feed{
		ctx:      sctx,
		cancel:   cancel,
		events:   make(chan Event, buf),
		activity: make(chan struct{}, 1),
	}
}

// Ctx 返回喂送器内部 ctx：适配器用它发 HTTP 请求，看门狗取消它即断流。
func (f *Feed) Ctx() context.Context { return f.ctx }

// Events 取事件通道；Close 时关闭。
func (f *Feed) Events() <-chan Event { return f.events }

// Ping 报告「上游还活着」：每收到一行数据调一次，重置空闲计时。
func (f *Feed) Ping() {
	select {
	case f.activity <- struct{}{}:
	default:
	}
}

// Send 发送一个事件；先非阻塞投递（缓冲有位就直接进，不受取消影响），
// 缓冲满才阻塞等待，此时 ctx 取消则放弃。两段式是为了避免
// 「通道可写但 ctx 恰好已取消」时 select 随机选分支丢掉收尾事件。
func (f *Feed) Send(e Event) {
	select {
	case f.events <- e:
		return
	default:
	}
	select {
	case f.events <- e:
	case <-f.ctx.Done():
	}
}

// TimedOut 报告流是否因空闲超时被看门狗掐断。
func (f *Feed) TimedOut() bool { return f.timedOut.Load() }

// WatchIdle 空闲看门狗：超时未活动就取消内部 ctx 掐断连接。
// 只在兜底路径触发，正常完成由 consume 自己收尾。
func (f *Feed) WatchIdle() {
	for {
		timer := time.NewTimer(StreamIdleTimeout)
		select {
		case <-f.ctx.Done():
			timer.Stop()
			return
		case <-f.activity:
			timer.Stop()
		case <-timer.C:
			f.timedOut.Store(true)
			f.cancel()
			return
		}
	}
}

// Close 收尾：取消内部 ctx 并关闭事件通道。由适配器 goroutine defer 调用。
func (f *Feed) Close() {
	f.once.Do(func() {
		f.cancel()
		close(f.events)
	})
}

// IdleError 空闲超时的统一错误。
func IdleError() error {
	return pkg.New(3002, "模型服务长时间没有返回数据，连接已断开，请重试", "")
}
