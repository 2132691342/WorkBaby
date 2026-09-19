package api

import "WorkBaby/internal/domain"

// ListDocs 返回内置文档列表。
func (h *Handler) ListDocs() ([]domain.DocItemRESP, error) {
	return h.docsSvc.List(h.ctx)
}

// GetDoc 返回单篇内置文档详情。
func (h *Handler) GetDoc(name string) (*domain.DocDetailRESP, error) {
	return h.docsSvc.Get(h.ctx, name)
}
