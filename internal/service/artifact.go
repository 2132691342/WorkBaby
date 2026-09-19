package service

import (
	"context"
	"path/filepath"
	"strings"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// ArtifactService 会话产出物登记。
//
// 工件是「引用」而非内容副本：只记路径与元信息，预览经 /files 服务走真实文件。
// 由文件变更旁路自动登记（file_write 成功即 upsert），无需模型额外调用工具。
type ArtifactService struct {
	repo   *repo.ArtifactRepo
	bus    *event.Bus
	events *event.RunEventLog
	root   string // 工作区根（算相对路径）
}

// NewArtifactService 构造；root 为工作区根。
func NewArtifactService(r *repo.ArtifactRepo, bus *event.Bus, root string) *ArtifactService {
	return &ArtifactService{repo: r, bus: bus, root: root}
}

// WithEventLog 启用 run 事件日志：工件事件与 chat 事件共享序号空间。
func (s *ArtifactService) WithEventLog(log *event.RunEventLog) *ArtifactService {
	s.events = log
	return s
}

// ArtifactInput 一次工件登记输入。
type ArtifactInput struct {
	SessionID string
	RunID     string
	Path      string // 磁盘绝对路径
	Size      int64
}

// Register 幂等登记（同文件重复产出只更新元信息）并推送 chat:artifact 事件。
func (s *ArtifactService) Register(ctx context.Context, in ArtifactInput) (*domain.ArtifactDO, error) {
	rel := relativeTo(s.root, in.Path)
	row := &domain.ArtifactDO{
		ID:        pkg.NewID("ART"),
		SessionID: in.SessionID,
		RunID:     in.RunID,
		Kind:      artifactKind(rel),
		Name:      filepath.Base(in.Path),
		RelPath:   rel,
		Path:      in.Path,
		MimeType:  mimeByExt(strings.ToLower(filepath.Ext(in.Path))),
		Size:      in.Size,
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	s.emit(ctx, "chat:artifact", map[string]any{"artifact": toArtifactRESP(row)})
	return row, nil
}

// List 会话工件清单（最近更新在前）。
func (s *ArtifactService) List(ctx context.Context, sessionID string, limit int) (domain.ArtifactListRESP, error) {
	rows, err := s.repo.ListBySession(ctx, sessionID, limit)
	if err != nil {
		return domain.ArtifactListRESP{}, err
	}
	out := domain.ArtifactListRESP{Items: make([]domain.ArtifactRESP, 0, len(rows)), Total: len(rows)}
	for i := range rows {
		out.Items = append(out.Items, toArtifactRESP(&rows[i]))
	}
	return out, nil
}

// Delete 删除工件登记（不动磁盘文件）。
func (s *ArtifactService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// emit 发布工件事件：注入 run_id/session_id、经事件日志分配 seq 后广播。
func (s *ArtifactService) emit(ctx context.Context, name string, payload map[string]any) {
	runID := core.RunIDFromCtx(ctx)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["run_id"] = runID
	payload["session_id"] = core.SessionIDFromCtx(ctx)
	if s.events != nil {
		s.events.Append(runID, name, payload)
	}
	s.bus.Publish(name, payload)
}

// toArtifactRESP DO → 出参（不泄漏磁盘路径）。
func toArtifactRESP(row *domain.ArtifactDO) domain.ArtifactRESP {
	return domain.ArtifactRESP{
		ID:        row.ID,
		SessionID: row.SessionID,
		RunID:     row.RunID,
		Kind:      row.Kind,
		Name:      row.Name,
		RelPath:   row.RelPath,
		MimeType:  row.MimeType,
		Size:      row.Size,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// artifactKind 按扩展名粗分工件类别（面板据此选预览器）。
func artifactKind(rel string) domain.ArtifactKind {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp":
		return domain.ArtifactImage
	case ".md", ".txt", ".pdf", ".doc", ".docx", ".rtf":
		return domain.ArtifactDoc
	case ".go", ".ts", ".js", ".vue", ".py", ".java", ".json", ".yaml", ".yml", ".toml", ".sql", ".sh", ".ps1":
		return domain.ArtifactCode
	case ".csv", ".xlsx", ".xls", ".ndjson", ".parquet":
		return domain.ArtifactData
	}
	return domain.ArtifactFile
}

// mimeByExt 常用扩展名 → MIME；未命中返回 application/octet-stream。
func mimeByExt(ext string) string {
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
	case ".md":
		return "text/markdown"
	case ".txt", ".go", ".ts", ".js", ".py", ".java", ".sql", ".sh", ".ps1":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	}
	return "application/octet-stream"
}
