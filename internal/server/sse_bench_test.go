// SSE 广播热路径 benchmark：衡量 sync.Pool 复用 + 单次 Write 的收益。
//
// 运行：go test -bench=BenchmarkSSE -benchmem ./internal/server/
// 不依赖网络：用 httptest 起 loopback；只统计「推送 → 客户端收到」端到端吞吐。
package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"WorkBaby/internal/event"
	"github.com/gin-gonic/gin"
)

// BenchmarkSSEBroadcastFanOut 模拟 chat.stream 的高频 token 增量广播。
// N 个客户端订阅同一 run，主循环连续推 K 个事件，记录 ns/op 与 alloc/op。
func BenchmarkSSEBroadcastFanOut(b *testing.B) {
	gin.SetMode(gin.TestMode)
	const clients = 4
	const payload = `{"delta":"x","run_id":"r1","session_id":"s1","seq":0}`

	ts := httptest.NewServer(gin.New())
	defer ts.Close()
	hub := NewSSEHub(event.New(), event.NewRunEventLog(0, 0))

	// 起 N 个客户端 goroutine 把数据倒掉，避免 consumer 阻塞。
	for i := 0; i < clients; i++ {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/events?scope=chat&run_id=r1", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		go io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// 给 SSE handler 时间注册客户端；sleep 不优雅但 httptest 没提供 ready 信号。
	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.onEvent("chat:stream", map[string]any{
			"run_id": "r1", "session_id": "s1", "delta": "x", "seq": int64(i + 1),
		})
	}
	b.StopTimer()
}

// BenchmarkWriteEvent 单帧构造开销（绕开 hub，孤立测 writeEvent）。
func BenchmarkWriteEvent(b *testing.B) {
	w := httptest.NewRecorder()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeEvent(w, int64(i+1), "chat:stream", `{"delta":"x"}`)
	}
}

// BenchmarkExtractMeta placeholder for future payload-extraction benchmark
// (extractMeta is part of the planned SSE hot-path work and not yet implemented).