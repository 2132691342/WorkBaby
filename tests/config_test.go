// 配置面的写入纪律：默认模型写入口、服务商密钥、MCP 台账。
// 这些入口共享同一份台账，坏一个写入口会连累别的键——被拒不落盘、并发不丢条目都要钉死。
package tests

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"WorkBaby/backend/domain"
)

// TestModelDefaultGuards 断言默认模型写入口的三条纪律：非法组合先拒、
// 写成功时回读一致、清空等于回到未设置。
func TestModelDefaultGuards(t *testing.T) {
	h := newHarness(t)
	if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
		ProviderName: "router", BaseUrl: "https://x.example/v1", ApiProtocol: "openai",
		ApiKey: "sk-secret", Models: []domain.ProviderModelEntryVO{{ModelId: "m1"}},
	}); err != nil {
		t.Fatalf("建服务商失败：%v", err)
	}

	// ---- 非法组合：先拒，不落盘，也不动台账里的其他键 ----
	// 台账是共享键值表：一个写入口顺手清了别人的键，用户会在别处莫名发现设置没了。
	if err := h.store.Save(map[string]string{"ui.theme": "dark"}); err != nil {
		t.Fatalf("预置设置失败：%v", err)
	}
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
	// 被拒的写入不该留下半个结果：三个键都还是空的，别人的键也原封不动。
	if got := h.app.Models.Defaults(); got.DefaultProvider != "" || got.DefaultModel != "" {
		t.Fatalf("被拒的写入不该落盘，实际 %+v", got)
	}
	if got, ok := h.store.Get("ui.theme"); !ok || got != "dark" {
		t.Fatalf("被拒的写入不该动其他设置，实际 %q（存在=%v）", got, ok)
	}

	// ---- 写成功：回读一致 ----
	view, err := h.app.Models.SaveDefaults(domain.ModelDefaultSaveREQ{
		DefaultProvider: "router", DefaultModel: "m1", DefaultThinkingLevel: "high",
	})
	if err != nil {
		t.Fatalf("保存默认模型失败：%v", err)
	}
	if view.DefaultProvider != "router" || view.DefaultModel != "m1" || view.DefaultThinkingLevel != "high" {
		t.Fatalf("回读与写入不一致：%+v", view)
	}

	// ---- 清空等于回到未设置：下次开新会话不受它约束 ----
	if _, err := h.app.Models.SaveDefaults(domain.ModelDefaultSaveREQ{}); err != nil {
		t.Fatalf("清空默认模型失败：%v", err)
	}
	cleared := h.app.Models.Defaults()
	if cleared.DefaultProvider != "" || cleared.DefaultModel != "" || cleared.DefaultThinkingLevel != "" {
		t.Fatalf("清空后应全部为空，实际 %+v", cleared)
	}
}

// TestProviderKeyMasked 断言服务商密钥读出来恒为掩码，空串语义是「保留原值」。
// 这两条是密钥不经界面明文外流的唯一保证：一条漏了就是一次泄露。
func TestProviderKeyMasked(t *testing.T) {
	h := newHarness(t)
	if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
		ProviderName: "router", BaseUrl: "https://x.example/v1", ApiProtocol: "openai",
		ApiKey: "sk-1234567890abcdef", Models: []domain.ProviderModelEntryVO{{ModelId: "m1"}},
	}); err != nil {
		t.Fatalf("建服务商失败：%v", err)
	}

	view := h.app.Models.Providers()
	if len(view.Providers) != 1 {
		t.Fatalf("应有 1 个服务商，实际 %d", len(view.Providers))
	}
	provider := view.Providers[0]
	if strings.Contains(provider.ApiKeyMasked, "1234567890") {
		t.Fatalf("密钥原文不该出现在掩码里：%q", provider.ApiKeyMasked)
	}
	if !strings.Contains(provider.ApiKeyMasked, "••••") {
		t.Fatalf("掩码该有固定的遮罩段：%q", provider.ApiKeyMasked)
	}
	if !provider.HasKey {
		t.Fatal("该报出「已配密钥」")
	}

	// 空串 = 保留原值：界面回传的是掩码，当成新值存会把密钥写坏。
	if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
		ProviderName: "router", BaseUrl: "https://x.example/v1", ApiProtocol: "openai",
		ApiKey: "", Models: []domain.ProviderModelEntryVO{{ModelId: "m1"}},
	}); err != nil {
		t.Fatalf("改地址失败：%v", err)
	}
	_, key, err := h.app.Models.Lookup("router", "m1")
	if err != nil {
		t.Fatalf("查模型失败：%v", err)
	}
	if key != "sk-1234567890abcdef" {
		t.Fatalf("空串不该覆盖原密钥，实际 %q", key)
	}
}

// TestMcpConfig 断言 mcp.json 的两条治理纪律：12 路并发读-改-写一条不丢（无锁时后写
// 会拿回旧表，用户刚加的服务器凭空消失）；删项目级条目不连带删被它遮蔽的用户级条目。
func TestMcpConfig(t *testing.T) {
	app, layout, _ := catalogHarness(t, "")

	// ---- 并发写 ----
	const total = 12
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "server-" + string(rune('a'+i))
			_, _ = app.Mcp.Save(domain.McpSaveREQ{
				ServerName:  name,
				CommandText: "node server.js",
				McpScope:    "user",
			})
		}(i)
	}
	wg.Wait()

	raw, err := os.ReadFile(layout.McpFile())
	if err != nil {
		t.Fatalf("读 mcp.json 失败：%v", err)
	}
	var parsed struct {
		McpServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("mcp.json 解析失败（写坏了）：%v", err)
	}
	if len(parsed.McpServers) != total {
		t.Errorf("并发写后应保留 %d 个服务器，实际 %d 个", total, len(parsed.McpServers))
	}

	// ---- 作用域遮蔽 ----
	ws := t.TempDir()
	if _, err := app.Settings.SetWorkspace(ws); err != nil {
		t.Fatalf("切工作区失败：%v", err)
	}
	if _, err := app.Mcp.Save(domain.McpSaveREQ{
		ServerName: "dup", CommandText: "node user.js", McpScope: "user",
	}); err != nil {
		t.Fatalf("写用户级失败：%v", err)
	}
	if _, err := app.Mcp.Save(domain.McpSaveREQ{
		ServerName: "dup", CommandText: "node project.js", McpScope: "project",
	}); err != nil {
		t.Fatalf("写项目级失败：%v", err)
	}
	if _, err := app.Mcp.Remove("dup"); err != nil {
		t.Fatalf("删除失败：%v", err)
	}

	raw, err = os.ReadFile(layout.McpFile())
	if err != nil {
		t.Fatalf("读用户级 mcp.json 失败：%v", err)
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if _, ok := parsed.McpServers["dup"]; !ok {
		t.Error("删项目级条目时把被遮蔽的用户级条目也删了")
	}
}
