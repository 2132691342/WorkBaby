// 公共测试装具：满装后端 harness（真 JSON 存储 + gin 中间件栈）、轻量 catalog harness、
// 按脚本吐分片的假上游、SSE 帧捕获，以及工作区/会话文件的助手。所有主题文件共用这一份。
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/api"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

const testToken = "test-token"

// harness 是装好的后端：真 JSON 存储、真临时布局、真 gin 中间件栈。
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
	layout := pkg.ResolveLayout(dir, filepath.Join(dir, "assets"), true)
	layout.DataDir = filepath.Join(dir, "data")
	layout.SessionsDir = filepath.Join(dir, "data", "sessions")
	layout.SkillsDir = filepath.Join(dir, "data", "skills")
	layout.SkillsOffDir = filepath.Join(dir, "data", "skills_disabled")
	layout.TemplatesDir = filepath.Join(dir, "data", "templates")
	layout.LogDir = filepath.Join(dir, "logs")
	layout.StorePath = filepath.Join(dir, "workbaby.json")
	if err := pkg.EnsureDir(layout.DataDir); err != nil {
		t.Fatalf("准备目录失败：%v", err)
	}
	pkg.SetLogDir("")

	store, err := repo.Open(layout.StorePath)
	if err != nil {
		t.Fatalf("开设置存储失败：%v", err)
	}

	hub := sse.New(64)
	db, err := repo.OpenDB(layout.DatabasePath())
	if err != nil {
		t.Fatalf("开数据库失败：%v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := service.New(service.Options{Layout: layout, Store: store, DB: db, Hub: hub, Version: "test"})
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

// repoAssetsDir 定位仓库里的 assets 目录（测试从 tests/ 跑，往上一级）。
func repoAssetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "assets"))
	if err != nil {
		t.Fatalf("解析资产目录失败：%v", err)
	}
	if !pkg.IsDir(filepath.Join(dir, "skills")) {
		t.Fatalf("资产目录里没有 skills：%s", dir)
	}
	return dir
}

// firstRecommendedSkill 取推荐清单里的第一个技能（测试需要任意一个真实技能名）。
func firstRecommendedSkill(t *testing.T) *service.RecommendedSkill {
	t.Helper()
	if len(service.RecommendedSkills) == 0 {
		t.Fatal("推荐技能清单不能为空")
	}
	return &service.RecommendedSkills[0]
}

// catalogHarness 只装技能/MCP 需要的那一小撮依赖；assetsDir 为空时用临时目录。
func catalogHarness(t *testing.T, assetsDir string) (*service.App, pkg.Layout, *repo.Store) {
	t.Helper()
	dir := t.TempDir()
	if assetsDir == "" {
		assetsDir = filepath.Join(dir, "assets")
	}
	layout := pkg.ResolveLayout(dir, assetsDir, true)
	layout.DataDir = dir
	layout.SessionsDir = filepath.Join(dir, "sessions")
	layout.SkillsDir = filepath.Join(dir, "skills")
	layout.SkillsOffDir = filepath.Join(dir, "skills_disabled")
	layout.LogDir = filepath.Join(dir, "logs")
	layout.StorePath = filepath.Join(dir, "workbaby.json")
	pkg.SetLogDir("")
	if err := pkg.EnsureDir(layout.DataDir); err != nil {
		t.Fatalf("准备目录失败：%v", err)
	}
	store, err := repo.Open(layout.StorePath)
	if err != nil {
		t.Fatalf("开设置存储失败：%v", err)
	}

	db, err := repo.OpenDB(layout.DatabasePath())
	if err != nil {
		t.Fatalf("开数据库失败：%v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := service.New(service.Options{Layout: layout, Store: store, DB: db, Hub: sse.New(16), Version: "test"})
	return app, layout, store
}

// 假上游与帧捕获：上游收到什么、界面看到什么，都从这里取样。
// frameCapture 收订阅到的帧：广播入队与出队是异步的，断言前要等泵送达。
type frameCapture struct {
	mu     sync.Mutex
	frames []sse.Frame
}

// sink 把一帧记下来。
func (c *frameCapture) sink(frame sse.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, frame)
	return nil
}

// of 取某个事件的所有帧。
func (c *frameCapture) of(event string) []sse.Frame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sse.Frame, 0, len(c.frames))
	for _, f := range c.frames {
		if f.Event == event {
			out = append(out, f)
		}
	}
	return out
}

