package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// 引用文件的读入上限：够看清一个表格/文档的骨架，又不至于把上下文一次撑爆。
const (
	attachMaxFiles   = 5
	attachMaxBytes   = 64 << 10
	imageMaxBytes    = 5 << 20
	imageMaxPerMsg   = 4
)

// 图片扩展名 → MIME。列表之外的一律当文本附件处理。
var imageMIMEs = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".webp": "image/webp", ".gif": "image/gif",
}

func isImageName(name string) (string, bool) {
	mime, ok := imageMIMEs[strings.ToLower(filepath.Ext(name))]
	return mime, ok
}

// buildAttachment 把引用附件拆成「文本上下文块 + 图片列表」。
// 图片（识图输入）不走文本块：base64 直接进消息，由协议层转成 image block。
// 读不到就跳过并说明——用户以为文件进去了、其实没有，是最糟糕的一种失败。
func buildAttachment(workspace string, items []domain.AttachmentREQ) (string, []llm.Image) {
	if len(items) == 0 {
		return "", nil
	}
	if len(items) > attachMaxFiles {
		items = items[:attachMaxFiles]
	}
	var b strings.Builder
	images := []llm.Image{}
	for _, it := range items {
		// 粘贴的图片：前端直接带 base64，没有路径
		if it.Path == "" && it.Image != "" {
			mime, ok := isImageName(it.Name)
			if !ok {
				mime = "image/png"
			}
			if len(images) >= imageMaxPerMsg {
				b.WriteString(fmt.Sprintf("### %s\n（图片最多带 %d 张，这张没有发送）\n\n", it.Name, imageMaxPerMsg))
				continue
			}
			images = append(images, llm.Image{MIME: mime, Base64: it.Image})
			b.WriteString(fmt.Sprintf("### %s\n（用户粘贴的截图，见随消息附带的图片）\n\n", it.Name))
			continue
		}
		if it.Path == "" {
			continue
		}
		full, err := pkg.SafeJoin(workspace, it.Path)
		if err != nil {
			b.WriteString(fmt.Sprintf("### %s\n（读不到：不在工作目录内）\n\n", it.Name))
			continue
		}
		// 工作目录里的图片：读成 base64 走识图通道
		if mime, ok := isImageName(it.Name); ok {
			raw, err := os.ReadFile(full)
			if err != nil || len(raw) == 0 {
				b.WriteString(fmt.Sprintf("### %s\n（读不到或内容为空）\n\n", it.Name))
				continue
			}
			if len(raw) > imageMaxBytes {
				b.WriteString(fmt.Sprintf("### %s\n（图片超过 5MB，没有发送）\n\n", it.Name))
				continue
			}
			if len(images) >= imageMaxPerMsg {
				b.WriteString(fmt.Sprintf("### %s\n（图片最多带 %d 张，这张没有发送）\n\n", it.Name, imageMaxPerMsg))
				continue
			}
			images = append(images, llm.Image{MIME: mime, Base64: base64.StdEncoding.EncodeToString(raw)})
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
	if b.Len() == 0 && len(images) == 0 {
		return "", nil
	}
	head := ""
	if b.Len() > 0 {
		head = "以下是用户引用的文件内容：\n\n" + b.String() + "---\n\n"
	}
	return head, images
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
