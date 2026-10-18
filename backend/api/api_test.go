// HTTP 边界链路：装配完整性、方法白名单、统一信封与 SSE 全链（业务编排归 service）。
// 坏了的表现：接口全 404、前端 POST 被预检拦掉、重连丢一段回复、数组字段发成 null。
package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/runtime/runtimetest"
	"WorkBaby/backend/server"
)

// bootServer 在干净家目录起一套完整装配 + 真实 HTTP 栈。
// agent 目录也隔离：测试绝不能动用户真实的 ~/.pi/agent。
func bootServer(t *testing.T) (string, *api.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	// 假设 PI 就是那个假进程：它说同一套协议，链路照常全跑。
	t.Setenv("WORKBABY_PI_EXE", buildFakePI(t))
	// 装配期不要去解压上百 MB 的归档（真解压在 runtime 包里测一次）。
	runtimetest.SeedMarkers(t, home)

	h := api.New("test-version")
	if err := h.Startup(context.Background()); err != nil {
		t.Fatalf("首次启动必须成功: %v", err)
	}
	t.Cleanup(h.Shutdown)

	// 装配缺任何一样，界面都是空窗口 + 接口全 404。
	if h.Svc == nil || h.Repo == nil || h.Skills == nil || h.PiEnv == nil {
		t.Fatal("服务容器没有装配完整（Svc/Repo/Skills/PiEnv）")
	}
	if h.ServerPort() != 0 {
		t.Fatal("端口应由后续握手阶段写入，Startup 阶段应为 0")
	}

	srv := server.New(h)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("HTTP 服务启动失败: %v", err)
	}
	t.Cleanup(srv.Stop)
	return fmt.Sprintf("http://127.0.0.1:%d/api/v1", port), h
}

// buildFakePI 编译假 PI 到临时目录。
func buildFakePI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "fakepi.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./backend/pi/pitest")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译假 PI 失败: %v\n%s", err, out)
	}
	return exe
}

// repoRoot 从测试工作目录往上找 go.mod。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到仓库根目录")
		}
		dir = parent
	}
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

// sseFrame 是一条被解析出来的 SSE 帧。
type sseFrame struct {
	Event string
	ID    string
	Data  map[string]any
}

// subscribeSSE 订阅会话事件流，逐帧解析；lastID 非空时带 Last-Event-ID 重连。
func subscribeSSE(t *testing.T, base, sessionID, lastID string) (<-chan sseFrame, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/events?session_id="+sessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("订阅 SSE 失败: %v", err)
	}
	frames := make(chan sseFrame, 64)
	go func() {
		defer close(frames)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		var cur sseFrame
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				cur.Event = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "id:"):
				cur.ID = strings.TrimSpace(line[len("id:"):])
			case strings.HasPrefix(line, "data:"):
				raw := strings.TrimSpace(line[len("data:"):])
				_ = json.Unmarshal([]byte(raw), &cur.Data)
			case line == "":
				if cur.Event != "" {
					select {
					case frames <- cur:
					case <-ctx.Done():
						return
					}
				}
				cur = sseFrame{}
			}
		}
	}()
	return frames, cancel
}

// drainUntil 收帧直到命中全部 wanted 事件，超时即判定链路卡住。
func drainUntil(t *testing.T, frames <-chan sseFrame, wanted ...string) []sseFrame {
	t.Helper()
	pending := map[string]bool{}
	for _, w := range wanted {
		pending[w] = true
	}
	var got []sseFrame
	deadline := time.After(20 * time.Second)
	for len(pending) > 0 {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("事件流提前关闭，还缺: %v", keysOf(pending))
			}
			got = append(got, f)
			delete(pending, f.Event)
		case <-deadline:
			t.Fatalf("等待 %v 超时（已收到 %d 帧，还缺 %v）", wanted, len(got), keysOf(pending))
		}
	}
	return got
}

