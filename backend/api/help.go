package api

import (
	"io/fs"
	"sort"
	"strings"

	"WorkBaby/assets"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListHelpDocs 返回内置帮助文档目录：新手引导排最前，其余按文件名排序。
func (h *Handler) ListHelpDocs(c *gin.Context) {
	entries, err := fs.ReadDir(assets.Docs, "docs")
	if err != nil {
		fail(c, pkg.Wrap(1012, "读取帮助文档失败", err))
		return
	}
	items := make([]domain.HelpDocVO, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		items = append(items, domain.HelpDocVO{Name: name, Title: docTitle(name)})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return docRank(items[i].Name) < docRank(items[j].Name)
	})
	ok(c, items)
}

// HelpDocContent 读单篇帮助文档正文。
func (h *Handler) HelpDocContent(c *gin.Context) {
	name := c.Param("name")
	if !validDocName(name) {
		fail(c, domain.ErrHelpDocNotFound)
		return
	}
	raw, err := fs.ReadFile(assets.Docs, "docs/"+name+".md")
	if err != nil {
		fail(c, domain.ErrHelpDocNotFound)
		return
	}
	ok(c, domain.HelpDocRESP{Name: name, Title: docTitle(name), Content: string(raw)})
}

// docTitle 取正文首行的一级标题；没有标题则退回文件名。
func docTitle(name string) string {
	raw, err := fs.ReadFile(assets.Docs, "docs/"+name+".md")
	if err != nil {
		return name
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if t, found := strings.CutPrefix(line, "# "); found {
			return strings.TrimSpace(t)
		}
		break
	}
	return name
}

// validDocName 只放行纯文件名：挡掉路径分隔与 .. 穿越，不依赖上层校验。
func validDocName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// docRank 目录排序权重：新手引导固定最前（数值越小越靠前）。
func docRank(name string) int {
	switch name {
	case "getting-started":
		return 0
	case "models":
		return 1
	case "tools-and-permissions":
		return 2
	case "knowledge-and-skills":
		return 3
	case "faq":
		return 4
	}
	return 100
}
