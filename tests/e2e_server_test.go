// Package tests 是场景测试：只断言「改这里会坏那里」的长链路。
package tests

import (
	"bytes"
	"context"
	"encoding/json"
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

// harness 是装好的后端：真 SQLite、真临时布局、真 gin 引擎。
type harness struct {
	t      *testing.T
	layout pkg.Layout
	app    *service.App
	store  *repo.Store
	server *api.Server
	port   int
}

// newHarness 造一个隔离的后端（数据落在 t.TempDir()，不碰真实数据目录）。
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
	for _, sub := range []string{layout.PiAgentDir, layout.LogDir} {
		if err := pkg.EnsureDir(sub); err != nil {
			t.Fatalf("准备目录 %s 失败：%v", sub, err)
		}
	}
	pkg.SetLogDir(layout.LogDir)
	t.Cleanup(func() { pkg.SetLogDir("") })

	store, err := repo.Open(layout.DbPath)
	if err != nil {
		t.Fatalf("开设置库失败：%v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hub := sse.New(64)
	app := service.New(service.Options{Layout: layout, Store: store, Hub: hub, Version: "test"})
	server := api.New(app, hub, testToken)
	port, err := server.Start()
	if err != nil {
		t.Fatalf("起 HTTP 失败：%v", err)
	}
	t.Cleanup(func() { _ = server.Stop(t.Context()) })

	return &harness{t: t, layout: layout, app: app, store: store, server: server, port: port}
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
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("编码请求体失败：%v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
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

// mustOK 断言信封 code 为 "0" 并把 data 解进 out。
func (h *harness) mustOK(env envelope, httpCode int, what string, out any) {
	h.t.Helper()
	if httpCode != http.StatusOK {
		h.t.Fatalf("%s：HTTP 状态应为 200，实际 %d（body=%s）", what, httpCode, string(env.Data))
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

// waitFor 轮询一个条件直到成立或超时。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 %s 超时", what)
}

// keysOf 取 map 的键，用于失败信息。
func keysOf(data map[string]any) []string {
	out := make([]string, 0, len(data))
	for key := range data {
		out = append(out, key)
	}
	return out
}

// TestAuthGate 断言鉴权是硬门：HTTP 头与 SSE 查询参数两条路都要查。
// SSE 单独走 ?token= 是因为 EventSource 带不了自定义头，不能只测 Header 这条路。
func TestAuthGate(t *testing.T) {
	h := newHarness(t)

	// HTTP：无令牌 / 错令牌都回 401，且不带业务信封——那是根本没进门，不是业务失败。
	for _, tc := range []struct {
		name  string
		token string
	}{
		{"无令牌", ""},
		{"错令牌", "wrong-token"},
	} {
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
			t.Errorf("%s：401 不应带业务信封，实际 body=%s", tc.name, rec.Body.String())
		}
	}

	// SSE 有令牌：等首帧写出就断开，证明鉴权放行、帧能写出、断开能退出。
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?token="+testToken, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan int, 1)
	go func() {
		h.server.Handler().ServeHTTP(rec, req)
		done <- rec.Code
	}()
	waitFor(t, "SSE 首帧", func() bool {
		return strings.Contains(rec.Body.String(), "stream:open")
	})
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("断开后 SSE 处理器应退出")
	}

	// SSE 无令牌：同样 401。
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec = httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("SSE 无令牌应回 401，实际 %d", rec.Code)
	}
}

// TestEnvelopeContract 断言信封是唯一出口形状：成功与失败同构、HTTP 恒 200。
// 前端把一切响应当信封解析，未知路由与坏请求体也必须长这样。
func TestEnvelopeContract(t *testing.T) {
	h := newHarness(t)

	// 未知路由 → 1404，仍是信封且带可读 message。
	env, code := h.call(http.MethodGet, "/nope/not-here", nil)
	if code != http.StatusOK {
		t.Fatalf("未知路由应回 HTTP 200（业务失败靠信封），实际 %d", code)
	}
	if env.Code != pkg.CodeUnknownRoute || env.Message == "" {
		t.Fatalf("未知路由应回 %q 且带 message，实际 code=%q message=%q",
			pkg.CodeUnknownRoute, env.Code, env.Message)
	}

	// 坏请求体 → 1000，仍是信封。
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/save", bytes.NewReader([]byte(`{not json`)))
	req.Header.Set("X-Desktop-Token", testToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(rec, req)
	var bad envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &bad); err != nil {
		t.Fatalf("坏请求体也必须回信封，实际 body=%s", rec.Body.String())
	}
	if bad.Code != pkg.CodeBadRequest || rec.Code != http.StatusOK {
		t.Fatalf("坏请求体应回 %q / HTTP 200，实际 code=%q http=%d",
			pkg.CodeBadRequest, bad.Code, rec.Code)
	}

	// 成功路径同构：bootstrap 总包带 app_contract，前端首屏全靠它。
	env, code = h.call(http.MethodGet, "/bootstrap", nil)
	var data map[string]any
	h.mustOK(env, code, "bootstrap", &data)
	if _, ok := data["app_contract"]; !ok {
		t.Fatalf("bootstrap 缺少 app_contract；实际键=%v", keysOf(data))
	}
}

// TestSettingsChain 断言设置链路的两个不变量：
// KV 真落 SQLite（写经 HTTP、查直连库、读走 HTTP 三个方向都对上），
// 重复切工作区只留一条历史。
func TestSettingsChain(t *testing.T) {
	h := newHarness(t)

	// KV：HTTP 写入后绕过 service 直查库确认落盘，再走 HTTP 读回确认读路径。
	env, code := h.call(http.MethodPost, "/settings/save", map[string]string{"ui.theme": "dark"})
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

	// 工作区：不存在的目录归 1001；同一目录重复切换只留一条历史。
	env, _ = h.call(http.MethodPost, "/settings/workspace", map[string]string{
		"workspace": filepath.Join(h.layout.DataDir, "does-not-exist"),
	})
	if env.Code != pkg.CodeFileNotFound {
		t.Fatalf("不存在的目录应回 %q，实际 %q（%s）", pkg.CodeFileNotFound, env.Code, env.Message)
	}
	ws := filepath.Join(h.layout.DataDir, "ws-one")
	if err := pkg.EnsureDir(ws); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	for i := 0; i < 3; i++ {
		env, code := h.call(http.MethodPost, "/settings/workspace", map[string]string{"workspace": ws})
		var data struct {
			Workspace string `json:"workspace"`
		}
		h.mustOK(env, code, "切工作区", &data)
		if data.Workspace == "" {
			t.Fatal("切工作区应回非空 workspace")
		}
	}
	rows := h.app.Settings.Workspaces()
	if len(rows) != 1 || rows[0].WorkspacePath != ws {
		t.Fatalf("重复切换应只留 1 条且为 %s，实际 %+v", ws, rows)
	}
}

// TestWorkspaceRenameGuards 断言两件会让数据变脏的事：
// 工作区清单带上 git 与会话概况（工作区页靠它决定显示什么），
// 以及引擎正打开着的会话不允许改名（两路同时追加会写坏唯一真相）。
func TestWorkspaceRenameGuards(t *testing.T) {
	h := newHarness(t)

	ws := filepath.Join(h.layout.DataDir, "ws-guard")
	if err := pkg.EnsureDir(ws); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if _, err := h.app.Settings.SetWorkspace(ws); err != nil {
		t.Fatalf("切工作区失败：%v", err)
	}

	// 清单接口要能用，且把仓库状态与会话数一并带上。
	env, code := h.call(http.MethodGet, "/workspaces", nil)
	var list struct {
		Workspace     string `json:"workspace"`
		WorkspaceRows []struct {
			WorkspacePath   string `json:"workspace_path"`
			WorkspaceExists bool   `json:"workspace_exists"`
			WorkspaceIsRepo bool   `json:"workspace_is_repo"`
			SessionCount    int    `json:"session_count"`
		} `json:"workspace_rows"`
	}
	h.mustOK(env, code, "工作区清单", &list)
	if list.Workspace != ws {
		t.Fatalf("清单应回当前工作区 %s，实际 %q", ws, list.Workspace)
	}
	if len(list.WorkspaceRows) != 1 || !list.WorkspaceRows[0].WorkspaceExists {
		t.Fatalf("清单应含 1 条且目录存在，实际 %+v", list.WorkspaceRows)
	}
	if list.WorkspaceRows[0].WorkspaceIsRepo {
		t.Fatal("空目录不是 git 仓库，is_repo 应为 false")
	}

	// 引擎正打开这份会话时改名必须被拦下，且文件不被追加任何东西。
	// 会话文件必须落在会话根内：路径越界本身就会先被拦掉，那就测不到改名保护了。
	sessDir := filepath.Join(h.layout.SessionsDir(), "ws-guard")
	if err := pkg.EnsureDir(sessDir); err != nil {
		t.Fatalf("建会话目录失败：%v", err)
	}
	file := filepath.Join(sessDir, "sess_guard.jsonl")
	body := `{"type":"session","version":3,"id":"sess_guard","cwd":"C:\\ws","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"内容"}}` + "\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatalf("写会话文件失败：%v", err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读会话文件失败：%v", err)
	}
	h.app.Sessions.Active = func() string { return file }
	env, _ = h.call(http.MethodPost, "/sessions/rename", map[string]string{
		"session_path": file, "session_title": "新名字",
	})
	if env.Code != pkg.CodeEngineBusy {
		t.Fatalf("引擎正打开时改名应回 %q，实际 %q（%s）", pkg.CodeEngineBusy, env.Code, env.Message)
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("重读会话文件失败：%v", err)
	}
	if string(before) != string(after) {
		t.Fatal("被拒的改名不得动会话文件一个字节")
	}

	// 引擎没在用它时改名要成功，并且是以追加 session_info 的方式。
	h.app.Sessions.Active = func() string { return "" }
	env, code = h.call(http.MethodPost, "/sessions/rename", map[string]string{
		"session_path": file, "session_title": "新名字",
	})
	h.mustOK(env, code, "改名", nil)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读改名后的会话文件失败：%v", err)
	}
	if !strings.Contains(string(raw), `"name":"新名字"`) {
		t.Fatalf("改名应以 session_info 条目追加，实际 %q", string(raw))
	}
	if !strings.HasPrefix(string(raw), string(before)) {
		t.Fatal("改名不得重写已有内容，只允许在末尾追加")
	}
}