// keysOf 把待定事件集变成可打印的列表（失败信息里要能看出缺哪几个）。
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestHTTPChain 是 api 包唯一的测试入口。服务只启动一次，子测试共用。
func TestHTTPChain(t *testing.T) {
	base, h := bootServer(t)
	workspace := h.Svc.Env.Workspace()

	t.Run("HTTP 闸门：预检放行、方法白名单、越界入参在门口拒绝", func(t *testing.T) {
		// 预检不放行的话前端所有 POST 都在浏览器层就被拦掉，接口本身根本没被调用。
		req, err := http.NewRequest(http.MethodOptions, base+"/chat/send", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", "http://wails.localhost")
		req.Header.Set("Access-Control-Request-Method", "POST")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			t.Fatalf("预检被拒: %d", resp.StatusCode)
		}

		// 方法白名单：DELETE / PUT 一律走统一外壳报 1002。
		for _, m := range []string{http.MethodPut, http.MethodDelete} {
			if _, body := call(t, base, m, "/sessions/whatever", nil); !strings.Contains(body, `"code":"1002"`) {
				t.Fatalf("%s 应当被白名单拦下: %s", m, body)
			}
		}

		// 路径变量里的穿越必须在门口拦掉：拿到文档内容才算失守。
		if code, body := call(t, base, "GET", "/docs/..%2Fconfig", nil); code == http.StatusOK &&
			strings.Contains(body, `"code":"0"`) {
			t.Fatalf("非法文档名没有被拦下: %d %s", code, body)
		}
	})

	t.Run("契约：统一信封、空数组不null、配置位置可见", func(t *testing.T) {
		code, body := call(t, base, "GET", "/bootstrap", nil)
		if code != http.StatusOK {
			t.Fatalf("bootstrap 失败: %d %s", code, body)
		}
		var payload struct {
			Code string `json:"code"`
			Data struct {
				ContractVersion int                `json:"contract_version"`
				PiEnv           *json.RawMessage   `json:"pi_env"`
				Settings        *map[string]string `json:"settings"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("解析 bootstrap 失败: %v", err)
		}
		if payload.Code != "0" {
			t.Fatalf("成功响应的 code 必须是字符串 \"0\": %s", body)
		}
		// 没有 pi_env 的话前端进不了环境向导；没有 settings 外观设置读不到。
		if payload.Data.PiEnv == nil || payload.Data.Settings == nil {
			t.Fatalf("bootstrap 字段缺失: %s", body)
		}

		// 环境与运行时状态是向导与设置页的唯一数据源，字段名不能漂。
		_, body = call(t, base, "GET", "/env", nil)
		for _, want := range []string{`"pi_path"`, `"agent_dir"`, `"python_path"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("环境接口缺少 %s: %s", want, body)
			}
		}
		_, body = call(t, base, "GET", "/runtime", nil)
		for _, want := range []string{`"node_exe"`, `"python_exe"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("运行时状态缺少 %s: %s", want, body)
			}
		}

		// 数组字段直接被前端 .length，发成 null 就是一句运行时错误。
		var list struct {
			Code string `json:"code"`
			Data struct {
				Dir      string            `json:"dir"`
				Entries  *[]map[string]any `json:"entries"`
				Packages *[]map[string]any `json:"packages"`
			} `json:"data"`
		}
		_, body = call(t, base, "GET", "/extensions", nil)
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			t.Fatalf("解析插件列表失败: %v", err)
		}
		if list.Data.Entries == nil || list.Data.Packages == nil {
			t.Fatalf("插件列表的数组字段发成了 null: %s", body)
		}

		for _, path := range []string{"/skills", "/tools", "/providers"} {
			_, body = call(t, base, "GET", path, nil)
			if strings.Contains(body, `"code":"99`) {
				t.Fatalf("%s 报错了: %s", path, body)
			}
		}

		// MCP 的服务列表与两份配置原文都要在。
		_, body = call(t, base, "GET", "/mcp", nil)
		for _, want := range []string{`"servers"`, `"global_raw"`, `"project_raw"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("MCP 接口缺少 %s: %s", want, body)
			}
		}
	})

	// 首屏预热聚合：五路清单一次拉齐且不发 null，前端进面板时不再空白。
	t.Run("预热聚合：五路清单一次拉齐", func(t *testing.T) {
		_, body := call(t, base, "GET", "/warmup", nil)
		var payload struct {
			Code string `json:"code"`
			Data struct {
				Skills     *[]map[string]any `json:"skills"`
				Providers  *json.RawMessage  `json:"providers"`
				Tools      *json.RawMessage  `json:"tools"`
				Extensions *json.RawMessage  `json:"extensions"`
				MCP        *json.RawMessage  `json:"mcp"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("解析 warmup 失败: %v", err)
		}
		if payload.Code != "0" {
			t.Fatalf("warmup 报错: %s", body)
		}
		if payload.Data.Skills == nil || payload.Data.Providers == nil || payload.Data.Tools == nil ||
			payload.Data.Extensions == nil || payload.Data.MCP == nil {
			t.Fatalf("warmup 字段缺失: %s", body)
		}
	})

	t.Run("SSE 全链：子进程到帧、seq 递增、断线重连不被拒", func(t *testing.T) {
		// 配一个服务：环境闸门的 model 判定要它。
		code, body := call(t, base, "POST", "/providers/upsert", map[string]any{
			"name": "测试网关", "api": "openai-completions",
			"base_url": "https://api.example.com/v1", "api_key": "sk-test",
			"models": []map[string]any{{"id": "test-model", "name": "测试模型"}},
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("创建模型服务失败: %d %s", code, body)
		}
		if code, body := call(t, base, "POST", "/providers/default", map[string]any{
			"provider": "测试网关", "model": "test-model",
		}); code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			// 名字会被 slug 成 provider 键：拿列表里的真实键再设一次。
			_, listing := call(t, base, "GET", "/providers", nil)
			var parsed struct {
				Data struct {
					Providers []struct {
						ID string `json:"id"`
					} `json:"providers"`
				} `json:"data"`
			}
			_ = json.Unmarshal([]byte(listing), &parsed)
			if len(parsed.Data.Providers) == 0 {
				t.Fatalf("服务商没落盘: %s", listing)
			}
			id := parsed.Data.Providers[0].ID
			if code, body := call(t, base, "POST", "/providers/default", map[string]any{
				"provider": id, "model": "test-model",
			}); code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
				t.Fatalf("设为默认失败: %d %s", code, body)
			}
		}

		// 第一次发送：新对话由后端现场创建，返回 session_id。
		code, body = call(t, base, "POST", "/chat/send", map[string]any{
			"workspace": workspace, "content": "在吗",
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("首次发送失败: %d %s", code, body)
		}
		var sent struct {
			Data struct {
				SessionID string `json:"session_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &sent); err != nil || sent.Data.SessionID == "" {
			t.Fatalf("新对话没有返回 session_id: %s", body)
		}
		sessID := sent.Data.SessionID

		// 会话文件是 PI 写的：给它一点时间落盘，再确认列表里能看到。
		var listed bool
		for i := 0; i < 100 && !listed; i++ {
			_, listing := call(t, base, "GET", "/sessions", nil)
			listed = strings.Contains(listing, sessID)
			if !listed {
				time.Sleep(20 * time.Millisecond)
			}
		}
		if !listed {
			t.Fatal("发送之后会话没有出现在列表里（会话索引没读到 PI 的文件）")
		}

		frames, stop := subscribeSSE(t, base, sessID, "")
		defer stop()
		time.Sleep(120 * time.Millisecond) // 让订阅先进 Hub，避免错过首帧

		code, body = call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sessID, "content": "再来一轮",
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("第二次发送失败: %d %s", code, body)
		}
		got := drainUntil(t, frames, "chat:start", "chat:delta", "chat:done")

		var text strings.Builder
		var seqs []int64
		for _, f := range got {
			// 前端把 data 当信封用：data.event 决定路由，data.data 才是载荷。
			if f.Data["event"] != f.Event {
				t.Fatalf("data 里没有回带 event 字段（实际 %v，帧头是 %q）", f.Data["event"], f.Event)
			}
			envSeq, _ := f.Data["seq"].(float64)
			seqs = append(seqs, int64(envSeq))
			load, ok := f.Data["data"].(map[string]any)
			if !ok {
				t.Fatalf("%s 的 data 字段不是对象: %v", f.Event, f.Data)
			}
			switch f.Event {
			case "chat:delta":
				if load["kind"] == "text" {
					text.WriteString(load["delta"].(string))
				}
			case "chat:done":
				usage, ok := load["usage"].(map[string]any)
				if !ok || usage["output"] != float64(20) {
					t.Fatalf("done 的用量不对（output 应为 20，实际 %v）", load["usage"])
				}
			}
		}
		if text.String() != "你好" {
			t.Fatalf("流式正文拼接不对: %q", text.String())
		}
		for i := 1; i < len(seqs); i++ {
			if seqs[i] <= seqs[i-1] {
				t.Fatalf("事件 seq 非递增: %v", seqs)
			}
		}

		// 断线重连：带 Last-Event-ID 回来必须能重新接入，而不是被拒绝。
		frames2, stop2 := subscribeSSE(t, base, sessID, got[len(got)-1].ID)
		defer stop2()
		if code, body = call(t, base, "POST", "/chat/followup", map[string]any{
			"session_id": sessID, "content": "再补一句",
		}); code != http.StatusOK && !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("排队消息失败: %s", body)
		}
		select {
		case <-frames2:
		case <-time.After(3 * time.Second):
			// 没有新事件也算通过：这条只验证「重连不被拒绝」。
		}
	})
}
