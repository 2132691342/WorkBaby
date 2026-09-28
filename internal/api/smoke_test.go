package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/internal/api"
	"WorkBaby/internal/server"
)

// 真实 HTTP 栈全端点冒烟：设置页全部读写 + 跨源预检。
// 曾经打包版所有 POST 都变成 Network Error（预检被 methodGuard 拦下且无 CORS 头），
// Go 的 http 客户端不发预检，所以普通单测永远测不出来——这条测试模拟浏览器行为。

func TestSettingsEndpointsSmoke(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)

	h := api.New("test")
	if err := h.Startup(context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(h.Shutdown)

	srv := server.New(h)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(srv.Stop)
	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)

	call := func(method, path string, body any) string {
		var rd io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rd = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, base+path, rd)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		// 模拟真实 WebView2 来源，走一遍 CORS
		req.Header.Set("Origin", "http://wails.localhost")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "TRANSPORT-ERROR: " + err.Error()
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("%d %s", resp.StatusCode, string(raw))
	}

	t.Run("endpoints", func(t *testing.T) {
		t.Logf("GET  /skills              -> %s", call("GET", "/skills", nil))
		t.Logf("GET  /sessions            -> %s", call("GET", "/sessions", nil))
		t.Logf("GET  /providers           -> %s", call("GET", "/providers", nil))
		t.Logf("GET  /knowledge/docs      -> %s", call("GET", "/knowledge/docs", nil))
		t.Logf("GET  /settings            -> %s", call("GET", "/settings", nil))
		t.Logf("GET  /stats               -> %s", call("GET", "/stats", nil))
		t.Logf("GET  /bootstrap           -> %s", call("GET", "/bootstrap", nil))
		t.Logf("GET  /approvals           -> %s", call("GET", "/approvals", nil))
		t.Logf("POST /sessions            -> %s", call("POST", "/sessions", map[string]any{}))
		t.Logf("POST /skills/create-skill@builtin/toggle -> %s", call("POST", "/skills/create-skill@builtin/toggle", map[string]any{"enabled": false}))
		t.Logf("POST /skills (create)     -> %s", call("POST", "/skills", map[string]any{"name": "demo-skill", "description": "d", "body": "## 怎么做\n1. x"}))

		// 导入：准备一个真实 SKILL.md
		dir := filepath.Join(home, "import-src", "my-imported")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "SKILL.md"),
			[]byte("---\nname: my-imported\ndescription: 测试导入\n---\n\n正文\n"), 0o644)
		r := call("POST", "/skills/import", map[string]any{"paths": []string{dir}})
		if !bytes.Contains([]byte(r), []byte(`"imported":1`)) {
			t.Fatalf("导入失败: %s", r)
		}
		t.Logf("POST /skills/import       -> %s", r)

		t.Logf("POST /knowledge/docs/add  -> %s", call("POST", "/knowledge/docs/add", map[string]any{"paths": []string{""}}))
		t.Logf("POST /knowledge/search    -> %s", call("POST", "/knowledge/search", map[string]any{"query": "x"}))
		t.Logf("POST /settings            -> %s", call("POST", "/settings", map[string]any{"key": "persona", "value": "v"}))
	})

	// 预检回归：浏览器跨源 POST 前必须拿到带 CORS 头的 204，
	// 否则前端所有写操作都会变成 Network Error。
	t.Run("preflight", func(t *testing.T) {
		req, _ := http.NewRequest("OPTIONS", base+"/skills", nil)
		req.Header.Set("Origin", "http://wails.localhost")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("预检传输失败: %v", err)
		}
		defer resp.Body.Close()
		acao := resp.Header.Get("Access-Control-Allow-Origin")
		if resp.StatusCode != http.StatusNoContent || acao != "http://wails.localhost" {
			t.Fatalf("预检被拦：status=%d ACAO=%q（POST 会全部变成 Network Error）", resp.StatusCode, acao)
		}
	})
}
