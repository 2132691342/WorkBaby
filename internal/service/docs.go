package service

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/assets"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// DocsService 内置用户文档（assets/docs/*.md，只读）。
type DocsService struct{ fsys fs.FS }

// NewDocsService 注入内置文档文件系统。
func NewDocsService() *DocsService { return &DocsService{fsys: assets.Docs} }

// List 返回全部内置文档（按文件名排序）；name 为不含扩展名的文件名。
func (s *DocsService) List(_ context.Context) ([]domain.DocItemRESP, error) {
	entries, err := fs.ReadDir(s.fsys, "docs")
	if err != nil {
		return nil, pkg.Wrap(2010, "read docs dir failed", err)
	}
	out := make([]domain.DocItemRESP, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		out = append(out, domain.DocItemRESP{Name: name, Title: s.title(name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get 返回单篇文档详情（name 不含扩展名；未命中返回 ErrDocNotFound）。
func (s *DocsService) Get(_ context.Context, name string) (*domain.DocDetailRESP, error) {
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return nil, domain.ErrDocNotFound
	}
	data, err := fs.ReadFile(s.fsys, "docs/"+name+".md")
	if err != nil {
		return nil, domain.ErrDocNotFound
	}
	return &domain.DocDetailRESP{
		Name:    name,
		Title:   s.title(name),
		Content: string(data),
	}, nil
}

// title 取正文首行 `# ` 标题；无则回退文件名。
func (s *DocsService) title(name string) string {
	data, err := fs.ReadFile(s.fsys, "docs/"+name+".md")
	if err != nil {
		return name
	}
	lines := strings.SplitN(string(data), "\n", 2)
	first := strings.TrimSpace(lines[0])
	return strings.TrimSpace(strings.TrimPrefix(first, "#"))
}
