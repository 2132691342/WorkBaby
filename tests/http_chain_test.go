// Package tests 是场景测试：只覆盖「改这里会坏那里」的长链路。
//
// 四个被测对象各一个文件：
//
//	http_chain_test.go    HTTP 全链路：鉴权门 → 信封 → 落盘，以及 SSE 通道
//	session_test.go       会话回放管线：分支遍历、轮组装、隐藏台账、改名保护
//	sandbox_test.go       沙箱治理：内置清单、技能落位、插件声明的启停与漂移
//	sandbox_live_test.go  真实运行时链路（go:build live，手动跑）
//
// 平凡断言（单 getter、纯函数往返、复述框架行为）不写：维护成本高于收益。
package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/api"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

const testToken = "test-token"

// harness 是装好的后端：真 SQLite、真临时布局、真 gin 中间件栈。
type harness struct {
	t      *testing.T
	layout pkg.Layout
	app    *service.App
	store  *repo.Store
	server *api.Server
}

// newHarness 造一个隔离的后端；数据落在 t.TempDir()，不碰真实数据目录。
func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	dir := t.TempDir()
	layout := pkg.Layout{
		DataDir:      dir,
		RuntimeDir:   filepath.Join(dir, "runtime"),
		NodeDir:      filepath.Join(dir, "runtime", "node"),
		GitDir:       filepath.Join(dir, "runtime", "PortableGit"),
		PythonDir:    filepath.Join(dir, "runtime", "python"),
		PiAppDir:     filepath.Join(dir, "pi-app"),
		PiAgentDir:   filepath.Join(dir, "pi"),
		CacheDir:     filepath.Join(dir, "cache"),
		NpmCacheDir:  filepath.Join(dir, "cache", "npm"),
		NpmGlobalDir: filepath.Join(dir, "cache", "npm-global"),
		LogDir:       filepath.Join(dir, "logs"),
		DbPath:       filepath.Join(dir, "workbaby.db"),
		AssetsDir:    filepath.Join(dir, "assets"),
	}
	if err := pkg.EnsureDir(layout.PiAgentDir); err != nil {
		t.Fatalf("准备目录失败：%v", err)
	}
	pkg.SetLogDir("")

	store, err := repo.Open(layout.DbPath)
	if err != nil {
		t.Fatalf("开设置库失败：%v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hub := sse.New(64)
	app := service.New(service.Options{Layout: layout, Store: store, Hub: hub, Version: "test"})
	return &harness{t: t, layout: layout, app: app, store: store, server: api.New(app, hub, testToken)}
}

// envelope 是 HTTP 出口形状。
type envelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

// call 发一个带令牌的请求，回解码后的信封与 HTTP 状态码。
func (h *harness) call(method, path string, body any) (envelope, int) {
	h.t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("编码请求体失败：%v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, reader)
	req.Header.Set("X-Desktop-Token", testToken)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)

	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env, rec.Code
}

// mustOK 断言信封 code 为 "0"、HTTP 200，并把 data 解进 out。
func (h *harness) mustOK(env envelope, httpCode int, what string, out any) {
	h.t.Helper()
	if httpCode != http.StatusOK {
		h.t.Fatalf("%s：HTTP 应为 200，实际 %d（body=%s）", what, httpCode, string(env.Data))
	}
	if env.Code != pkg.CodeOK {
		h.t.Fatalf("%s：信封 code 应为 %q，实际 %q %s（detail=%s）",
			what, pkg.CodeOK, env.Code, env.Message, env.Detail)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			h.t.Fatalf("%s：data 解析失败 %v（raw=%s）", what, err, string(env.Data))
		}
	}
}

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
// 广播的帧真的到达订阅者、断开后服务端处理器退出（否则每个连接漏一个 goroutine）。
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

	// 广播必须真的发号：seq 单调是断线补帧的对齐依据。
	// 注意 stream:open 是连接时直写的，不经过 hub，所以不计入 seq。
	if h.app.Hub.Seq() < 1 {
		t.Fatalf("广播应至少发过 1 帧，实际 seq=%d", h.app.Hub.Seq())
	}

	// 断开：读端关掉后处理器必须退出，不能留着一个写不出去的 goroutine。
	res.Body.Close()
	srv.Close()
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

// writeSessionFile 在会话根下写一份会话 JSONL，返回文件路径。
// 必须落在会话根内：路径越界本身会先被拦掉，那就测不到后续的写入纪律了。
func writeSessionFile(t *testing.T, root string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "--C--ws--")
	if err := pkg.EnsureDir(dir); err != nil {
		t.Fatalf("建会话目录失败：%v", err)
	}
	file := filepath.Join(dir, "sess.jsonl")
	header := `{"type":"session","version":3,"id":"sess","cwd":"C:\\ws","timestamp":"2026-01-01T00:00:00.000Z"}`
	body := header + "\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatalf("写会话文件失败：%v", err)
	}
	return file
}
