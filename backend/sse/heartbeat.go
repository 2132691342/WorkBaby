package sse

import "time"

// Heartbeat 周期发一行注释保活：中间有代理或防火墙时，静默连接会被掐掉。
func Heartbeat(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 25 * time.Second
	}
	return interval
}

// PingLine 是心跳写作的原文（SSE 注释行）。
const PingLine = ": ping\n\n"
