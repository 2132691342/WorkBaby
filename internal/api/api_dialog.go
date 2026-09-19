package api

import (
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"WorkBaby/internal/pkg"
)

// OpenFileDialog 单选文件，返回选中绝对路径；用户取消返回空串。
// 用扁平参数而非 struct：Wails 为 struct 生成的 TS 是带 convertValues 的 class，前端无法传对象字面量。
func (h *Handler) OpenFileDialog(title, pattern string) (string, error) {
	opts := wruntime.OpenDialogOptions{Title: title}
	if pattern != "" {
		opts.Filters = []wruntime.FileFilter{{DisplayName: pattern, Pattern: pattern}}
	}
	return wruntime.OpenFileDialog(h.ctx, opts)
}

// OpenDirectoryDialog 选目录，返回绝对路径；用户取消返回空串。
// 供会话工作区绑定（WorkspacePickerDialog）调原生壳，避免让用户手敲绝对路径。
func (h *Handler) OpenDirectoryDialog(title, defaultPath string) (string, error) {
	return wruntime.OpenDirectoryDialog(h.ctx, wruntime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultPath,
	})
}

// OpenExternal 用系统默认浏览器打开外部链接（http/https/file）。
//
// 聊天正文与产物卡里的链接一律经此打开：WebView 内直接导航会把整个 SPA
// 页面替换掉，应用随之不可操作。仅放行安全 scheme。
func (h *Handler) OpenExternal(url string) error {
	u := strings.TrimSpace(url)
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "file:///") {
		return pkg.New(1003, "不支持的链接协议", u)
	}
	if h.ctx == nil {
		return pkg.New(1004, "应用未就绪", "")
	}
	wruntime.BrowserOpenURL(h.ctx, u)
	return nil
}
