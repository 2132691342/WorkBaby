package rag

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"

	"WorkBaby/internal/pkg"
)

const (
	// maxFileBytes 单文件最大读取字节（防 OOM）。
	//
	// 与 service.KnowledgeService.MaxManagedDocBytes 对齐：导入允许 ≤ 60MB，
	// loader 必须能读完整文件否则索引必然失败。URL 抓取有更严的 urlMaxBytes（5MB），
	// 与单文件上限独立。
	maxFileBytes = 60 << 20
	// urlTimeout / urlMaxBytes 远程抓取限制。
	urlTimeout  = 30 * time.Second
	urlMaxBytes = 5 << 20
)

// Loader 按来源抽取纯文本。
type Loader interface {
	Supports(mime string) bool
	Load(ctx context.Context, source string) (string, error)
}

// PickLoader 按 MIME / 扩展名选 loader；无匹配返回 nil（调用方报 7006）。
func PickLoader(mime string) Loader {
	m := normalizeMIME(mime)
	for _, l := range []Loader{&TextLoader{}, &PDFLoader{}, &DocxLoader{}, &HTMLLoader{}} {
		if l.Supports(m) {
			return l
		}
	}
	return nil
}

// normalizeMIME 用扩展名补全空 MIME：.md → text/markdown 等。
func normalizeMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch mime {
	case "":
		return ""
	case "text/plain":
		return "text/plain"
	case "text/markdown", "text/x-markdown":
		return "text/markdown"
	case "text/html":
		return "text/html"
	case "application/pdf":
		return "application/pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return mime
}

// ---- TextLoader：txt / md / 无后缀 ----

// TextLoader 纯文本与 Markdown。
type TextLoader struct{}

// Supports 判断是否纯文本（含 JSON / YAML / XML 等结构化文本）。
func (TextLoader) Supports(mime string) bool {
	switch mime {
	case "text/plain", "text/markdown", "text/csv", "application/json",
		"application/yaml", "application/xml", "text/xml":
		return true
	}
	return false
}

// Load 读取文件内容。
func (TextLoader) Load(_ context.Context, source string) (string, error) {
	data, err := readFileLimited(source, maxFileBytes)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ---- HTMLLoader：html / htm ----

// HTMLLoader 剥离标签提取正文。
type HTMLLoader struct{}

// Supports 判断是否 HTML。
func (HTMLLoader) Supports(mime string) bool { return mime == "text/html" }

// Load 读取并清洗。
func (HTMLLoader) Load(_ context.Context, source string) (string, error) {
	data, err := readFileLimited(source, maxFileBytes)
	if err != nil {
		return "", err
	}
	return StripHTML(string(data)), nil
}

// blockTagRes 整块丢弃 script/style 等对检索无意义的内容。
// Go RE2 不支持反向引用（</\1>），逐标签编译。
var blockTagRes = func() []*regexp.Regexp {
	tags := []string{"script", "style", "noscript", "svg", "head"}
	out := make([]*regexp.Regexp, 0, len(tags))
	for _, t := range tags {
		out = append(out, regexp.MustCompile(`(?is)<`+t+`\b.*?</`+t+`>`))
	}
	return out
}()

var blockEndRe = regexp.MustCompile(`(?i)</(p|div|br|h[1-6]|li|tr|table|section|article|blockquote)>`)
var brRe = regexp.MustCompile(`(?i)<br\s*/?>`)
var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// StripHTML 把 HTML 转成保留段落结构的纯文本。
func StripHTML(s string) string {
	for _, re := range blockTagRes {
		s = re.ReplaceAllString(s, "")
	}
	// 块级标签结束 → 段落边界
	s = blockEndRe.ReplaceAllString(s, "\n\n")
	s = brRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return normalizeBlankLines(s)
}

// ---- DocxLoader：解 zip 直接读 word/document.xml ----

// DocxLoader 不引重型库，用 archive/zip 解包。
type DocxLoader struct{}

// Supports 判断是否 docx。
func (DocxLoader) Supports(mime string) bool {
	return mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

// Load 解包并抽取正文。
func (DocxLoader) Load(_ context.Context, source string) (string, error) {
	data, err := readFileLimited(source, maxFileBytes)
	if err != nil {
		return "", err
	}
	return parseDocx(data)
}

func parseDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", pkg.Wrap(7001, "open docx failed", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", pkg.Wrap(7001, "read document.xml failed", err)
		}
		defer rc.Close()
		xmlBody, err := io.ReadAll(io.LimitReader(rc, maxFileBytes))
		if err != nil {
			return "", pkg.Wrap(7001, "read document.xml failed", err)
		}
		return docxText(string(xmlBody)), nil
	}
	return "", pkg.New(7001, "docx missing word/document.xml", "")
}

