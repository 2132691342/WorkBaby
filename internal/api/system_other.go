//go:build !windows

package api

// 非 Windows 上系统能力为空实现：本应用仅面向 Windows，这里只为保持可编译。

func (h *Handler) OpenFileDialog(title, filter string) (string, error) { return "", nil }
func (h *Handler) OpenDirectoryDialog(title string) (string, error)    { return "", nil }
func (h *Handler) ClipboardGetText() string                            { return "" }
func (h *Handler) ClipboardSetText(text string) bool                   { return false }
func (h *Handler) WindowMinimise()                                     {}
func (h *Handler) WindowToggleMaximise()                               {}
func (h *Handler) WindowHide()                                         {}
func (h *Handler) WindowShow()                                         {}
