package pkg

import "strings"

// SplitLines 按 LF 切行；不碰 U+2028 / U+2029——它们是 JSON 字符串里的合法内容。
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	out := make([]string, 0, len(parts))
	for _, line := range parts {
		out = append(out, strings.TrimSuffix(line, "\r"))
	}
	return out
}

// SplitLines 是 splitLines 的导出形式，给需要按 LF 切帧的调用方用。
func SplitLines(text string) []string { return splitLines(text) }

// SplitCommandLine 把一行命令行拆成 argv，支持单双引号包住带空格的参数。
// 只做分词，不进 shell：拼出来的 argv 一律直传子进程。
func SplitCommandLine(line string) []string {
	var out []string
	var buf strings.Builder
	var quote rune
	flushed := true
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			flushed = false
		case r == ' ' || r == '\t':
			if !flushed || buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
				flushed = true
			}
		default:
			buf.WriteRune(r)
			flushed = false
		}
	}
	if !flushed || buf.Len() > 0 {
		out = append(out, buf.String())
	}
	// 落单的引号会产生空串：命令行里 "" 是合法的空参数，所以保留但要过滤尾部空串。
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}
