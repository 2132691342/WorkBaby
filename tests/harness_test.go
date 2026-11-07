// Package tests 的公共装具：两档 harness——
// newHarness 装满全后端（真 JSON 存储、真 gin 中间件栈），catalogHarness 只装需要的一小撮依赖。
package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/api"
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
		StorePath:    filepath.Join(dir, "workbaby.json"),
		AssetsDir:    filepath.Join(dir, "assets"),
	}
	if err := pkg.EnsureDir(layout.PiAgentDir); err != nil {
		t.Fatalf("准备目录失败：%v", err)
	}
	pkg.SetLogDir("")

	store, err := repo.Open(layout.StorePath)
	if err != nil {
		t.Fatalf("开设置存储失败：%v", err)
	}

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

// defaultSkill 取清单里第一个指定默认态的技能。
func defaultSkill(t *testing.T, wantOn bool) *service.BuiltinSkill {
	t.Helper()
	for i := range service.BuiltinSkills {
		if service.BuiltinSkills[i].SkillDefaultOn == wantOn {
			return &service.BuiltinSkills[i]
		}
	}
	t.Fatalf("清单里应至少有一个默认启用=%v 的技能", wantOn)
	return nil
}

// catalogHarness 只装技能/插件/MCP 需要的那一小撮依赖；assetsDir 为空时用临时目录。
func catalogHarness(t *testing.T, assetsDir string) (*service.App, pkg.Layout, *repo.Store) {
	t.Helper()
	dir := t.TempDir()
	if assetsDir == "" {
		assetsDir = filepath.Join(dir, "assets")
	}
	layout := pkg.Layout{
		DataDir:    dir,
		PiAgentDir: filepath.Join(dir, "pi"),
		AssetsDir:  assetsDir,
		StorePath:  filepath.Join(dir, "workbaby.json"),
		LogDir:     filepath.Join(dir, "logs"),
	}
	pkg.SetLogDir("")
	if err := pkg.EnsureDir(layout.PiAgentDir); err != nil {
		t.Fatalf("准备目录失败：%v", err)
	}
	store, err := repo.Open(layout.StorePath)
	if err != nil {
		t.Fatalf("开设置存储失败：%v", err)
	}

	app := service.New(service.Options{Layout: layout, Store: store, Hub: sse.New(16), Version: "test"})
	return app, layout, store
}
