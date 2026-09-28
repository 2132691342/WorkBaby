package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// 引用文件的读入上限：够看清一个表格/文档的骨架，又不至于把上下文一次撑爆。
const (
	attachMaxFiles = 5
	attachMaxBytes = 64 << 10
)

// buildAttachBlock 把引用文件读成一段上下文，拼在用户消息最前面。
// 读不到就跳过并说明——用户以为文件进去了、其实没有，是最糟糕的一种失败。
func buildAttachBlock(workspace string, items []domain.AttachmentREQ) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > attachMaxFiles {
		items = items[:attachMaxFiles]
	}
	var b strings.Builder
	for _, it := range items {
		path := it.Path
		if path == "" {
			continue
		}
		full, err := pkg.SafeJoin(workspace, path)
		if err != nil {
			b.WriteString(fmt.Sprintf("### %s\n（读不到：不在工作目录内）\n\n", it.Name))
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil || len(raw) == 0 {
			b.WriteString(fmt.Sprintf("### %s\n（读不到或内容为空）\n\n", it.Name))
			continue
		}
		body := string(raw)
		if len(body) > attachMaxBytes {
			body = cutRunes(body, attachMaxBytes)
		}
		b.WriteString("### " + filepath.Base(full) + "\n\n```\n" + body + "\n```\n\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return "以下是用户引用的文件内容：\n\n" + b.String() + "---\n\n"
}

// cutRunes 按字节上限截断但保证不切在多字节字符中间。
func cutRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n……（内容过长已截断）"
}
