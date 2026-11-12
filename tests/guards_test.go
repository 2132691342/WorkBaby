// 会话分叉与模型默认值的写入口边界：参数校验在触达引擎之前就要拦住，
// 坏了配置文件时宁可拒绝写入也不能整表覆盖。
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
	"WorkBaby/backend/service/engine"
)

// TestChatGuards 断言两条「先校验再动手」的纪律：
// 分叉缺起点直接拒（否则引擎会拿空 id 去查树），没选工作区时拒（不误报引擎故障）。
func TestChatGuards(t *testing.T) {
	h := newHarness(t)

	// ---- 分叉缺起点：入参不合法，不能落到引擎 ----
	env, code := h.call("POST", "/chat/fork", map[string]any{"session_file": "x.jsonl", "turn_id": ""})
	if code != 200 {
		t.Fatalf("HTTP 应恒 200，实际 %d", code)
	}
	if env.Code != pkg.CodeBadRequest {
		t.Errorf("空 turn_id 应回 %s，实际 %s（%s）", pkg.CodeBadRequest, env.Code, env.Message)
	}

	// ---- 没有工作区：报的是「先选工作区」，不是「没检测到 pi」----
	env, _ = h.call("POST", "/chat/fork", map[string]any{"session_file": "x.jsonl", "turn_id": "u1"})
	if env.Code != pkg.CodeSessionBind && env.Code != pkg.CodeEngineMissing {
		t.Errorf("没有工作区时应有可读引导，实际 %s（%s）", env.Code, env.Message)
	}
}

// TestModelsCorruptGuard 断言 models.json 坏掉时读失败说出来、写操作拒绝执行。
// 坏文件里可能还有用户手写的服务商，整表覆盖等于一次清空，且不可恢复。
func TestModelsCorruptGuard(t *testing.T) {
	h := newHarness(t)
	if err := pkg.EnsureDir(filepath.Dir(h.layout.ModelsFile())); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(h.layout.ModelsFile(), []byte("{ 这不是 JSON"), 0o644); err != nil {
		t.Fatalf("写坏配置失败：%v", err)
	}

	// ---- 读：把原因摆出来 ----
	var view struct {
		ReadError string `json:"read_error"`
	}
	env, _ := h.call("GET", "/providers", nil)
	h.mustOK(env, 200, "读服务商", &view)
	if !strings.Contains(view.ReadError, "models.json") {
		t.Errorf("坏配置应报出原因，实际 %q", view.ReadError)
	}

	// ---- 写：拒绝执行，且不把原文件改掉 ----
	env, _ = h.call("POST", "/providers/upsert", map[string]any{
		"provider_name": "x", "base_url": "https://example.com", "api_protocol": "openai",
	})
	if env.Code == pkg.CodeOK {
		t.Fatal("坏配置上的写入必须拒绝，否则会整表覆盖用户的配置")
	}
	raw, err := os.ReadFile(h.layout.ModelsFile())
	if err != nil {
		t.Fatalf("读回配置失败：%v", err)
	}
	if !strings.Contains(string(raw), "这不是 JSON") {
		t.Errorf("被拒的写入不该改动文件，实际内容 %q", string(raw))
	}
}

