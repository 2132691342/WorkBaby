package server

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMetaHandler 只实现测试需要的几个方法（meta 域），
// 避免依赖完整 Handler 的 Startup（需要 DB/配置/网络）。
type mockMetaHandler struct {
	*api.Handler
}

func (m *mockMetaHandler) GetVersion() domain.VersionInfo {
	return domain.VersionInfo{AppName: "WorkBaby", Version: "test", Phase: "dual-host"}
}

func (m *mockMetaHandler) GetHealth() domain.HealthInfo {
	return domain.HealthInfo{Status: "ok", Providers: 0}
}

// newSSETestServer 起一个只挂 SSE 路由的测试服务。
func newSSETestServer(log *event.RunEventLog) (*httptest.Server, *SSEHub) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	hub := NewSSEHub(event.New(), log)
	r.GET("/api/v1/events", hub.Serve)
	return httptest.NewServer(r), hub
}

// readFrames 按 SSE 帧（以空行结束）读取 n 帧；读不到时阻塞至连接关闭。
func readFrames(t *testing.T, body interface{ Read([]byte) (int, error) }, n int) string {
	t.Helper()
	br := bufio.NewReader(body)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return sb.String()
			}
			sb.WriteString(line)
			if line == "\n" {
				break
			}
		}
	}
	return sb.String()
}

func TestSSEServe(t *testing.T) {
	ts, hub := newSSETestServer(event.NewRunEventLog(0, 0))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/events?scope=chat&runId=r1")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// 先读 sse-ready 帧
	got := readFrames(t, resp.Body, 1)
	assert.Contains(t, got, "sse-ready")

	// 触发事件：hub.onEvent 推送 chat:stream 给匹配 run_id 的连接
	hub.onEvent("chat:stream", map[string]any{"run_id": "r1", "delta": "hi", "seq": int64(1)})
	got = readFrames(t, resp.Body, 1)
	assert.Contains(t, got, "event: chat:stream")
	assert.Contains(t, got, `"delta":"hi"`)
	assert.Contains(t, got, "id: 1")
}

// TestSSEReplayFromLastEventID 断线重连按 Last-Event-ID 补发错过的事件。
func TestSSEReplayFromLastEventID(t *testing.T) {
	log := event.NewRunEventLog(0, 0)
	ts, _ := newSSETestServer(log)
	defer ts.Close()

	for i := 1; i <= 3; i++ {
		log.Append("r1", "chat:stream", map[string]any{"delta": fmt.Sprintf("d%d", i)})
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?scope=chat&runId=r1", nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	got := readFrames(t, resp.Body, 3) // 重放 d2、d3 + sse-ready
	assert.Contains(t, got, `"delta":"d2"`)
	assert.Contains(t, got, `"delta":"d3"`)
	assert.NotContains(t, got, `"delta":"d1"`)
}

// TestSSEReplayGap 缓冲滚动覆盖后重放不完整，必须推 chat:gap 让前端转全量回补。
func TestSSEReplayGap(t *testing.T) {
	log := event.NewRunEventLog(2, 4) // 每 run 只留 2 条
	ts, _ := newSSETestServer(log)
	defer ts.Close()

	for i := 1; i <= 5; i++ {
		log.Append("r1", "chat:stream", map[string]any{"delta": fmt.Sprintf("d%d", i)})
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events?scope=chat&runId=r1", nil)
	require.NoError(t, err)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	got := readFrames(t, resp.Body, 2) // gap + sse-ready
	assert.Contains(t, got, "event: chat:gap")
}

// TestSSESlowClientClosed 慢客户端不静默丢帧，而是关闭连接触发重连重放。
func TestSSESlowClientClosed(t *testing.T) {
	ts, hub := newSSETestServer(event.NewRunEventLog(0, 0))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/events?scope=chat&runId=r1")
	require.NoError(t, err)
	defer resp.Body.Close()

	// 客户端不消费；缓冲（256）打满后 hub 关闭连接
	for i := 0; i < 300; i++ {
		hub.onEvent("chat:stream", map[string]any{"run_id": "r1", "delta": "x", "seq": int64(i + 1)})
	}

	br := bufio.NewReader(resp.Body)
	var lastErr error
	// CI 上 Go 调度 + 网络 I/O 都比本地慢：原 3s deadline 在 windows-latest runner
	// 上不够，延长到 10s 并跳过 -short 模式（CI 默认 short，本地显式 -short 跳过）。
	if testing.Short() {
		t.Skip("flake test, skipped in -short mode")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, lastErr = br.ReadString('\n'); lastErr != nil {
			break
		}
	}
	assert.Error(t, lastErr, "慢客户端连接应被关闭")
}
