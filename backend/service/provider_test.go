// 模型服务的默认继承链：设默认服务 → 落默认模型 → 新会话拿得到模型名。
// 这条链断掉时界面显示「默认模型」且窗口永远算不出来，症状分散在多处，
// 单看任何一处都正常，所以单独锁一条。
package service

import (
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
)

// 模型服务的默认继承链：设默认服务 → 落默认模型 → 新会话拿得到模型名 →
// 存量空模型会话跑一次 run 就地补齐。这条链断掉时界面显示「默认模型」
// 且窗口永远算不出来，症状分散在多处，单看任何一处都正常。
func TestDefaultModelChain(t *testing.T) {
	t.Run("默认服务落默认模型并被新会话继承", func(t *testing.T) {
		useScripted(t)
		env, svc := newEnv(t)

		p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{
			Name: "m", API: "test", Models: []string{"MiniMax-M3", "gpt-4o"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Providers.SetDefault(p.ID); err != nil {
			t.Fatal(err)
		}

		// 只存服务不存模型时，新会话继承到空模型名，能力查询与上下文水位全失效。
		got, _ := env.Repo.GetSetting(domain.SettingDefaultModel)
		if got != "MiniMax-M3" {
			t.Fatalf("设默认服务后应落默认模型，实际 %q", got)
		}

		sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
		if err != nil {
			t.Fatal(err)
		}
		if sess.ProviderID != p.ID {
			t.Fatalf("会话没继承到默认服务: %q", sess.ProviderID)
		}
		if sess.Model == "" {
			t.Fatal("新会话没有继承到模型名，输入区会一直显示「默认模型」")
		}
	})

	t.Run("存量空模型会话在 run 时回填", func(t *testing.T) {
		useScripted(t, llm.Message{Content: "好的"})
		env, svc := newEnv(t)

		p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{
			Name: "m", API: "test", Models: []string{"MiniMax-M3"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Providers.SetDefault(p.ID); err != nil {
			t.Fatal(err)
		}
		sess, err := svc.Sessions.Create(domain.CreateSessionREQ{})
		if err != nil {
			t.Fatal(err)
		}
		// 人为把模型名清空，模拟老数据。
		if err := env.Repo.UpdateSessionColumns(sess.ID, map[string]any{"model": ""}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Chat.Send(sess.ID, "在吗", nil); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "模型名回写", func() bool {
			fresh, err := env.Repo.GetSession(sess.ID)
			return err == nil && fresh.Model != ""
		})
		fresh, _ := env.Repo.GetSession(sess.ID)
		if fresh.Model != "MiniMax-M3" {
			t.Fatalf("模型名未回写: %q", fresh.Model)
		}
	})
}

// 密钥显式查看链：加密落库 → Reveal 解密回明文；没存密钥时说清楚而不是返回空串。
// 这条链断掉时用户看不到自己填过的 Key，只能删了重填。
func TestProviderKeyReveal(t *testing.T) {
	useScripted(t)
	env, svc := newEnv(t)
	// 测试环境补一把主密钥：加密链要真的跑通，否则这条测试测不到任何东西
	mk, err := pkg.NewMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	env.Cfg.MasterKey = mk

	p, err := svc.Providers.Upsert(domain.UpsertProviderREQ{
		Name: "k", API: "test", APIKey: "sk-secret-123", Models: []string{"m1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Providers.Reveal(p.ID)
	if err != nil {
		t.Fatalf("Reveal 失败: %v", err)
	}
	if got != "sk-secret-123" {
		t.Fatalf("解密结果不对: %q", got)
	}
	// 库里必须是密文：明文落库等于没有加密
	d, err := env.Repo.GetProvider(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.APIKeyEnc == "sk-secret-123" {
		t.Fatal("API Key 以明文落库")
	}

	// 没存过密钥的服务要报明确错误，而不是返回空串让前端显示空掩码
	p2, err := svc.Providers.Upsert(domain.UpsertProviderREQ{
		Name: "nokey", API: "test", Models: []string{"m1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Providers.Reveal(p2.ID); err == nil {
		t.Fatal("没有密钥时应报错")
	}
}