// waitFrames 等某个事件攒够 n 帧；超时即失败。
func (c *frameCapture) waitFrames(t *testing.T, event string, n int) []sse.Frame {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.of(event); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等事件 %s ×%d 超时，实收 %d 帧", event, n, len(c.of(event)))
	return nil
}

// scriptServer 起一个按脚本吐 SSE 分片的假上游，并记下收到的请求体。
type scriptServer struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
	// Chunks 是正常对话那套分片。
	chunks []string
	// SummaryChunks 非空时，请求体带压缩系统提示的改走这套分片：总结与对话共用一个假上游。
	summaryChunks []string
	// FollowUpChunks 非空时，第一次请求之后都改走这套分片：先要工具、拿到结果再收尾的场景。
	followUpChunks []string
}

// newScriptServer 造一个假上游：每个请求吐完 chunks 后结束（不再续轮）。
func newScriptServer(t *testing.T, chunks []string) *scriptServer {
	t.Helper()
	s := &scriptServer{chunks: chunks}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = readFull(r, raw)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(raw))
		count := len(s.bodies)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range s.replyChunks(string(raw), count) {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(s.server.Close)
	return s
}

// newSummaryServer 造一个两用假上游：压缩总结走摘要脚本，正常对话走对话脚本。
func newSummaryServer(t *testing.T, chatChunks []string) *scriptServer {
	t.Helper()
	s := newScriptServer(t, chatChunks)
	s.summaryChunks = []string{
		`{"choices":[{"delta":{"content":"用户问了目录结构，助手作答。"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
	return s
}

// replyChunks 按请求体与请求次数挑该回哪套分片。
func (s *scriptServer) replyChunks(body string, count int) []string {
	if len(s.summaryChunks) > 0 && strings.Contains(body, compactionSystemDigest) {
		return s.summaryChunks
	}
	if len(s.followUpChunks) > 0 && count > 1 {
		return s.followUpChunks
	}
	return s.chunks
}

// requestBodies 取到目前为止的请求体副本。
func (s *scriptServer) requestBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.bodies...)
}

// resetBodies 清掉已记的请求体，用于切换场景。
func (s *scriptServer) resetBodies() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies = nil
}

// compactionSystemDigest 用于识别总结调用的片段：取压缩系统提示里稳定的词。
const compactionSystemDigest = "对话整理助手"

// readFull 把请求体读满 buf。
func readFull(r *http.Request, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Body.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// replyChunks 是一轮「纯文本回复、不要工具」的脚本。
func replyChunks(text string) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, text),
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}
}

// bindWorkspaceAndModel 给 harness 备好工作区、服务商与选中模型。
func bindWorkspaceAndModel(t *testing.T, h *harness, base string) {
	t.Helper()
	env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": t.TempDir()})
	h.mustOK(env, code, "workspace", nil)
	env, code = h.call("POST", "/providers/upsert", domain.ProviderUpsertREQ{
		ProviderName: "up", BaseUrl: base, ApiKey: "k",
		Models: []domain.ProviderModelEntryVO{{ModelId: "m", ContextWindow: 100000}},
	})
	h.mustOK(env, code, "upsert", nil)
	if err := h.app.Chat.SetModel("", "up", "m"); err != nil {
		t.Fatalf("选模型失败：%v", err)
	}
}

// waitIdle 等当前运行收尾；超时即失败。
func waitIdle(t *testing.T, h *harness) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !h.app.Chat.Busy() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("3 秒后运行仍未收尾")
}

// writeFile 写一个文件，父目录不存在就建。
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}
