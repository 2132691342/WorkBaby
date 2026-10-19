// 文本清洗：把外部进程与内核带出来的终端控制序列剥掉，只留可读文本。
// 内核与扩展按终端习惯给文字上色，GUI 没有终端语义，原样显示就是一片乱码。
package pkg

import "strings"

// StripANSI 去掉 CSI / OSC / 单字符转义序列。
// CSI（如 \x1b[2m、\x1b[22;39m）承担颜色与显隐，OSC（如 \x1b]0;标题\x07）多被误当成正文。
func StripANSI(s string) string {
	if s == "" || !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		i = skipEscape(s, i)
	}
	return b.String()
}

// skipEscape 返回第 i 个转义符之后的下标；遇到不完整的序列时停在其后。
func skipEscape(s string, i int) int {
	i++ // 吃掉 ESC
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[': // CSI：参数与中间字节后跟一个终结字节（@ 到 ~）
		i++
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
			i++
		}
		if i < len(s) {
			i++
		}
	case ']': // OSC：以 BEL 或 ST（ESC \）结束
		i++
		for i < len(s) {
			if s[i] == 0x07 {
				i++
				break
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				i += 2
				break
			}
			i++
		}
	case 'P', 'X', '^', '_': // DCS / SOS / PM / APC：同样以 ST 结束
		i++
		for i < len(s) {
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				i += 2
				break
			}
			i++
		}
	default: // 单字符转义（ESC ( B 之类的两字符序列按两字符吃）
		i++
		if i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
	}
	return i
}
