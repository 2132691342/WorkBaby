package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"WorkBaby/internal/pkg"
)

// SSE 流式：每个 chunk 是 `data: {json}\n\n` 形式（OpenAI / Anthropic），空行分帧。
// Ollama 用 ndjson（每行一个 JSON 对象），单独处理。

// NewHTTPClient 建连 10s / 响应头 30s；不设 Client.Timeout——它会计入响应体读取，
// 会把长流式对话在 30s 处硬截断。流式请求的生命周期由调用方 ctx 控制。
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          8,
			IdleConnTimeout:       60 * time.Second,
		},
	}
}

// DoJSON 简单 JSON POST；用于非流式的 Chat / Models / Ping。
func DoJSON(ctx context.Context, hc *http.Client, method, url string, headers map[string]string, body any, out any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bs, err := json.Marshal(body)
		if err != nil {
			return nil, pkg.Wrap(3030, "marshal request failed", err)
		}
		bodyReader = bytes.NewReader(bs)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, pkg.Wrap(3031, "build request failed", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		// transport error → 3005 timeout / 网络错误
		return nil, pkg.Wrap(3005, "http request failed", err)
	}
	if out == nil {
		return resp, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return resp, UpstreamStatusErr(resp.StatusCode, pkg.TruncateRunes(string(raw), 200), resp.Header.Get)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp, pkg.Wrap(3032, "decode response failed", err)
	}
	return resp, nil
}
