package api

import (
	"WorkBaby/internal/domain"
)

// AddKnowledgeDoc 上传文档并后台索引。
func (h *Handler) AddKnowledgeDoc(req domain.KnowledgeDocREQ) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.AddDoc(h.ctx, &req)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ImportKnowledgeFile 本地文件导入受管知识库（复制进 {home}/knowledge 后注册 + 后台索引）。
// 参数：name（文档名，可空自动取文件名）、sourcePath（原生对话框返回的绝对路径）、folderID（可空空串）。
func (h *Handler) ImportKnowledgeFile(name, sourcePath, folderID string) (domain.KnowledgeDocRESP, error) {
	var f *string
	if folderID != "" {
		f = &folderID
	}
	p, err := h.knowledgeSvc.ImportLocalFile(h.ctx, name, sourcePath, f)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ListKnowledgeDocs 文档列表（含索引状态）。
func (h *Handler) ListKnowledgeDocs() ([]domain.KnowledgeDocRESP, error) {
	return h.knowledgeSvc.ListDocs(h.ctx)
}

// GetKnowledgeDoc 单个文档（轮询索引状态用）。
func (h *Handler) GetKnowledgeDoc(id string) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.GetDoc(h.ctx, id)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// DeleteKnowledgeDoc 删除文档。
func (h *Handler) DeleteKnowledgeDoc(id string) error {
	return h.knowledgeSvc.DeleteDoc(h.ctx, id)
}

// ReindexKnowledgeDoc 重新索引。
func (h *Handler) ReindexKnowledgeDoc(id string) error {
	return h.knowledgeSvc.ReindexDoc(h.ctx, id)
}

// SearchKnowledge 检索知识库。
func (h *Handler) SearchKnowledge(query string, topK int) ([]domain.KnowledgeHitRESP, error) {
	return h.knowledgeSvc.Search(h.ctx, query, topK)
}

// UpdateKnowledgeDoc 更新知识文档（改名/改来源+重索引）。
func (h *Handler) UpdateKnowledgeDoc(id string, req domain.KnowledgeDocREQ) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.UpdateDoc(h.ctx, id, &req)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ListKnowledgeGroups 知识库分组列表。
func (h *Handler) ListKnowledgeGroups() ([]string, error) {
	return h.knowledgeSvc.ListGroups(h.ctx)
}

// ListKnowledgeByGroup 按来源类型过滤。
func (h *Handler) ListKnowledgeByGroup(group string) ([]domain.KnowledgeDocRESP, error) {
	return h.knowledgeSvc.ListByGroup(h.ctx, group)
}
