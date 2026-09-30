// 内联 <think> 标签的分流：真实故障是部分模型把推理直接写进 content，
// 界面会露出裸 <think> 标签且思考永远折叠不了。这里锁两条链路：
// 常规整段分流、标签被流式切块切碎（跨 chunk 状态保持）。
package llm

import (
	"strings"
	"testing"
)

// feed 按给定分块逐块喂入，返回（思考拼接, 正文拼接）。
func feed(chunks []string) (string, string) {
	var s ThinkSplitter
	var think, text strings.Builder
	for _, c := range chunks {
		a, b := s.Split(c)
		think.WriteString(a)
		text.WriteString(b)
	}
	return think.String(), text.String()
}

// 整段标签、多块思考、前后混杂正文：分流的常规形态。
func TestThinkSplitterSeparatesContent(t *testing.T) {
	think, text := feed([]string{"<think>我在想</think>", "你好"})
	if think != "我在想" || text != "你好" {
		t.Fatalf("整段分流不对: think=%q text=%q", think, text)
	}

	think, text = feed([]string{
		"开头", "<think>第一段</think>", "中间", "<think>第二段</think>", "结尾",
	})
	if think != "第一段第二段" {
		t.Errorf("多块思考不对: %q", think)
	}
	if text != "开头中间结尾" {
		t.Errorf("正文拼接不对: %q", text)
	}

	// 纯文本必须原样透传，一个字符都不能动
	think, text = feed([]string{"普通回答", "没有标签"})
	if think != "" || text != "普通回答没有标签" {
		t.Fatalf("纯文本应原样透传: think=%q text=%q", think, text)
	}
}

// 标签被拆到多个 delta 里：这是流式场景的常态，必须跨块保持状态。
func TestThinkSplitterHandlesSplitTags(t *testing.T) {
	think, text := feed([]string{"<thi", "nk>推理中", "</thi", "nk>", "答案"})
	if think != "推理中" || text != "答案" {
		t.Fatalf("切碎标签分流不对: think=%q text=%q", think, text)
	}

	// 逐字符喂入：最恶劣的切块方式也不能漏字
	whole := "<think>深度思考</think>最终结论"
	chars := make([]string, 0, len([]rune(whole)))
	for _, r := range whole {
		chars = append(chars, string(r))
	}
	think, text = feed(chars)
	if think != "深度思考" || text != "最终结论" {
		t.Fatalf("逐字喂入分流不对: think=%q text=%q", think, text)
	}
}
