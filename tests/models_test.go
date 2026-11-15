// 模型与服务商：清单对账、默认模型写入口、密钥处理。
// 这三件事共享同一份台账与同一张表，一个写入口坏了会连累别的键，所以放一起测。
package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/service"
)

// modelIdsOf 取某服务商下的模型 ID 清单。
func modelIdsOf(t *testing.T, h *harness, provider string) []string {
	t.Helper()
	env, code := h.call("GET", "/models", nil)
	h.mustOK(env, code, "models", nil)
	var list domain.ModelListRESP
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("解析模型清单失败：%v", err)
	}
	ids := []string{}
	for _, m := range list.Models {
		if m.ProviderName == provider {
			ids = append(ids, m.ModelId)
		}
	}
	return ids
}

// TestModelReconcile 断言弹窗整表回传的对账语义：
// 已删的行从清单消失、活跃指针指向被删行时一并清掉、
// 删除边界是「服务商 + 模型 ID」而不是裸 ID、单条增删接口不被对账破坏。
func TestModelReconcile(t *testing.T) {
	t.Run("整表替换删掉已删行", func(t *testing.T) {
		h := newHarness(t)
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "a"}, {ModelId: "b"}},
		}); err != nil {
			t.Fatalf("建服务商失败：%v", err)
		}

		// 编辑弹窗整表回传只剩 a：b 是用户删掉的那行。
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "a"}},
		}); err != nil {
			t.Fatalf("整表回传失败：%v", err)
		}
		ids := modelIdsOf(t, h, "router")
		if len(ids) != 1 || ids[0] != "a" {
			t.Fatalf("已删的模型应从清单消失，实际 %v", ids)
		}

		// req.Models 为 nil（只改地址）时保留原清单：那是「清空」与「保留」的分界。
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://y.example/v1", ApiKey: "k",
		}); err != nil {
			t.Fatalf("改地址失败：%v", err)
		}
		ids = modelIdsOf(t, h, "router")
		if len(ids) != 1 || ids[0] != "a" {
			t.Fatalf("不带清单的保存不该动模型，实际 %v", ids)
		}
	})

	t.Run("活跃指针指向被删行时清掉", func(t *testing.T) {
		h := newHarness(t)
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "a"}, {ModelId: "b"}},
		}); err != nil {
			t.Fatalf("建服务商失败：%v", err)
		}
		if err := h.store.Save(map[string]string{
			service.KeyActiveProvider: "router", service.KeyActiveModel: "b",
		}); err != nil {
			t.Fatalf("预置活跃设置失败：%v", err)
		}

		// 整表回传只剩 a：活跃指针指向被删的 b，必须一并清掉。
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "a"}},
		}); err != nil {
			t.Fatalf("整表回传失败：%v", err)
		}
		if got, _ := h.store.Get(service.KeyActiveProvider); got == "router" {
			t.Fatalf("活跃服务商指针指向被删的模型，应被清掉，实际 %q", got)
		}
		if got, _ := h.store.Get(service.KeyActiveModel); got == "b" {
			t.Fatalf("活跃模型指针指向被删的 b，应被清掉，实际 %q", got)
		}
		// 不牵连别的行：a 还在，指针也没必要清。
		ids := modelIdsOf(t, h, "router")
		if len(ids) != 1 || ids[0] != "a" {
			t.Fatalf("保留的行不该被对账删掉，实际 %v", ids)
		}
	})

	t.Run("删除边界是行不是 ID", func(t *testing.T) {
		h := newHarness(t)
		for _, p := range []string{"one", "two"} {
			if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
				ProviderName: p, BaseUrl: "https://x.example/v1", ApiKey: "k",
				Models: []domain.ProviderModelEntryVO{{ModelId: "shared"}},
			}); err != nil {
				t.Fatalf("建服务商 %s 失败：%v", p, err)
			}
		}
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "one", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "kept"}},
		}); err != nil {
			t.Fatalf("整表回传失败：%v", err)
		}
		if ids := modelIdsOf(t, h, "two"); len(ids) != 1 || ids[0] != "shared" {
			t.Fatalf("另一个服务商的同 ID 模型不该被牵连，实际 %v", ids)
		}
	})

	t.Run("单条增删接口不被对账破坏", func(t *testing.T) {
		h := newHarness(t)
		if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
			ProviderName: "router", BaseUrl: "https://x.example/v1", ApiKey: "k",
			Models: []domain.ProviderModelEntryVO{{ModelId: "a"}},
		}); err != nil {
			t.Fatalf("建服务商失败：%v", err)
		}
		if _, err := h.app.Models.SaveModel(domain.ProviderModelSaveREQ{
			ProviderName: "router", Model: domain.ProviderModelEntryVO{ModelId: "added"},
		}); err != nil {
			t.Fatalf("单条增模型失败：%v", err)
		}
		ids := modelIdsOf(t, h, "router")
		if len(ids) != 2 {
			t.Fatalf("单条增的模型该在清单里，实际 %v", ids)
		}
		if _, err := h.app.Models.DeleteModel(domain.ProviderModelDeleteREQ{
			ProviderName: "router", ModelId: "added",
		}); err != nil {
			t.Fatalf("单条删模型失败：%v", err)
		}
		ids = modelIdsOf(t, h, "router")
		if len(ids) != 1 || ids[0] != "a" {
			t.Fatalf("单条删后清单不对，实际 %v", ids)
		}
	})
}

// TestModelDefaults 断言默认模型写入口的三条纪律：非法组合先拒、
// 写成功时回读一致、清空等于回到未设置。
func TestModelDefaults(t *testing.T) {
	h := newHarness(t)
	if _, err := h.app.Models.Upsert(domain.ProviderUpsertREQ{
		ProviderName: "router", BaseUrl: "https://x.example/v1", ApiProtocol: "openai",
		ApiKey: "k", Models: []domain.ProviderModelEntryVO{{ModelId: "m1"}},
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
		{"缺模型", domain.ModelDefaultSaveREQ{DefaultProvider: "router"}},
		{"缺服务商", domain.ModelDefaultSaveREQ{DefaultModel: "m1"}},
		{"服务商不存在", domain.ModelDefaultSaveREQ{DefaultProvider: "nope", DefaultModel: "m1"}},
		{"思考档非法", domain.ModelDefaultSaveREQ{DefaultProvider: "router", DefaultModel: "m1", DefaultThinkingLevel: "ultra"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.app.Models.SaveDefaults(tc.req); err == nil {
				t.Fatal("非法组合应被拒绝")
			}
		})
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