// docxText 段落边界换行、制表符转空格、剥标签。
func docxText(xmlBody string) string {
	xmlBody = regexp.MustCompile(`(?i)</w:p>`).ReplaceAllString(xmlBody, "\n\n")
	xmlBody = regexp.MustCompile(`(?i)<w:tab[^>]*/?>`).ReplaceAllString(xmlBody, " ")
	xmlBody = tagRe.ReplaceAllString(xmlBody, "")
	return normalizeBlankLines(html.UnescapeString(xmlBody))
}

// ---- PDFLoader ----

// PDFLoader 纯 Go 实现；扫描版 PDF 无文本层时报 7001。
type PDFLoader struct{}

// Supports 判断是否 PDF。
func (PDFLoader) Supports(mime string) bool { return mime == "application/pdf" }

// Load 抽取 PDF 文本。
func (PDFLoader) Load(_ context.Context, source string) (string, error) {
	data, err := readFileLimited(source, maxFileBytes)
	if err != nil {
		return "", err
	}
	return parsePDF(data)
}

func parsePDF(data []byte) (string, error) {
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", pkg.Wrap(7001, "open pdf failed", err)
	}
	var sb strings.Builder
	for i := 1; i <= rd.NumPage(); i++ {
		page := rd.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // 单页解析失败不拖垮整篇
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	out := normalizeBlankLines(sb.String())
	if strings.TrimSpace(out) == "" {
		return "", pkg.New(7001, "pdf has no text layer (scanned?)", "")
	}
	return out, nil
}

// ---- URLLoader：抓取后按 Content-Type 二次分发 ----

// URLLoader 不参与 PickLoader 的 MIME 分发，由调用方按 sourceType 直接选用。
type URLLoader struct{}

// Supports 恒 false（见上）。
func (URLLoader) Supports(string) bool { return false }

// Load 抓取远程 URL 并按响应类型继续解析。
func (URLLoader) Load(ctx context.Context, source string) (string, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return "", pkg.New(7001, "url must start with http(s)://", source)
	}
	cctx, cancel := context.WithTimeout(ctx, urlTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, source, nil)
	if err != nil {
		return "", pkg.Wrap(7001, "build request failed", err)
	}
	req.Header.Set("User-Agent", "WorkBaby/0.1 (+https://local)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", pkg.Wrap(7001, "fetch url failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", pkg.New(7001, "fetch url failed", fmt.Sprintf("status %d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, urlMaxBytes+1))
	if err != nil {
		return "", pkg.Wrap(7001, "read url body failed", err)
	}
	if len(body) > urlMaxBytes {
		return "", pkg.New(7001, "url body too large", fmt.Sprintf("> %d bytes", urlMaxBytes))
	}

	ct := resp.Header.Get("Content-Type")
	mime := ct
	if i := strings.Index(ct, ";"); i >= 0 {
		mime = strings.TrimSpace(ct[:i])
	}
	switch {
	case strings.Contains(mime, "html"):
		return StripHTML(string(body)), nil
	case strings.Contains(mime, "pdf"):
		return parsePDF(body)
	case mime == "" || strings.HasPrefix(mime, "text/"):
		return string(body), nil
	default:
		return "", pkg.New(7006, "unsupported content type", mime)
	}
}

// readFileLimited 读文件并限制大小。
func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, pkg.Wrap(7001, "open file failed", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, pkg.Wrap(7001, "read file failed", err)
	}
	if int64(len(data)) > limit {
		return nil, pkg.New(7001, "file too large", fmt.Sprintf("> %d bytes", limit))
	}
	return data, nil
}

// normalizeBlankLines 去掉行尾空白，折叠连续空行（保留单空行作段落边界）。
func normalizeBlankLines(s string) string {
	var sb strings.Builder
	prevBlank := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		prevBlank = blank
	}
	return strings.TrimSpace(sb.String())
}

// MIMEFromPath 按扩展名推断 MIME（上传时用户不填 mime 的兜底）。
func MIMEFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".log", ".csv":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".html", ".htm":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".xml":
		return "application/xml"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return ""
	}
}
