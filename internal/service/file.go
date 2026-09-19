package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// FileService 文件编排：上传（复制到 {home}/files/{id}）+ 搜索 + 删除 + 磁盘解析。
type FileService struct {
	repo    *repo.FileRepo
	fileDir string // {home}/files
}

func NewFileService(r *repo.FileRepo, fileDir string) *FileService {
	return &FileService{repo: r, fileDir: fileDir}
}

// Upload 将本地文件复制进托管目录并落库（srcPath 由前端文件对话框给出）。
func (s *FileService) Upload(ctx context.Context, name, srcPath, sessionID, folderID string) (domain.FileRESP, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return domain.FileRESP{}, pkg.Wrap(1202, "open source file failed", err)
	}
	defer src.Close()
	if name == "" {
		name = filepath.Base(srcPath)
	}
	return s.save(ctx, name, src, sessionID, folderID)
}

// UploadData 从内存字节落库（粘贴 / 拖拽的图片拿不到本地路径，只能传内容）。
func (s *FileService) UploadData(ctx context.Context, name string, data []byte, sessionID, folderID string) (domain.FileRESP, error) {
	if len(data) == 0 {
		return domain.FileRESP{}, pkg.New(1203, "empty file data", "")
	}
	return s.save(ctx, name, bytes.NewReader(data), sessionID, folderID)
}

// ReadDataURL 读取图片内容为 data URI（多模态模型输入用）；非图片返回空串。
func (s *FileService) ReadDataURL(ctx context.Context, id string) (string, error) {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(f.MimeType, "image/") {
		return "", nil
	}
	raw, err := os.ReadFile(f.StoragePath)
	if err != nil {
		return "", pkg.Wrap(1202, "read file failed", err)
	}
	return "data:" + f.MimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// save 落盘 + 落库（Upload / UploadData 共用）。
func (s *FileService) save(ctx context.Context, name string, src io.Reader, sessionID, folderID string) (domain.FileRESP, error) {
	if err := pkg.EnsureDir(s.fileDir); err != nil {
		return domain.FileRESP{}, err
	}
	id := pkg.NewID(domain.IDFile)
	target := filepath.Join(s.fileDir, id)
	dst, err := os.Create(target)
	if err != nil {
		return domain.FileRESP{}, pkg.Wrap(1202, "create dest file failed", err)
	}
	n, cerr := io.Copy(dst, src)
	dst.Close()
	if cerr != nil {
		_ = os.Remove(target)
		return domain.FileRESP{}, pkg.Wrap(1202, "copy file failed", cerr)
	}
	mime := detectMime(name)
	f := &domain.FileDO{
		ID:           id,
		Name:         name,
		OriginalName: name,
		FileType:     domain.DetectFileType(name, mime),
		MimeType:     mime,
		Size:         n,
		Status:       domain.FileStatusUploaded,
		UserID:       domain.LocalUserID,
		SessionID:    sessionID,
		FolderID:     folderID,
		StoragePath:  target,
	}
	if err := s.repo.Create(ctx, f); err != nil {
		_ = os.Remove(target)
		return domain.FileRESP{}, err
	}
	return toFileRESP(f), nil
}

// Get 按 ID 查询元数据。
func (s *FileService) Get(ctx context.Context, id string) (domain.FileRESP, error) {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.FileRESP{}, err
	}
	return toFileRESP(f), nil
}

// Search 按文件名模糊搜索。
func (s *FileService) Search(ctx context.Context, q string) ([]domain.FileRESP, error) {
	rows, err := s.repo.SearchByName(ctx, domain.LocalUserID, q)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FileRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toFileRESP(&rows[i]))
	}
	return out, nil
}

// Delete 删除磁盘文件 + 记录。
func (s *FileService) Delete(ctx context.Context, id string) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if f.StoragePath != "" {
		_ = os.Remove(f.StoragePath)
	}
	return s.repo.Delete(ctx, id)
}

// DiskPath 解析磁盘路径（AssetServer 预览/下载用）；内容缺失返回 ErrFileNotFound。
func (s *FileService) DiskPath(ctx context.Context, id string) (string, error) {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(f.StoragePath); err != nil {
		return "", domain.ErrFileNotFound
	}
	return f.StoragePath, nil
}

// ListFiles 列出托管目录全部文件（预览/工作区面板）。
func (s *FileService) ListFiles(ctx context.Context) ([]domain.FileRESP, error) {
	rows, err := s.repo.ListByUser(ctx, domain.LocalUserID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FileRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toFileRESP(&rows[i]))
	}
	return out, nil
}

func toFileRESP(f *domain.FileDO) domain.FileRESP {
	return domain.FileRESP{
		ID:           f.ID,
		Name:         f.Name,
		OriginalName: f.OriginalName,
		FileType:     f.FileType,
		MimeType:     f.MimeType,
		Size:         f.Size,
		Status:       f.Status,
		SessionID:    f.SessionID,
		FolderID:     f.FolderID,
		CreatedAt:    f.CreatedAt,
	}
}

func detectMime(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".md", ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
