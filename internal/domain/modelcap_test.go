// 模型能力目录与工具目录：这两个接口是用户质疑「你怎么知道上下文多大、
// 支不支持思考」与「助手到底有哪些工具」的直接答案，行为必须稳定可断言。
package domain

import "testing"

func TestModelCapabilityKnownModels(t *testing.T) {
	cases := []struct {
		model    string
		window   int
		thinking bool
	}{
		{"gpt-4o-mini", 128000, false},
		{"o3-mini", 200000, true},
		{"claude-sonnet-4-20250514", 200000, true},
		{"deepseek-reasoner", 128000, true},
		{"MiniMax-M3", 1000000, true},
		{"qwen3-32b", 131072, true},
	}
	for _, c := range cases {
		got := ModelCapabilityOf(c.model)
		if !got.Known {
			t.Errorf("%s 应命中能力目录", c.model)
			continue
		}
		if got.ContextWindow != c.window || got.Thinking != c.thinking {
			t.Errorf("%s: 期望 window=%d thinking=%v，实际 window=%d thinking=%v",
				c.model, c.window, c.thinking, got.ContextWindow, got.Thinking)
		}
	}
}

// 认不出来的模型必须诚实地标 unknown 并给出说明，
// 拿一个猜出来的 128K 当权威显示给用户，比显示「未知」更糟。
func TestModelCapabilityUnknownIsHonest(t *testing.T) {
	got := ModelCapabilityOf("some-private-model-v9")
	if got.Known {
		t.Fatal("未知模型不该标成已知")
	}
	if got.Note == "" {
		t.Error("未知模型必须带一句说明，告诉用户可以手填")
	}
	if got.ContextWindow <= 0 {
		t.Error("未知模型仍要给一个可用作缺省的窗口值")
	}
}

// 更具体的规则必须优先于宽泛规则：o3 不能被 gpt 规则或缺省兜底吃掉。
func TestModelCapabilityPrefersSpecificRule(t *testing.T) {
	if got := ModelCapabilityOf("claude-3-haiku-20240307"); got.Thinking {
		t.Error("claude-3-haiku 不支持思考，宽泛的 claude 规则不应误命中")
	}
	if got := ModelCapabilityOf("claude-3-5-sonnet-20241022"); !got.Thinking {
		t.Error("claude-3-5-sonnet 应命中支持思考的规则")
	}
}