// TestModelDefaults 断言默认模型写入口的三条纪律：非法组合先拒、
// 写成功时把用户自己的 settings.json 键原样带回去、清空等于删键。
func TestModelDefaults(t *testing.T) {
	h := newHarness(t)
	// 预置用户手配的键与包声明：保存默认模型不能把它们弄丢。
	if err := pkg.WriteJSON(h.layout.SettingsFile(), map[string]any{
		"packages": []any{"npm:pi-lens@1.2.3"},
		"theme":    "dark",
	}); err != nil {
		t.Fatalf("预置 settings.json 失败：%v", err)
	}
	if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
		ProviderName: "router", BaseUrl: "https://x.example/v1", ApiProtocol: "openai",
		ApiKey: "sk-secret", Models: []domain.ProviderModelEntryVO{{ModelId: "m1"}},
	}); err != nil {
		t.Fatalf("建服务商失败：%v", err)
	}

	// ---- 非法组合：先拒，不落盘 ----
	for _, tc := range []struct {
		name string
		req  domain.ModelDefaultSaveREQ
	}{
		{"服务商不存在", domain.ModelDefaultSaveREQ{DefaultProvider: "nope", DefaultModel: "m1"}},
		{"模型不在该服务商下", domain.ModelDefaultSaveREQ{DefaultProvider: "router", DefaultModel: "zzz"}},
		{"半截组合", domain.ModelDefaultSaveREQ{DefaultProvider: "router"}},
		{"未知思考档", domain.ModelDefaultSaveREQ{DefaultThinkingLevel: "superhigh"}},
	} {
		if _, err := h.app.Models.SaveDefaults(tc.req); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}

	// ---- 写成功：回读一致，且用户自己的键还在 ----
	view, err := h.app.Models.SaveDefaults(domain.ModelDefaultSaveREQ{
		DefaultProvider: "router", DefaultModel: "m1", DefaultThinkingLevel: "high",
	})
	if err != nil {
		t.Fatalf("保存默认模型失败：%v", err)
	}
	if view.DefaultProvider != "router" || view.DefaultModel != "m1" || view.DefaultThinkingLevel != "high" {
		t.Fatalf("回读与写入不一致：%+v", view)
	}
	raw, err := os.ReadFile(h.layout.SettingsFile())
	if err != nil {
		t.Fatalf("读回 settings.json 失败：%v", err)
	}
	for _, want := range []string{"pi-lens", `"theme"`, "defaultModel", "defaultThinkingLevel"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("settings.json 里丢了 %q：%s", want, raw)
		}
	}

	// ---- 清空：三个键一起消失，其余仍在 ----
	cleared, err := h.app.Models.SaveDefaults(domain.ModelDefaultSaveREQ{})
	if err != nil {
		t.Fatalf("清空失败：%v", err)
	}
	if cleared.DefaultProvider != "" || cleared.DefaultModel != "" || cleared.DefaultThinkingLevel != "" {
		t.Fatalf("清空后仍读到了默认值：%+v", cleared)
	}
	raw, _ = os.ReadFile(h.layout.SettingsFile())
	if strings.Contains(string(raw), "defaultModel") {
		t.Errorf("清空应删键而不是留空串：%s", raw)
	}
	if !strings.Contains(string(raw), "pi-lens") {
		t.Errorf("清空默认模型不该动用户的其他键：%s", raw)
	}
}

// TestEngineNeedsNode 断言「pi 在但 node 缺」不会被误报成「没装 pi」。
// 两种情况的下一步完全不同：前者要装 node，后者要装 pi。
func TestEngineNeedsNode(t *testing.T) {
	dir := t.TempDir()
	// 造一个只有 cli.js、没有 node 的包目录（PATH 清空即等价于机器上没 node）。
	pkgDir := filepath.Join(dir, "pi-coding-agent")
	script := filepath.Join(pkgDir, "dist", "bundle", "cli.js")
	if err := pkg.EnsureDir(filepath.Dir(script)); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(script, []byte("// cli"), 0o644); err != nil {
		t.Fatalf("写脚本失败：%v", err)
	}

	t.Setenv("PATH", "")
	t.Setenv("npm_config_prefix", "")

	layout := pkg.ResolveLayout(dir, "", true)
	rt := engine.New(layout, func() string { return pkgDir })
	entry := rt.Resolve()
	if entry.Command != "" {
		t.Fatalf("没有 node 时不该给出可执行入口：%+v", entry)
	}
	if !entry.NodeMissing {
		t.Fatal("应识别出「pi 在但 node 缺」，而不是退化成「没装 pi」")
	}
	if entry.ScriptPath != script {
		t.Errorf("应带回命中的脚本路径，实际 %q", entry.ScriptPath)
	}
	if !rt.NeedsNode() {
		t.Error("NeedsNode 应与 Entry.NodeMissing 一致")
	}

	// 状态与文案：指引要指向 node，不是重装 pi。
	status := service.RuntimeStatus(rt)
	if status.PiReady {
		t.Error("没有 node 时 PiReady 应为 false")
	}
	if !status.PiNeedsNode {
		t.Error("状态里应带上 PiNeedsNode")
	}
	if !strings.Contains(status.DetectionHint, "node") && !strings.Contains(status.DetectionHint, "Node") {
		t.Errorf("指引应指向 node，实际 %q", status.DetectionHint)
	}

}
