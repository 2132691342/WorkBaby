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

// ClipboardGetText 读剪贴板。
func (h *Handler) ClipboardGetText() string {
	text, err := runtime.ClipboardGetText(h.ctx)
	if err != nil {
		return ""
	}
	return text
}

// ClipboardSetText 写剪贴板。
func (h *Handler) ClipboardSetText(text string) bool {
	runtime.ClipboardSetText(h.ctx, text)
	return true
}

// WindowMinimise 最小化。
func (h *Handler) WindowMinimise() { runtime.WindowMinimise(h.ctx) }

// WindowToggleMaximise 最大化 / 还原。
func (h *Handler) WindowToggleMaximise() { runtime.WindowToggleMaximise(h.ctx) }

// WindowHide 隐藏窗口（关闭到托盘）。
func (h *Handler) WindowHide() { runtime.WindowHide(h.ctx) }

// WindowShow 显示窗口（托盘唤起）。
func (h *Handler) WindowShow() { runtime.WindowShow(h.ctx) }
