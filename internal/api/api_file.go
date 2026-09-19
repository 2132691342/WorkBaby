package api

import (
	"encoding/base64"
	"net/url"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// SearchFiles 按文件名模糊搜索。
func (h *Handler) SearchFiles(q string) ([]domain.FileRESP, error) {
	return h.fileSvc.Search(h.ctx, q)
}

// ListFiles 托管目录全部文件。
func (h *Handler) ListFiles() ([]domain.FileRESP, error) {
	return h.fileSvc.ListFiles(h.ctx)
}

// UploadFile 上传本地文件（srcPath 由前端文件对话框选路径；复制到托管目录）。
func (h *Handler) UploadFile(name, srcPath, sessionID, folderID string) (domain.FileRESP, error) {
	if srcPath == "" {
		return domain.FileRESP{}, pkg.New(1203, "source path required", "")
	}
	return h.fileSvc.Upload(h.ctx, name, srcPath, sessionID, folderID)
}

// UploadFileData 上传内存字节（粘贴 / 拖拽的图片没有本地路径，只能传内容）。
func (h *Handler) UploadFileData(req domain.UploadDataREQ) (domain.FileRESP, error) {
	if req.DataBase64 == "" {
		return domain.FileRESP{}, pkg.New(1203, "file data required", "")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return domain.FileRESP{}, pkg.Wrap(1202, "decode file data failed", err)
	}
	return h.fileSvc.UploadData(h.ctx, req.Name, data, req.SessionID, req.FolderID)
}

// DeleteFile 删除文件（磁盘 + 记录）。
func (h *Handler) DeleteFile(id string) (map[string]any, error) {
	if err := h.fileSvc.Delete(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deleted": true}, nil
}

// GetFilePreviewURL 生成预览 URL（背景图等场景；AssetServer /files/files/{id}）。
func (h *Handler) GetFilePreviewURL(id string) (map[string]any, error) {
	if _, err := h.fileSvc.Get(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"url": "/files/files/" + id}, nil
}

// ListWorkspaceFiles 会话工作区文件面板列表。
func (h *Handler) ListWorkspaceFiles(sessionID string) ([]domain.WorkspaceFileItem, error) {
	return h.workspaceSvc.ListFiles(h.ctx, sessionID)
}

// ListWorkspaceDir 懒加载列工作区单层目录（真实磁盘内容；路径相对工作区根）。
func (h *Handler) ListWorkspaceDir(sessionID, path string) (domain.WorkspaceListRESP, error) {
	return h.workspaceSvc.ListDir(h.ctx, sessionID, path)
}

// ReadWorkspaceFile 读取工作区单文件内容（沙箱校验；返回 base64 便于绑定跨边界传输）。
func (h *Handler) ReadWorkspaceFile(sessionID, path string) (string, error) {
	bs, _, err := h.workspaceSvc.ReadFile(h.ctx, sessionID, path)
	if err != nil {
		return "", err
	}
	return url.PathEscape(string(bs)), nil
}

// RenameWorkspaceEntry 工作区内同级重命名（文件/目录）。
func (h *Handler) RenameWorkspaceEntry(sessionID, path, newName string) (map[string]any, error) {
	if err := h.workspaceSvc.RenameEntry(h.ctx, sessionID, path, newName); err != nil {
		return nil, err
	}
	return map[string]any{"renamed": true}, nil
}

// CopyWorkspaceEntry 复制文件为同级副本，返回新相对路径。
func (h *Handler) CopyWorkspaceEntry(sessionID, path string) (map[string]any, error) {
	newPath, err := h.workspaceSvc.CopyEntry(h.ctx, sessionID, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": newPath}, nil
}

// DeleteWorkspaceEntry 删除文件/目录（目录递归；沙箱校验）。
func (h *Handler) DeleteWorkspaceEntry(sessionID, path string) (map[string]any, error) {
	if err := h.workspaceSvc.DeleteEntry(h.ctx, sessionID, path); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true}, nil
}
