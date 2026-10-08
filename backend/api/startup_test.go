// 从零装配 → 真实 HTTP 栈的整条启动链路：装配完整性、磁盘导入技能、跨源预检。
// Go 的 http 客户端不发预检，只有这里模拟浏览器行为才测得出来 CORS 缺失
// （症状是打包版所有 POST 报 Network Error）。

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
	"strings"
	"testing"

	"WorkBaby/backend/api"
	"WorkBaby/backend/runtime/runtimetest"
	"WorkBaby/backend/server"
)

// newBootedServer 在干净家目录起一套完整装配 + 真实 HTTP 栈，返回 base URL。
// 运行时标记先预置好：配置 / DB / 服务 / 工具注册全链路照常跑，只跳过归档解压。
func newBootedServer(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
	runtimetest.SeedMarkers(t, home)

	h := api.New("test")
	if err := h.Startup(context.Background()); err != nil {
		t.Fatalf("首次启动必须成功: %v", err)
	}
	t.Cleanup(h.Shutdown)

	if h.Svc == nil || h.Repo == nil || h.Registry == nil || h.Skills == nil {
		t.Fatal("服务容器没有装配完整，界面会空着且接口全 404")
	}
	if h.Cfg.MasterKey == "" {
		t.Fatal("配置没有生成主密钥")
	}
	if len(h.Skills.List()) == 0 {
		t.Fatal("内置技能没有加载")
	}
	if got := len(h.Registry.All()); got == 0 {
		t.Fatal("内置工具一个都没注册，模型会突然找不到文件能力")
	}
	for _, name := range []string{"read", "write"} {
		if _, found := h.Registry.Get(name); !found {
			t.Fatalf("%s 工具没有注册", name)
		}
	}
	// 端口握手必须早于 domReady 写入，否则前端拿不到端口，表现为所有接口 404。
	if h.Port() != 0 {
		t.Fatal("端口应由 domReady 阶段写入，Startup 阶段应为 0")
	}

	srv := server.New(h)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("HTTP 服务启动失败: %v", err)
	}
	t.Cleanup(srv.Stop)
	return fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)
}

// call 发一个带 WebView2 来源的请求，返回状态码与响应体。
func call(t *testing.T, base, method, path string, body any) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "http://wails.localhost")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "TRANSPORT-ERROR: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// 启动链路：导入与 CORS 共用一个装配好的进程，验证「从零起来就能用」。
func TestStartupChain(t *testing.T) {
	base := newBootedServer(t)

	// 导入技能要真的把磁盘上的 SKILL.md 收进注册表。
	t.Run("从磁盘导入技能", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-imported")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: my-imported\ndescription: 测试导入\n---\n\n正文\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		code, resp := call(t, base, "POST", "/skills/import", map[string]any{"paths": []string{dir}})
		if code != http.StatusOK || !strings.Contains(resp, `"imported":1`) {
			t.Fatalf("导入失败: %d %s", code, resp)
		}
		if code, resp := call(t, base, "GET", "/skills/my-imported/content", nil); code != http.StatusOK || !strings.Contains(resp, "正文") {
			t.Fatalf("导入后取不到正文: %d %s", code, resp)
		}
	})

	// 浏览器跨源 POST 前必须拿到带 CORS 头的 204，否则前端所有写操作都是 Network Error。
	t.Run("跨源预检放行 POST", func(t *testing.T) {
		req, err := http.NewRequest("OPTIONS", base+"/skills", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "http://wails.localhost")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("预检传输失败: %v", err)
		}
		defer resp.Body.Close()
		if acao := resp.Header.Get("Access-Control-Allow-Origin"); resp.StatusCode != http.StatusNoContent || acao != "http://wails.localhost" {
			t.Fatalf("预检被拦：status=%d ACAO=%q（POST 会全部变成 Network Error）", resp.StatusCode, acao)
		}
	})
}
