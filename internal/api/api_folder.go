package api

import (
	"WorkBaby/internal/domain"
)

// ListFolders 列出指定父目录下的子文件夹（parentId 空 = 根目录）。
func (h *Handler) ListFolders(parentID string) ([]domain.FolderRESP, error) {
	var pid *string
	if parentID != "" {
		pid = &parentID
	}
	return h.folderSvc.ListByParent(h.ctx, pid)
}

// GetFolderTree 文件夹树；workspaceId 非空时仅返回绑定该 workspace 的树。
func (h *Handler) GetFolderTree(workspaceID string) ([]domain.FolderTreeNode, error) {
	if workspaceID != "" {
		return h.folderSvc.TreeForWorkspace(h.ctx, workspaceID)
	}
	return h.folderSvc.Tree(h.ctx)
}

// CreateFolder 新建文件夹。
func (h *Handler) CreateFolder(req domain.FolderREQ) (domain.FolderRESP, error) {
	if req.Name == "" {
		return domain.FolderRESP{}, domain.ErrFolderNameEmpty
	}
	return h.folderSvc.Create(h.ctx, req)
}

// UpdateFolder 重命名/移动。
func (h *Handler) UpdateFolder(id string, req domain.FolderREQ) (domain.FolderRESP, error) {
	if req.Name == "" {
		return domain.FolderRESP{}, domain.ErrFolderNameEmpty
	}
	return h.folderSvc.Update(h.ctx, id, req)
}

// DeleteFolder 删除文件夹（非空拒绝）。
func (h *Handler) DeleteFolder(id string) (map[string]any, error) {
	if err := h.folderSvc.Delete(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deleted": true}, nil
}
