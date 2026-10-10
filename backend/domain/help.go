// 内置帮助文档的对外形态（只读静态资源，不落库）。
package domain

import "WorkBaby/backend/pkg"

// ErrHelpDocNotFound 帮助文档不存在或读取失败（内置资源，正常不该发生）。
var ErrHelpDocNotFound = pkg.New(1012, "这篇帮助文档不存在", "")

// HelpDocVO 帮助文档目录项：name 为文件标识，title 取正文首行标题。
type HelpDocVO struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// HelpDocRESP 单篇帮助文档：content 为原始 Markdown，渲染一律在前端完成。
type HelpDocRESP struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Content string `json:"content"`
}
