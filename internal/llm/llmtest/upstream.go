// Package llmtest 提供脚本化的假 LLM 上游（httptest），供三家协议 client 的线协议测试复用。
// 协议帧由调用方拼装（data:/event: 前缀与终止标记属于被测协议的一部分），本包只负责逐帧写出。
package llmtest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// SSE 逐帧 text/event-stream：每帧原样写出并以空行分帧、逐帧 flush。
func SSE(t *testing.T, frames ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f, ok := w.(http.Flusher)
		for _, frame := range frames {
			_, _ = w.Write([]byte(frame + "\n\n"))
			if ok {
				f.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// NDJSON 逐行 JSON（ollama 风格）：每行独立 flush。
func NDJSON(t *testing.T, lines ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		f, ok := w.(http.Flusher)
		for _, line := range lines {
			_, _ = w.Write([]byte(line + "\n"))
			if ok {
				f.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Error 固定状态/响应体/响应头的错误上游（429 限流、5xx 等路径）。
func Error(t *testing.T, status int, body string, header http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, vs := range header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}
