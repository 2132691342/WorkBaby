// 写入口的边界纪律：参数校验在触达存储之前就要拦住，坏了配置时宁可拒绝写入也不能整表覆盖。
package tests

import (
	"strings"
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
	// 被拒的写入不该留下半个结果：三个键都还是空的。
	if got := h.app.Models.Defaults(); got.DefaultProvider != "" || got.DefaultModel != "" {
		t.Fatalf("被拒的写入不该落盘，实际 %+v", got)
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

// TestModelDefaultKeepsOtherSettings 断言写默认模型不碰设置台账里的其他键。
// 键值表是共享的：一个写入口顺手清了别人的键，用户会在别处莫名发现设置没了。
func TestModelDefaultKeepsOtherSettings(t *testing.T) {
	h := newHarness(t)
	if err := h.store.Save(map[string]string{"ui.theme": "dark"}); err != nil {
		t.Fatalf("预置设置失败：%v", err)
	}
	if _, err := h.app.Models.SaveDefaults(domain.ModelDefaultSaveREQ{
		DefaultProvider: "router", DefaultModel: "m1",
	}); err == nil {
		t.Fatal("服务商不存在时该被拒绝")
	}
	if got, ok := h.store.Get("ui.theme"); !ok || got != "dark" {
		t.Fatalf("被拒的写入不该动其他设置，实际 %q（存在=%v）", got, ok)
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
