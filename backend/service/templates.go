package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Templates 是提示词模板库：一份模板一段可复用的提示词，用 /name 触发后填进输入框。
// 存成 data\templates 下的 md 文件（带 YAML 头），和技能一样「文件即数据」——
// 用户能直接用编辑器改，也能整个目录拷走备份。
type Templates struct {
	// Layout 目录布局（取 TemplatesDir）
	Layout pkg.Layout
}

// NewTemplates 造一个模板门面。
func NewTemplates(layout pkg.Layout) *Templates {
	return &Templates{Layout: layout}
}

// List 扫全部模板，按名字排序。
func (t *Templates) List() domain.TemplateListRESP {
	entries, err := os.ReadDir(t.Layout.TemplatesDir)
	if err != nil {
		return domain.TemplateListRESP{Rows: []domain.PromptTemplateVO{}}
	}
	rows := make([]domain.PromptTemplateVO, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		if row, ok := t.readOne(filepath.Join(t.Layout.TemplatesDir, entry.Name())); ok {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TemplateName < rows[j].TemplateName })
	return domain.TemplateListRESP{Rows: rows}
}

// readOne 读一份模板；坏文件跳过而不是让整张清单报错——一份手改坏的模板不该拖垮全部。
func (t *Templates) readOne(path string) (domain.PromptTemplateVO, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.PromptTemplateVO{}, false
	}
	fm := pkg.ParseFrontmatter(string(raw))
	name := fm.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return domain.PromptTemplateVO{
		TemplateName: name,
		TemplateDesc: fm.Description,
		TemplateBody: templateBody(string(raw)),
		TemplateArgs: fm.Args,
	}, true
}

// Commands 把模板列成斜杠命令（source=prompt），前端斜杠菜单与命令面板直接收。
func (t *Templates) Commands() []domain.SlashCommandVO {
	rows := t.List().Rows
	out := make([]domain.SlashCommandVO, 0, len(rows))
	for _, row := range rows {
		desc := row.TemplateDesc
		if row.TemplateArgs != "" {
			desc = strings.TrimSpace(row.TemplateArgs + " · " + desc)
		}
		out = append(out, domain.SlashCommandVO{
			CommandName:   row.TemplateName,
			CommandSource: "prompt",
			CommandDesc:   desc,
			CommandBody:   row.TemplateBody,
		})
	}
	return out
}

// Save 写一份模板（新建或覆盖）。父目录不存在就建。
func (t *Templates) Save(req domain.PromptTemplateREQ) error {
	name := sanitizeTemplateName(req.TemplateName)
	if name == "" {
		return pkg.Err(pkg.CodeBadRequest, "模板名不能为空", req.TemplateName)
	}
	if err := os.MkdirAll(t.Layout.TemplatesDir, 0o755); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "模板目录创建失败", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	if req.TemplateDesc != "" {
		b.WriteString("description: " + oneLine(req.TemplateDesc) + "\n")
	}
	if req.TemplateArgs != "" {
		b.WriteString("args: " + oneLine(req.TemplateArgs) + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimLeft(req.TemplateBody, "\n"))
	if !strings.HasSuffix(req.TemplateBody, "\n") {
		b.WriteString("\n")
	}
	file := filepath.Join(t.Layout.TemplatesDir, name+".md")
	if err := pkg.WriteFileAtomic(file, []byte(b.String())); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "模板写入失败", err)
	}
	return nil
}

// Delete 删一份模板；不存在不算错（幂等）。
func (t *Templates) Delete(name string) error {
	safe := sanitizeTemplateName(name)
	if safe == "" {
		return pkg.Err(pkg.CodeBadRequest, "模板名不能为空", name)
	}
	file := filepath.Join(t.Layout.TemplatesDir, safe+".md")
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return pkg.WrapErr(pkg.CodeFileIO, "模板删除失败", err)
	}
	return nil
}

// templateBody 取头部之后的正文：头部用 `---` 包，正文是闭合行之后的所有内容。
func templateBody(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return strings.TrimSpace(text)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

// sanitizeTemplateName 只留小写字母/数字/连字符，且不能是纯点：模板名要能当文件名用。
func sanitizeTemplateName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}

// oneLine 压成一行：YAML 头里的标量值不能带换行，否则会把头部结构写坏。
func oneLine(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
