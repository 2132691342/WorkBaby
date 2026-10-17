//go:build windows

package api

import (
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OpenFileDialog 打开文件选择框；filter 为分号分隔的 pattern，空串表示不限类型。
// 不在这里写死扩展名白名单：助手实际能读的类型远多于办公文档。
func (h *Handler) OpenFileDialog(title, filter string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: title}
	if f := strings.TrimSpace(filter); f != "" {
		opts.Filters = []runtime.FileFilter{{DisplayName: "可选文件", Pattern: f}}
	}
	return runtime.OpenFileDialog(h.ctx, opts)
}

// OpenDirectoryDialog 打开目录选择框（工作目录、知识库目录）。
func (h *Handler) OpenDirectoryDialog(title string) (string, error) {
	return runtime.OpenDirectoryDialog(h.ctx, runtime.OpenDialogOptions{Title: title})
}
