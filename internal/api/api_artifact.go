package api

import "WorkBaby/internal/domain"

// ListSessionArtifacts 会话产出工件清单（最近更新在前）。
func (h *Handler) ListSessionArtifacts(sessionID string, limit int) (domain.ArtifactListRESP, error) {
	return h.artifactSvc.List(h.ctx, sessionID, limit)
}

// DeleteSessionArtifact 删除工件登记（不删磁盘文件）。
func (h *Handler) DeleteSessionArtifact(id string) error {
	return h.artifactSvc.Delete(h.ctx, id)
}
