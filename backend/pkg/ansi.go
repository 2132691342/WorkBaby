package pkg

import "strings"

// StripANSI 去掉 ANSI 转义序列。
// 引擎侧的状态文字与通知是给终端写的，夹带颜色码（`\x1b[39m` 之类）；
// 界面是 HTML，原样渲染出来是一串方括号数字。出口统一洗一遍。
func StripANSI(text string) string {
	if text == "" || !strings.ContainsRune(text, '\x1b') {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] != '\x1b' {
			out.WriteByte(text[i])
			continue
		}
		// ESC 后紧跟的引导符决定这条序列多长：'[' 是 CSI，吃到终止字节（0x40~0x7E）；
		// ']' 是 OSC，吃到 BEL 或 ST；其余两字节序列只跳掉下一个字符。
		if i+1 >= len(text) {
			break
		}
		switch text[i+1] {
		case '[':
			i += 2
			for i < len(text) && (text[i] < 0x40 || text[i] > 0x7e) {
				i++
			}
		case ']':
			i += 2
			for i < len(text) {
				if text[i] == '\a' {
					break
				}
				if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '\\' {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return out.String()
}

// CleanText 洗掉控制码并压掉首尾空白：给界面的一句话。
func CleanText(text string) string {
	return strings.TrimSpace(StripANSI(text))
}
