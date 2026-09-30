// 模型服务的默认继承链：设默认服务 → 落默认模型 → 新会话拿得到模型名。
// 这条链断掉时界面显示「默认模型」且窗口永远算不出来，症状分散在多处，
// 单看任何一处都正常，所以单独锁一条。
package service

import (
	"testing"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
)

func TestDefaultProviderCarriesDefaultModel(t *testing.T) {
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
	if !svc.Chat.capabilityOf(sess.Model).Known {
		t.Errorf("模型 %q 应能查到能力画像", sess.Model)
	}
}

// 存量会话模型名为空时，跑一次 run 应当就地补齐并回写。
// 缺了这条，老会话永远显示「默认模型」，能力查询也认不出窗口大小。
func TestLegacySessionBackfillsModelOnRun(t *testing.T) {
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
}
