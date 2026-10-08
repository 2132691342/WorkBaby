//go:build windows

package api

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OpenFileDialog 打开文件选择框（知识库导入、附件）。
func (h *Handler) OpenFileDialog(title, filter string) (string, error) {
	return runtime.OpenFileDialog(h.ctx, runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{{
			DisplayName: "支持的文档",
			Pattern:     "*.md;*.txt;*.csv;*.pdf;*.docx;*.xlsx;*.html",
		}},
	})
}

// OpenDirectoryDialog 打开目录选择框（工作目录、知识库目录）。
func (h *Handler) OpenDirectoryDialog(title string) (string, error) {
	return runtime.OpenDirectoryDialog(h.ctx, runtime.OpenDialogOptions{Title: title})
}
