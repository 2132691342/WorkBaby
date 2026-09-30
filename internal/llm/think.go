// 内联思考标签的分流器：把 <think>…</think> 从正文里拆出来。
//
// 有的模型没有独立的 reasoning 字段，直接把 <think>…</think> 写进 content。
// 不拆的话用户会看到裸标签，思考过程也永远折叠不了。
//
// 难点在增量：标签本身可能被切成多块（"<thi" + "nk>推理…</thi" + "nk>"），
// 所以必须跨块保持状态，不能每个 delta 各自判断一次。
package llm

import "strings"

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// ThinkSplitter 把混着思考标签的正文流拆成「思考」与「正文」两路。
// 零值可用；Split 不持有状态，状态在结构体内。
type ThinkSplitter struct {
	inThink bool   // 当前是否处在思考段内
	hold    string // 可能是标签前缀的尾巴，留到下一块再判
}

// Split 输入一个正文增量，返回本次应归入思考段与正文段的文本。
// 两者的拼接（去掉标签本身）必须等于去掉全部标签后的输入。
func (s *ThinkSplitter) Split(delta string) (think string, text string) {
	if delta == "" {
		return "", ""
	}
	// 先把上一块留下的尾巴接回来。
	buf := s.hold + delta
	s.hold = ""

	var thinkOut, textOut strings.Builder
	emitThink := func(v string) {
		if v != "" {
			thinkOut.WriteString(v)
		}
	}
	emitText := func(v string) {
		if v != "" {
			textOut.WriteString(v)
		}
	}

	for buf != "" {
		if s.inThink {
			idx := strings.Index(buf, thinkClose)
			if idx < 0 {
				// 可能是闭合标签的前缀，尾巴留到下一块。
				keep := tailLen(buf, thinkClose)
				emitThink(buf[:len(buf)-keep])
				s.hold = buf[len(buf)-keep:]
				return thinkOut.String(), textOut.String()
			}
			emitThink(buf[:idx])
			buf = buf[idx+len(thinkClose):]
			s.inThink = false
			continue
		}
		idx := strings.Index(buf, thinkOpen)
		if idx < 0 {
			keep := tailLen(buf, thinkOpen)
			emitText(buf[:len(buf)-keep])
			s.hold = buf[len(buf)-keep:]
			return thinkOut.String(), textOut.String()
		}
		emitText(buf[:idx])
		buf = buf[idx+len(thinkOpen):]
		s.inThink = true
	}
	return thinkOut.String(), textOut.String()
}

// tailLen 返回 buf 末尾有多少个字符可能是 tag 的真前缀。
func tailLen(buf, tag string) int {
	max := len(tag) - 1
	if max > len(buf) {
		max = len(buf)
	}
	for n := max; n > 0; n-- {
		if strings.HasPrefix(tag, buf[len(buf)-n:]) {
			return n
		}
	}
	return 0
}
