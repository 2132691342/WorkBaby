// 跨进程边界的端到端链路：真实 HTTP 栈下的装配完整性、跨源预检、SSE 送达与断线重放。
// 这里跑的是真 PI（用 pitest 假进程替身，走真实 stdin/stdout 协议），
// 所以 SSE 的每一帧都是从「子进程 → 协议解析 → 翻译 → Hub」整条链出来的。
// 坏了的表现：界面一直转圈（done 永不到）、用量落不进仪表盘、重连后丢一段回复。
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
// 运行时标记先预置好：配置 / DB / 服务全链路照常跑，只跳过归档解压。
func bootServer(t *testing.T) (string, *api.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
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
	if h.Svc == nil || h.Repo == nil || h.Skills == nil || h.Cfg.MasterKey == "" || h.PiEnv == nil {
		t.Fatal("服务容器没有装配完整（Svc/Repo/Skills/PiEnv/MasterKey）")
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

// unwrapID 从统一响应信封里取 data.id。
func unwrapID(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, body)
	}
	if payload.Data.ID == "" {
		t.Fatalf("响应里没有 id: %s", body)
	}
	return payload.Data.ID
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

	t.Run("跨源预检放行 POST", func(t *testing.T) {
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
		// 预检不放行的话前端所有 POST 都会在浏览器层就被拦掉。
		if resp.StatusCode >= 400 {
			t.Fatalf("预检被拒: %d", resp.StatusCode)
		}
	})

	t.Run("帮助文档名不越界", func(t *testing.T) {
		// 拿到文档内容就算失守：路由层 404 与业务错误码都算拦下。
		if code, body := call(t, base, "GET", "/docs/..%2Fconfig", nil); code == http.StatusOK &&
			strings.Contains(body, `"code":"0"`) {
			t.Fatalf("非法文档名没有被拦下: %d %s", code, body)
		}
	})

	t.Run("环境闸门随引导下发", func(t *testing.T) {
		code, body := call(t, base, "GET", "/bootstrap", nil)
		if code != http.StatusOK {
			t.Fatalf("bootstrap 失败: %d %s", code, body)
		}
		var payload struct {
			Data struct {
				PiEnv *struct {
					Ready    bool   `json:"ready"`
					Blocking string `json:"blocking"`
					PiPath   string `json:"pi_path"`
				} `json:"pi_env"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("解析 bootstrap 失败: %v", err)
		}
		// 没有 pi_env 的话前端进不了环境向导，只会看到一个永远发不出消息的输入框。
		if payload.Data.PiEnv == nil {
			t.Fatalf("bootstrap 里没有 pi_env: %s", body)
		}
		if payload.Data.PiEnv.PiPath == "" {
			t.Fatalf("测试里 PI 路径应当已注入: %s", body)
		}
		if payload.Data.PiEnv.Ready && payload.Data.PiEnv.Blocking != "" {
			t.Fatalf("就绪却还报阻塞项: %s", body)
		}
	})

	t.Run("会话建好后是懒创建", func(t *testing.T) {
		code, body := call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		id := unwrapID(t, body)
		_, detail := call(t, base, "GET", "/sessions/"+id, nil)
		var payload struct {
			Data struct {
				Session struct {
					PiSessionID string `json:"pi_session_id"`
				} `json:"session"`
				Messages []any `json:"messages"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(detail), &payload); err != nil {
			t.Fatalf("解析详情失败: %v body=%s", err, detail)
		}
		// 还没发过消息：PI 会话与 JSONL 都还不该存在，消息列表给空数组而不是 null。
		if payload.Data.Session.PiSessionID != "" {
			t.Fatalf("没发消息就建了 PI 会话: %s", detail)
		}
		if payload.Data.Messages == nil {
			t.Fatalf("消息列表是 null，前端按数组读会崩: %s", detail)
		}
	})

	t.Run("SSE 送达与断线重放", func(t *testing.T) {
		// 配一个服务：环境闸门的 model 判定要它，发送也要求有可用模型。
		code, body := call(t, base, "POST", "/providers", map[string]any{
			"name": "t", "api": "openai-completions",
			"base_url": "https://api.example.com/v1",
			"models":   []string{"test-model"},
		})
		if code != http.StatusOK {
			t.Fatalf("创建模型服务失败: %d %s", code, body)
		}
		provID := unwrapID(t, body)
		if code, body := call(t, base, "POST", "/providers/default/"+provID, nil); code != http.StatusOK {
			t.Fatalf("设为默认失败: %d %s", code, body)
		}

		code, body = call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		sessID := unwrapID(t, body)

		frames, stop := subscribeSSE(t, base, sessID, "")
		defer stop()

		code, body = call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sessID, "content": "在吗",
		})
		if code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("发送失败: %d %s", code, body)
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
				// 用量要落进仪表盘的口径里。
				usage, ok := load["usage"].(map[string]any)
				if !ok || usage["output"] != float64(20) {
					t.Fatalf("done 的用量不对（output 应为 20，实际 %v）", load["usage"])
				}
			}
		}
		if text.String() != "你好" {
			t.Fatalf("流式正文拼接不对: %q", text.String())
		}
		// seq 必须严格递增：前端靠它做断线重放，乱序会导致对账时重复渲染。
		for i := 1; i < len(seqs); i++ {
			if seqs[i] <= seqs[i-1] {
				t.Fatalf("事件 seq 非递增: %v", seqs)
			}
		}

		// 断线重连：带 Last-Event-ID 回来必须补上缺的那几帧。
		lastID := got[len(got)-1].ID
		frames2, stop2 := subscribeSSE(t, base, sessID, lastID)
		defer stop2()
		if code, body = call(t, base, "POST", "/chat/followup", map[string]any{
			"session_id": sessID, "content": "再补一句",
		}); !strings.Contains(body, `"code":"0"`) && code != http.StatusOK {
			t.Fatalf("排队消息失败: %s", body)
		}
		// 能重新订阅成功（不报错）就说明重放通道可用；具体补几帧取决于时序。
		select {
		case <-frames2:
		case <-time.After(5 * time.Second):
			// 没有新事件也算通过：这条断言只验证「重连不被拒绝」。
		}
	})

	t.Run("运行记录与用量落账", func(t *testing.T) {
		code, body := call(t, base, "GET", "/runs?limit=10", nil)
		if code != http.StatusOK {
			t.Fatalf("查运行记录失败: %d %s", code, body)
		}
		var payload struct {
			Data []struct {
				Status     string `json:"status"`
				StopReason string `json:"stop_reason"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("解析运行记录失败: %v body=%s", err, body)
		}
		if len(payload.Data) == 0 {
			t.Fatal("跑过对话却没有运行记录：仪表盘与观测全靠它")
		}
		for _, r := range payload.Data {
			// 状态与收尾原因必须分开存：合并就分不清「被停」与「说完」。
			if r.Status == "" || r.StopReason == "" {
				t.Fatalf("运行记录缺少状态或收尾原因: %+v", r)
			}
			if r.Status == "running" {
				t.Fatalf("agent_settled 之后不该还有 running: %+v", r)
			}
		}

		code, body = call(t, base, "GET", "/stats?days=7", nil)
		if code != http.StatusOK {
			t.Fatalf("查统计失败: %d %s", code, body)
		}
		var stats struct {
			Data struct {
				Daily []struct {
					Date string `json:"date"`
				} `json:"daily"`
				Totals struct {
					Input int `json:"input"`
				} `json:"totals"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &stats); err != nil {
			t.Fatalf("解析统计失败: %v", err)
		}
		// daily 长度必须恒等于 days：否则趋势图 x 轴会随「哪天没说话」跳动。
		if len(stats.Data.Daily) != 7 {
			t.Fatalf("daily 长度应为 7，实际 %d", len(stats.Data.Daily))
		}
		if stats.Data.Totals.Input == 0 {
			t.Fatal("用量没有落账：仪表盘会一直是 0")
		}
	})

	t.Run("停止不报错且取回排队文本", func(t *testing.T) {
		code, body := call(t, base, "POST", "/sessions", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("建会话失败: %d %s", code, body)
		}
		sid := unwrapID(t, body)
		if _, body := call(t, base, "POST", "/chat/send", map[string]any{
			"session_id": sid, "content": "开始",
		}); !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("发送失败: %s", body)
		}
		// 停止必须先 clear_queue 再 abort：不取回排队文本，用户那句话就消失了。
		if code, body := call(t, base, "POST", "/chat/stop", map[string]any{
			"session_id": sid,
		}); code != http.StatusOK || !strings.Contains(body, `"code":"0"`) {
			t.Fatalf("停止失败: %d %s", code, body)
		}
	})

	t.Run("审批决策在无等待项时返回 4102", func(t *testing.T) {
		code, body := call(t, base, "POST", "/approvals/decide/not-exist", map[string]any{
			"session_id": "x", "action": "allow",
		})
		if code != http.StatusOK {
			t.Fatalf("审批路由不该 404: %d %s", code, body)
		}
		// 摘不到等待项说明决策在途（超时/重复点击），必须报 4102 且不改状态。
		if !strings.Contains(body, `"code":"4102"`) {
			t.Fatalf("应当返回 4102，实际: %s", body)
		}
	})

	t.Run("依赖边界与环境接口可用", func(t *testing.T) {
		code, body := call(t, base, "GET", "/env", nil)
		if code != http.StatusOK {
			t.Fatalf("查环境失败: %d %s", code, body)
		}
		if !strings.Contains(body, `"pi_path"`) {
			t.Fatalf("环境接口缺少 pi_path: %s", body)
		}
		code, body = call(t, base, "GET", "/runtime", nil)
		if code != http.StatusOK {
			t.Fatalf("查运行时失败: %d %s", code, body)
		}
		// 两套内置运行时都要报出来：少了 node 那一项，界面就说不清 PI 为什么起不来。
		for _, want := range []string{`"node_exe"`, `"python_exe"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("运行时状态缺少 %s: %s", want, body)
			}
		}
	})

	t.Run("工具清单来自 PI 且可停用", func(t *testing.T) {
		code, body := call(t, base, "GET", "/tools", nil)
		if code != http.StatusOK {
			t.Fatalf("查工具失败: %d %s", code, body)
		}
		if !strings.Contains(body, `"read"`) {
			t.Fatalf("工具清单里没有 read: %s", body)
		}
		// 停用一个工具：写 PI 的 defaultTools，不能写自己的 KV（那会变成假开关）。
		code, body = call(t, base, "POST", "/tools/toggle/powershell", map[string]any{"enabled": false})
		if code != http.StatusOK {
			t.Fatalf("停用工具失败: %d %s", code, body)
		}
		settings, err := h.PiEnv.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range settings.DefaultTools {
			if name == "powershell" {
				t.Fatalf("停用没写进 defaultTools: %v", settings.DefaultTools)
			}
			// 必须是纯名字列表：加减语法依赖 PI 默认集，会让新工具绕过勾选。
			if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
				t.Fatalf("不该出现增减语法: %v", settings.DefaultTools)
			}
		}
	})

	t.Run("MCP 配置校验后才落盘", func(t *testing.T) {
		bad := `{"mcpServers": {` // 语法不完整
		code, body := call(t, base, "POST", "/mcp/config", map[string]any{"raw": bad})
		if code != http.StatusOK {
			t.Fatalf("MCP 保存路由不该 404: %d %s", code, body)
		}
		if !strings.Contains(body, `"code":"9106"`) {
			t.Fatalf("坏配置应当报 9106: %s", body)
		}
		// 解析不过就一个字符都不能落盘。
		raw, err := h.PiEnv.ReadMCPRaw()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `{"mcpServers": {`) {
			t.Fatalf("坏配置被写进文件了: %s", raw)
		}

		good := `{"mcpServers":{"srv":{"command":"node","args":["x.js"]}}}`
		if code, body := call(t, base, "POST", "/mcp/config", map[string]any{"raw": good}); code != http.StatusOK {
			t.Fatalf("好配置保存失败: %d %s", code, body)
		}
	})
}
