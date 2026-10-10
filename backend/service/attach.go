package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
)

// 引用文件的读入上限：够看清一个表格/文档的骨架，又不至于把上下文一次撑爆。
const (
	attachMaxFiles = 5
	attachMaxBytes = 64 << 10
	imageMaxBytes  = 5 << 20
	imageMaxPerMsg = 4
	// 读入上限：先看大小再读，@ 一个 2GB 日志不该让进程内存先尖峰一次。
	// 落上下文的上限是 attachMaxBytes，这层只是「别把整文件搬进内存」的闸门。
	attachReadMaxBytes = 32 << 20
)

// errTooBig 表示文件超过读入上限，调用方据此给出可读的提示而不是「读不到」。
var errTooBig = pkg.New(1013, "文件超过读入上限", "")

// readCapped 先看大小再读：os.ReadFile 是把整文件搬进内存，
// 引用面板里一个几百 MB 的日志就足以让桌面进程内存尖峰。
func readCapped(path string, limit int) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, pkg.New(1008, "目标是目录，不是文件", "")
	}
	if fi.Size() > int64(limit) {
		return nil, errTooBig
	}
	return os.ReadFile(path)
}

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
// 图片走 base64 image block，不进文本块；读不到就跳过并说明。
func buildAttachment(workspace string, items []domain.AttachmentREQ) (string, []llm.Image) {
	if len(items) == 0 {
		return "", nil
	}
	// 超出数量上限必须留痕：静默丢弃会让模型说「没看到那个文件」，
	// 用户两头都找不到原因（为什么少了一个）。
	note := ""
	if len(items) > attachMaxFiles {
		note = fmt.Sprintf("（本次引用了 %d 个文件，一次最多读 %d 个，其余 %d 个没有带上）\n\n",
			len(items), attachMaxFiles, len(items)-attachMaxFiles)
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
			raw, err := readCapped(full, imageMaxBytes)
			if err != nil || len(raw) == 0 {
				b.WriteString(fmt.Sprintf("### %s\n（%s）\n\n", it.Name, attachReason(err, "图片超过 5MB，没有发送")))
				continue
			}
			if len(images) >= imageMaxPerMsg {
				b.WriteString(fmt.Sprintf("### %s\n（图片最多带 %d 张，这张没有发送）\n\n", it.Name, imageMaxPerMsg))
				continue
			}
			images = append(images, llm.Image{MIME: mime, Base64: base64.StdEncoding.EncodeToString(raw)})
			continue
		}
		raw, err := readCapped(full, attachReadMaxBytes)
		if err != nil || len(raw) == 0 {
			b.WriteString(fmt.Sprintf("### %s\n（%s）\n\n", it.Name, attachReason(err, "文件超过 32MB，没有读入")))
			continue
		}
		// Word / Excel / PDF 是二进制文档：按字节当文本读只会塞进一堆乱码，
		// 明说做不到并给出可行路径，别让模型拿着乱码瞎猜。
		if pkg.LooksBinary(raw) {
			b.WriteString(fmt.Sprintf("### %s\n（这是二进制文档，直接引用读不出内容：把它加进知识库再检索，或用 python 处理）\n\n", it.Name))
			continue
		}
		body := pkg.DecodeText(raw)
		if len(body) > attachMaxBytes {
			body = cutRunes(body, attachMaxBytes)
		}
		b.WriteString("### " + filepath.Base(full) + "\n\n```\n" + body + "\n```\n\n")
	}
	head := note
	if b.Len() > 0 {
		head += "以下是用户引用的文件内容：\n\n" + b.String() + "---\n\n"
	}
	if head == "" && len(images) == 0 {
		return "", nil
	}
	return head, images
}

// attachReason 把读文件的失败翻成人话：模型要靠这句话告诉用户「为什么没带上」，
// 一句笼统的「读不到」会让用户反复重发同一个文件。
func attachReason(err error, bigMsg string) string {
	if err != nil {
		if errors.Is(err, errTooBig) {
			return bigMsg
		}
		if pkg.CodeOf(err) == 1008 {
			return "这是一个目录，引用不了"
		}
	}
	return "读不到或内容为空"
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
