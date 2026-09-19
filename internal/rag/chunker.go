// Package rag 是知识库检索增强：文档加载 → 切分 → FTS5 索引 → 统一检索接口。
// v1 检索主力是 SQLite FTS5（trigram + BM25）：零外部依赖、中文友好、离线可用；向量混合检索留待 v2。
//
// 边界：不依赖 harness / api / service / wails；只依赖 domain 与 pkg。
package rag

import (
	"strings"
)

// Chunk 单个文本块；Meta 记录来源标题等定位信息。
type Chunk struct {
	Content string
	Meta    map[string]string
}

// Chunker 按段落聚合切分：段落优先，单段超长才按句子硬切。
type Chunker struct {
	ChunkSize int // 块大小（rune 数）
	Overlap   int // 相邻块重叠（rune 数）
}

// DefaultChunker 默认 500 字符 / 50 重叠。
func DefaultChunker() *Chunker { return &Chunker{ChunkSize: 500, Overlap: 50} }

// Chunk 切分文本；空文本返回空切片。
func (c *Chunker) Chunk(text string) []Chunk {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	out := make([]Chunk, 0, 8)

	var buf strings.Builder
	dirty := false // buf 是否含未出块的新内容（排除 overlap 尾巴，避免末尾产生重复短块）
	heading := ""

	emit := func(withOverlap bool) {
		if !dirty {
			buf.Reset()
			return
		}
		content := strings.TrimSpace(buf.String())
		buf.Reset()
		dirty = false
		if content == "" {
			return
		}
		var meta map[string]string
		if heading != "" {
			meta = map[string]string{"title": heading}
		}
		out = append(out, Chunk{Content: content, Meta: meta})
		if withOverlap && c.Overlap > 0 {
			buf.WriteString(tailRunes(content, c.Overlap))
		}
	}

	for _, para := range splitParagraphs(text) {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if h, ok := headingOf(para); ok {
			heading = h
		}
		pieces := []string{para}
		if runeLen(para) > c.ChunkSize {
			pieces = splitBySentence(para, c.ChunkSize)
		}
		for _, piece := range pieces {
			if buf.Len() > 0 && runeLen(buf.String())+runeLen(piece) > c.ChunkSize {
				emit(true)
			}
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(piece)
			dirty = true
		}
	}
	emit(false)
	return out
}

// splitParagraphs 按空行切段落。
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	var out []string
	var cur strings.Builder
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// headingOf Markdown 标题行（# ~ ######，单行）→ 提取正文作为块归属标题。
func headingOf(para string) (string, bool) {
	lines := strings.Split(para, "\n")
	if len(lines) > 1 {
		return "", false
	}
	line := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	trimmed := strings.TrimLeft(line, "#")
	if trimmed == line { // 没有 # 前缀（理论上不可达，防御）
		return "", false
	}
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || runeLen(trimmed) > 120 {
		return "", false
	}
	return trimmed, true
}

// splitBySentence 把超长段按句子边界切成若干片：目标长度 size，
// 在 size ~ 2*size 窗口内找句子边界；找不到（单句超长）才在 size 处硬切。
func splitBySentence(para string, size int) []string {
	if size <= 0 {
		size = 500
	}
	runes := []rune(para)
	out := make([]string, 0, len(runes)/size+2)
	start := 0
	for start < len(runes) {
		if start+size >= len(runes) {
			if piece := strings.TrimSpace(string(runes[start:])); piece != "" {
				out = append(out, piece)
			}
			break
		}
		end := start + size
		limit := start + size*2
		if limit > len(runes) {
			limit = len(runes)
		}
		for i := end; i < limit; i++ {
			if isSentenceBreak(runes[i]) {
				end = i + 1
				break
			}
		}
		if piece := strings.TrimSpace(string(runes[start:end])); piece != "" {
			out = append(out, piece)
		}
		start = end
	}
	return out
}

func isSentenceBreak(r rune) bool {
	return r == '。' || r == '！' || r == '？' || r == '!' || r == '?' || r == '\n'
}

func runeLen(s string) int { return len([]rune(s)) }

func tailRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
