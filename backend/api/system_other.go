//go:build !windows

package api

// 非 Windows 上系统能力为空实现：本应用仅面向 Windows，这里只为保持可编译。

// OpenFileDialog 打开文件对话框；非 Windows 恒返回空。
func (h *Handler) OpenFileDialog(title, filter string) (string, error) { return "", nil }

// OpenDirectoryDialog 打开目录对话框；非 Windows 恒返回空。
func (h *Handler) OpenDirectoryDialog(title string) (string, error) { return "", nil }
