// 回环 HTTP 全链路：鉴权门 → CORS 预检 → 信封 → 落盘 → SSE 通道。
package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/pkg"
)

// TestChain 断言回环 HTTP 的全部契约：鉴权门、CORS 预检、信封同构、写入落盘、SSE 通道。
func TestChain(t *testing.T) {
	h := newHarness(t)

	// ---- 鉴权门：无令牌 / 错令牌回 401 且不带业务信封 ----
	for _, tc := range []struct{ name, token string }{{"无令牌", ""}, {"错令牌", "wrong"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
		if tc.token != "" {
			req.Header.Set("X-Desktop-Token", tc.token)
		}
		rec := httptest.NewRecorder()
		h.server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s：应回 401，实际 %d", tc.name, rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"code"`)) {
			t.Errorf("%s：401 不该带业务信封，实际 body=%s", tc.name, rec.Body.String())
		}
	}

	// ---- CORS 预检：绕过鉴权门、回 204 与允许头 ----
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/bootstrap", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检应回 204，实际 %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("预检应允许源 wails.localhost，实际 %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Desktop-Token") {
		t.Fatalf("预检应允许 X-Desktop-Token 头，实际 %q", got)
	}
	// 带源的正常请求（含 SSE 的 EventSource）响应上也要有允许头。
	req = httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	req.Header.Set("X-Desktop-Token", testToken)
	req.Header.Set("Origin", "http://wails.localhost")
	rec = httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://wails.localhost" {
		t.Fatalf("响应应携带允许头给 EventSource，实际 %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	// ---- 信封同构：成功 / 业务失败 / 未知路由 / 坏请求体全为 200 + code ----
	env, code := h.call(http.MethodGet, "/nope/not-here", nil)
	if code != http.StatusOK || env.Code != pkg.CodeUnknownRoute || env.Message == "" {
		t.Fatalf("未知路由应回 HTTP 200 / %q 且带 message，实际 http=%d code=%q msg=%q",
			pkg.CodeUnknownRoute, code, env.Code, env.Message)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/settings/save", strings.NewReader(`{not json`))
	req.Header.Set("X-Desktop-Token", testToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	var bad envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &bad); err != nil {
		t.Fatalf("坏请求体也必须回信封，实际 body=%s", rec.Body.String())
	}
	if bad.Code != pkg.CodeBadRequest || rec.Code != http.StatusOK {
		t.Fatalf("坏请求体应回 %q / HTTP 200，实际 code=%q http=%d", pkg.CodeBadRequest, bad.Code, rec.Code)
	}

	// ---- 写入落盘：HTTP 写 → 直查存储文件 → HTTP 读回 ----
	env, code = h.call(http.MethodPost, "/settings/save", map[string]string{"ui.theme": "dark"})
	var saved map[string]string
	h.mustOK(env, code, "settings/save", &saved)
	if saved["ui.theme"] != "dark" {
		t.Fatalf("保存后应回 dark，实际 %q", saved["ui.theme"])
	}
	if stored, ok := h.store.Get("ui.theme"); !ok || stored != "dark" {
		t.Fatalf("存储文件里应是 dark，实际 %q（存在=%v）", stored, ok)
	}
	env, code = h.call(http.MethodGet, "/settings", nil)
	var readBack map[string]string
	h.mustOK(env, code, "settings", &readBack)
	if readBack["ui.theme"] != "dark" {
		t.Fatalf("重新读取应得 dark，实际 %q", readBack["ui.theme"])
	}
	env, code = h.call(http.MethodGet, "/bootstrap", nil)
	var boot map[string]any
	h.mustOK(env, code, "bootstrap", &boot)
	if _, ok := boot["app_contract"]; !ok {
		t.Fatalf("bootstrap 缺 app_contract：前端首屏全靠它，实际键=%v", keysOf(boot))
	}

	// ---- SSE 通道：查询参数鉴权、先补 stream:open、广播到达、seq 单调 ----
	srv := httptest.NewServer(h.server.Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SSE 无令牌应回 401，实际 %d", res.StatusCode)
	}
	res, err = http.Get(srv.URL + "/api/v1/events?token=" + testToken)
	if err != nil {
		t.Fatalf("连接 SSE 失败：%v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("SSE 有令牌应回 200，实际 %d", res.StatusCode)
	}
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		// 读出错也要报出来：连接半路断了会让 waitForLine 超时，
		// 但那时的真实原因是传输错误，不是「帧没到」。
		if err := scanner.Err(); err != nil {
			lines <- "scan error: " + err.Error()
		}
	}()
	waitForLine(t, lines, "stream:open")
	h.app.Hub.Broadcast("test:probe", map[string]any{"probe": "ok"})
	waitForLine(t, lines, `"event":"test:probe"`)
	if h.app.Hub.Seq() < 1 {
		t.Fatalf("广播应至少发过 1 帧，实际 seq=%d", h.app.Hub.Seq())
	}
}

// waitForLine 在 SSE 行流里等一段文本出现；超时即失败。
func waitForLine(t *testing.T, lines <-chan string, needle string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("SSE 流已结束，没等到 %q", needle)
			}
			if strings.Contains(line, needle) {
				return
			}
		case <-deadline:
			t.Fatalf("等 SSE 行 %q 超时", needle)
		}
	}
}

// keysOf 取 map 的键，用于失败信息。
func keysOf(data map[string]any) []string {
	out := make([]string, 0, len(data))
	for key := range data {
		out = append(out, key)
	}
	return out
}
