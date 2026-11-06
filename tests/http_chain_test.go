// Package tests 是场景测试：只覆盖「改这里会坏那里」的长链路，一文件一个被测对象：
//
//	http_chain_test.go    HTTP 全链路：鉴权门 → 信封 → 落盘，以及 SSE 通道
//	session_test.go       会话回放管线：分支遍历、轮组装、隐藏台账、改名保护
//	sandbox_test.go       沙箱治理：内置清单与资产咬合、技能生命周期
//	plugin_test.go        插件治理：声明启停、用户配置保留、版本漂移、卸载
//	mcp_test.go           MCP 配置：并发写与作用域遮蔽
//	harness_test.go       公共装具：两档 harness 与 HTTP 调用辅助
//	sandbox_live_test.go  真实运行时链路（go:build live，手动跑）
//
// 平凡断言（单 getter、纯函数往返、复述框架行为）不写：维护成本高于收益。
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

// TestHTTPChain 断言 HTTP 层的三条不变量，它们是前端全部调用的前提：
//
//  1. 鉴权是硬门。失败回 401 而不是业务信封——那是根本没进门。
//  2. 信封是唯一出口形状。成功、业务失败、未知路由、坏请求体全同构且恒 200，
//     前端才能只写一套解析。
//  3. 写入真的落盘。写经 HTTP、查直连 SQLite、读回走 HTTP 三个方向都要对上。
func TestHTTPChain(t *testing.T) {
	h := newHarness(t)

	// ---- 鉴权门 ----
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

	// ---- 信封同构 ----
	env, code := h.call(http.MethodGet, "/nope/not-here", nil)
	if code != http.StatusOK || env.Code != pkg.CodeUnknownRoute || env.Message == "" {
		t.Fatalf("未知路由应回 HTTP 200 / %q 且带 message，实际 http=%d code=%q msg=%q",
			pkg.CodeUnknownRoute, code, env.Code, env.Message)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/save", strings.NewReader(`{not json`))
	req.Header.Set("X-Desktop-Token", testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	var bad envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &bad); err != nil {
		t.Fatalf("坏请求体也必须回信封，实际 body=%s", rec.Body.String())
	}
	if bad.Code != pkg.CodeBadRequest || rec.Code != http.StatusOK {
		t.Fatalf("坏请求体应回 %q / HTTP 200，实际 code=%q http=%d", pkg.CodeBadRequest, bad.Code, rec.Code)
	}

	// ---- 写入落盘 ----
	env, code = h.call(http.MethodPost, "/settings/save", map[string]string{"ui.theme": "dark"})
	var saved map[string]string
	h.mustOK(env, code, "settings/save", &saved)
	if saved["ui.theme"] != "dark" {
		t.Fatalf("保存后应回 dark，实际 %q", saved["ui.theme"])
	}
	if stored, ok := h.store.Get("ui.theme"); !ok || stored != "dark" {
		t.Fatalf("setting 表里应是 dark，实际 %q（存在=%v）", stored, ok)
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
}

// TestSSEStream 断言事件流的四个契约：
// 鉴权走查询参数（EventSource 带不了自定义头）、连上先补一帧 stream:open、
// 广播的帧真的到达订阅者、广播必须发号（seq 单调是断线补帧的对齐依据）。
func TestSSEStream(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(h.server.Handler())
	t.Cleanup(srv.Close)

	// 无令牌：401。
	res, err := http.Get(srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SSE 无令牌应回 401，实际 %d", res.StatusCode)
	}

	// 有令牌：读到 stream:open 再读到一条广播帧。
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
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	waitForLine(t, lines, "stream:open")
	h.app.Hub.Broadcast("test:probe", map[string]any{"probe": "ok"})
	waitForLine(t, lines, `"event":"test:probe"`)

	// stream:open 是连接时直写的，不经过 hub，所以不计入 seq。
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
