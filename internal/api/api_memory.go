package api

import "WorkBaby/internal/domain"

// GetMemoryOverview 记忆概览（文件路径 + 条目与分节计数）。
func (h *Handler) GetMemoryOverview() (domain.MemoryOverviewRESP, error) {
	return h.memProxy.Overview(), nil
}

// ListMemory 记忆条目列表；section 为空列出全部。
func (h *Handler) ListMemory(section string) (domain.MemoryListRESP, error) {
	return h.memProxy.List(section), nil
}

// SearchMemory 检索记忆条目。
func (h *Handler) SearchMemory(query string, topK int) ([]domain.MemoryEntryRESP, error) {
	return h.memProxy.Search(query, topK), nil
}

// GetMemoryText 读 MEMORY.md 全文（设置页编辑器）。
func (h *Handler) GetMemoryText() (string, error) {
	return h.memProxy.Read(), nil
}

// AppendMemory 追加一条记忆。
func (h *Handler) AppendMemory(req domain.MemoryWriteREQ) error {
	return h.memProxy.Append(&req)
}

// DeleteMemory 删除一条记忆（分节 + 正文精确匹配）。
func (h *Handler) DeleteMemory(req domain.MemoryDeleteREQ) error {
	return h.memProxy.Delete(&req)
}

// ReplaceMemory 整篇覆盖 MEMORY.md。
func (h *Handler) ReplaceMemory(req domain.MemoryReplaceREQ) error {
	return h.memProxy.Replace(&req)
}
