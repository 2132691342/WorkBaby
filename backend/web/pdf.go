package web

import (
	"regexp"
	"strings"
)

// ExtractPDFText 从 PDF 字节里抽出可读文本。
//
// 纯 Go 的 PDF 解析很难写全：压缩流、字体编码、跨页排版各有各的坑。
// 这里用一条实用主义的路子：解压 FlateDecode 流，抽出文本绘制指令里的字符串。
// 它能读懂绝大多数「文字为主」的 PDF（文档、论文、说明书），
// 遇到扫描件（整页图片）会读不出字——那种情况得走 OCR，
// 而 OCR 需要外部依赖，不在这个能力的边界内。
func ExtractPDFText(raw []byte) string {
	decoded := inflatePDFStreams(raw)
	texts := extractTextOperators(decoded)
	joined := strings.Join(texts, "\n")
	return tidyPDFText(joined)
}

// pdfStreamPattern 找出 PDF 里的 stream ... endstream 段落。
var pdfStreamPattern = regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)

// inflatePDFStreams 把 PDF 里能解压的流解开，其余原样保留。
// 解不开的流保留压缩态：抽文本时它只会成为噪声，被后面的规则滤掉。
func inflatePDFStreams(raw []byte) []byte {
	matches := pdfStreamPattern.FindAllSubmatch(raw, -1)
	out := make([]byte, 0, len(raw)+len(matches)*1024)
	out = append(out, raw...)
	for _, match := range matches {
		decoded, ok := inflate(match[1])
		if !ok {
			continue
		}
		// 把压缩内容追加到末尾：文本指令在解压后的流里，
		// 追加比原位替换简单，也不会破坏 PDF 的其他结构。
		out = append(out, '\n')
		out = append(out, decoded...)
	}
	return out
}

// textOperatorPattern 匹配 PDF 的文本绘制指令。
// Tj 与 TJ 是两种写法：前者一个串，后者一个数组。
var textOperatorPattern = regexp.MustCompile(`\((?:[^()\\]|\\.)*\)\s*Tj|\[(?:[^\[\]]*)\]\s*TJ`)

// pdfStringPattern 抓出一段 PDF 字符串字面量。
var pdfStringPattern = regexp.MustCompile(`\((?:[^()\\]|\\.)*\)`)

// extractTextOperators 从字节流里抽出文本绘制指令的内容。
func extractTextOperators(raw []byte) []string {
	matches := textOperatorPattern.FindAll(raw, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		literals := pdfStringPattern.FindAll(match, -1)
		for _, item := range literals {
			text := decodePDFString(string(item))
			if strings.TrimSpace(text) == "" {
				continue
			}
			out = append(out, text)
		}
	}
	return out
}

// decodePDFString 解一段 PDF 字符串字面量。
// 处理转义与八进制码：PDF 里的换行是 \n，括号是 \( \)。
func decodePDFString(literal string) string {
	trimmed := strings.TrimSpace(literal)
	trimmed = strings.TrimPrefix(trimmed, "(")
	trimmed = strings.TrimSuffix(trimmed, ")")
	var out strings.Builder
	runes := []rune(trimmed)
	for index := 0; index < len(runes); index++ {
		ch := runes[index]
		if ch != '\\' {
			out.WriteRune(ch)
			continue
		}
		index++
		if index >= len(runes) {
			break
		}
		switch runes[index] {
		case 'n':
			out.WriteRune('\n')
		case 'r':
			out.WriteRune('\r')
		case 't':
			out.WriteRune('\t')
		case 'b', 'f':
			out.WriteRune('\n')
		case '\\', '(', ')':
			out.WriteRune(runes[index])
		case '\n':
			// 行续接：反斜杠后换行表示这一行没结束。
		case '\r':
			if index+1 < len(runes) && runes[index+1] == '\n' {
				index++
			}
		default:
			if runes[index] >= '0' && runes[index] <= '7' {
				value := 0
				digits := 0
				for digits < 3 && index < len(runes) &&
					runes[index] >= '0' && runes[index] <= '7' {
					value = value*8 + int(runes[index]-'0')
					index++
					digits++
				}
				index--
				out.WriteRune(rune(value))
				continue
			}
			out.WriteRune(runes[index])
		}
	}
	return out.String()
}

// pdfSpacePattern 找「字与字之间靠定位符拉开」的位置。
// PDF 没有空格的概念，词边界靠坐标差表示；
// 这里用启发式：连续的定位符就当作一个空格。
var pdfSpacePattern = regexp.MustCompile(`(?:-?\d+\.?\d*|\)\s*\(-?\d+\.?\d*)\s+TJ|\]\s*-?\d+\.\d+`)

// tidyPDFText 把抽出的文本整理成可读形态。
func tidyPDFText(text string) string {
	replaced := pdfSpacePattern.ReplaceAllString(text, " ")
	lines := strings.Split(replaced, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}
