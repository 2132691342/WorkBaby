package knowledge

import (
	"archive/zip"
	"io"
	"regexp"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	"github.com/ledongthuc/pdf"
)

// 支持的类型：面向办公文档，超出即标记失败并在列表里说明原因。
var supported = map[string]bool{
	"md": true, "txt": true, "csv": true, "log": true, "json": true, "yml": true, "yaml": true,
	"html": true, "htm": true, "pdf": true, "docx": true, "xlsx": true,
}

// RE2 不支持反向引用，脚本与样式各写一条。
var (
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	blankRe    = regexp.MustCompile(`\n{3,}`)
	cellRe     = regexp.MustCompile(`(?s)<(t|v)[^>]*>(.*?)</(t|v)>`)
	paraBreak  = regexp.MustCompile(`(?s)</w:p>`)
	docxTextRe = regexp.MustCompile(`(?s)<w:t[^>]*>(.*?)</w:t>`)
)

// LoadText 按扩展名抽纯文本。
func LoadText(path string) (string, error) {
	ext := pkg.Ext(path)
	if !supported[ext] {
		return "", domain.ErrDocType
	}
	switch ext {
	case "pdf":
		return loadPDF(path)
	case "docx":
		return loadZipEntry(path, "word/document.xml", docxToText)
	case "xlsx":
		return loadXLSX(path)
	case "html", "htm":
		raw, err := pkg.ReadText(path)
		if err != nil {
			return "", err
		}
		return htmlToText(raw), nil
	default:
		return pkg.ReadText(path)
	}
}

func loadPDF(path string) (string, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return "", pkg.Wrap(7101, "打开 PDF 失败", err)
	}
	defer f.Close()

	var b strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(text)
		b.WriteString("\n")
	}
	out := b.String()
	if strings.TrimSpace(out) == "" {
		return "", pkg.New(7102, "这个 PDF 里没有可提取的文字", "可能是扫描件")
	}
	return out, nil
}

func loadZipEntry(path, entry string, conv func(string) string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", pkg.Wrap(7101, "打开文档失败", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", pkg.Wrap(7101, "读取文档内容失败", err)
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return "", pkg.Wrap(7101, "读取文档内容失败", err)
		}
		return conv(string(raw)), nil
	}
	return "", pkg.New(7101, "文档里没有找到正文", entry)
}

// docxToText 抽 Word 正文：先按段落断行，再抽 <w:t> 文本。
func docxToText(xml string) string {
	parts := paraBreak.Split(xml, -1)
	var b strings.Builder
	for _, p := range parts {
		line := ""
		for _, m := range docxTextRe.FindAllStringSubmatch(p, -1) {
			line += m[1]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(strings.TrimSpace(line))
		b.WriteString("\n")
	}
	return b.String()
}

func loadXLSX(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", pkg.Wrap(7101, "打开表格失败", err)
	}
	defer r.Close()

	shared := []string{}
	for _, f := range r.File {
		if f.Name == "xl/sharedStrings.xml" {
			raw, err := readZipFile(f)
			if err != nil {
				return "", err
			}
			for _, m := range cellRe.FindAllStringSubmatch(string(raw), -1) {
				shared = append(shared, decodeEntities(m[2]))
			}
			break
		}
	}

	var b strings.Builder
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/sheet") {
			continue
		}
		raw, err := readZipFile(f)
		if err != nil {
			continue
		}
		b.WriteString(sheetToText(string(raw), shared))
	}
	out := b.String()
	if strings.TrimSpace(out) == "" {
		return "", pkg.New(7102, "这个表格里没有可提取的内容", "")
	}
	return out, nil
}

// sheetToText 把 sheet 的单元格按行拼成制表符分隔的文本。
func sheetToText(xml string, shared []string) string {
	rows := regexp.MustCompile(`(?s)<row[^>]*>(.*?)</row>`).FindAllStringSubmatch(xml, -1)
	var b strings.Builder
	for _, row := range rows {
		cells := regexp.MustCompile(`(?s)<c[^>]*?(?:\st="([^"]*)")?[^>]*>(.*?)</c>`).FindAllStringSubmatch(row[1], -1)
		values := []string{}
		for _, c := range cells {
			m := cellRe.FindStringSubmatch(c[2])
			if m == nil {
				continue
			}
			value := decodeEntities(m[2])
			if c[1] == "s" {
				if idx := atoi(value); idx >= 0 && idx < len(shared) {
					value = shared[idx]
				}
			}
			values = append(values, value)
		}
		if len(values) == 0 {
			continue
		}
		b.WriteString(strings.Join(values, "\t"))
		b.WriteString("\n")
	}
	return b.String()
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, pkg.Wrap(7101, "读取文档内容失败", err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func htmlToText(s string) string {
	s = scriptRe.ReplaceAllString(s, "")
	s = styleRe.ReplaceAllString(s, "")
	s = tagRe.ReplaceAllString(s, "\n")
	return collapseBlank(decodeEntities(s))
}

func decodeEntities(s string) string {
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}

func collapseBlank(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return blankRe.ReplaceAllString(s, "\n\n")
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return -1
	}
	return n
}
