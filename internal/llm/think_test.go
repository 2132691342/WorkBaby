// 内联 <think> 标签的分流：标签被切碎、切在内容中间、多次出现都要能正确拆开。
// 真实故障：MiniMax-M3 把推理写在 content 里，界面直接显示裸 <think> 标签，
// 且思考过程永远折叠不了——因为它进了 content 而不是 thinking 通道。
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

func TestThinkSplitterSeparatesContent(t *testing.T) {
	think, text := feed([]string{"<think>我在想</think>", "你好"})
	if think != "我在想" {
		t.Errorf("思考段不对: %q", think)
	}
	if text != "你好" {
		t.Errorf("正文段不对: %q", text)
	}
}

// 标签被拆到多个 delta 里：这是流式场景的常态，必须跨块保持状态。
func TestThinkSplitterHandlesSplitTags(t *testing.T) {
	think, text := feed([]string{"<thi", "nk>推理中", "</thi", "nk>", "答案"})
	if think != "推理中" {
		t.Errorf("思考段不对: %q", think)
	}
	if text != "答案" {
		t.Errorf("正文段不对: %q", text)
	}
}

// 每个字符单独喂一次：最坏情况下的碎片化。
func TestThinkSplitterCharByChar(t *testing.T) {
	var chunks []string
	for _, r := range "<think>思考</think>正文" {
		chunks = append(chunks, string(r))
	}
	think, text := feed(chunks)
	if think != "思考" {
		t.Errorf("思考段不对: %q", think)
	}
	if text != "正文" {
		t.Errorf("正文段不对: %q", text)
	}
}

// 标签前后的正文都必须原样保留，不能被吞掉。
func TestThinkSplitterKeepsSurroundingText(t *testing.T) {
	think, text := feed([]string{"前面<think>中间</think>后面"})
	if think != "中间" {
		t.Errorf("思考段不对: %q", think)
	}
	if text != "前面后面" {
		t.Errorf("正文段不对: %q", text)
	}
}

// 多段思考：模型可能反复进入退出思考状态。
func TestThinkSplitterMultipleBlocks(t *testing.T) {
	think, text := feed([]string{"<think>一</think>正文1<think>二</think>正文2"})
	if think != "一二" {
		t.Errorf("思考段不对: %q", think)
	}
	if text != "正文1正文2" {
		t.Errorf("正文段不对: %q", text)
	}
}

// 没有标签的普通正文必须原样透传，一个字符都不能少。
func TestThinkSplitterPassthroughPlainText(t *testing.T) {
	_, text := feed([]string{"普通回答", "，没有任何标签。"})
	if text != "普通回答，没有任何标签。" {
		t.Errorf("普通正文被改动了: %q", text)
	}
}
