package llm

import "testing"

// ProviderPresets 的 Models 必须是非 nil 切片。
//
// nil slice 会 JSON 序列化成 null，前端 ProviderSettings 模板里按数组下标访问
// （p.models[0]）会抛 "Cannot read properties of null"，导致「设置 → 模型」整页空白。
// 本测试锁住这个契约，防止后续新增预设时漏填 Models 再次踩坑。
func TestProviderPresetsModelsNonNull(t *testing.T) {
	presets := ProviderPresets()
	if len(presets) == 0 {
		t.Fatal("ProviderPresets 为空，内置预设清单不应为空")
	}
	for _, p := range presets {
		if p.Models == nil {
			t.Fatalf("预设 %q 的 Models 为 nil（会序列化成 null，导致前端整页渲染失败）", p.Name)
		}
	}
}
