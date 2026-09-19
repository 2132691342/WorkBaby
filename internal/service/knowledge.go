package service

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/rag"
	"WorkBaby/internal/repo"
)

// KnowledgeService 知识库编排：上传即返回 + 后台索引 + 统一检索。
type KnowledgeService struct {
	docRepo *repo.KnowledgeDocRepo
	ix      *rag.Indexer
	rt      rag.Retriever
	dir     string // {home}/knowledge 受管导入目录（文件复制后 doc.Source 指向此处，删除文档时一并清理）
}

// NewKnowledgeService 注入仓储、索引器与检索器；managedDir 为受管导入目录。
func NewKnowledgeService(r *repo.KnowledgeDocRepo, ix *rag.Indexer, rt rag.Retriever, managedDir string) *KnowledgeService {
	return &KnowledgeService{docRepo: r, ix: ix, rt: rt, dir: managedDir}
}

// 受管导入限制。
const (
	MaxManagedDocBytes = 60 << 20 // 单文件 ≤ 60MB（对应当前文档解析面）
)

// managedDocExts 允许导入受管库的扩展名白名单（与文档 loader 支持面一致）。
//
// .docx 已在 loader 中实现解包（archive/zip + word/document.xml），白名单一并放开；
// 旧版私文档二进制 .doc 仍拒收（结构老旧、依赖重）。
var managedDocExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".pdf": true,
	".json": true, ".yaml": true, ".yml": true, ".xml": true,
	".csv": true, ".html": true, ".htm": true,
	".docx": true,
}

// AddDoc 新建文档并后台索引；索引状态通过 GetDoc 轮询。
// 注意：file 型一律先受管复制到 {home}/knowledge/{id}.{ext}（与 ImportLocalFile 同源），
// DB 不裸引用用户任意路径——否则用户删除源文件后，DB/索引与文件产生不一致。
func (s *KnowledgeService) AddDoc(ctx context.Context, req *domain.KnowledgeDocREQ) (*domain.KnowledgeDocRESP, error) {
	sourceType, err := normalizeSourceType(req.SourceType, req.Source)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Source) == "" {
		return nil, pkg.New(7001, "knowledge source is required", "")
	}
	if err := validateSource(sourceType, req.Source); err != nil {
		return nil, err
	}

	row := &domain.KnowledgeDocDO{
		ID:         pkg.NewID(domain.IDKnowledgeDoc),
		FolderID:   req.FolderID,
		Name:       strings.TrimSpace(req.Name),
		SourceType: sourceType,
		MIME:       docMIME(sourceType, req.Source),
		Status:     domain.KnowledgeStatusPending,
	}
	if sourceType == domain.KnowledgeSourceFile {
		// file 型受管复制：{home}/knowledge 是知识文件的唯一存放点
		dst, mime, size, cerr := s.importCopy(req.Source, row.ID)
		if cerr != nil {
			return nil, cerr
		}
		row.Source = dst
		row.MIME = mime
		row.SizeBytes = size
	} else {
		row.Source = req.Source
	}
	if row.Name == "" {
		row.Name = inferName(sourceType, req.Source)
	}
	if err := s.docRepo.Create(ctx, row); err != nil {
		if sourceType == domain.KnowledgeSourceFile && row.Source != "" {
			_ = os.Remove(row.Source)
		}
		return nil, err
	}

	// 后台索引：上传立即返回；失败状态落在文档上，可 Reindex 重试
	s.startIndex(row.ID)

	return toKnowledgeRESP(row), nil
}

// importCopy 把外部文件复制进受管目录（扩展名白名单 + 大小限制 + 逐块复制）。
// 返回受管路径、MIME、字节数。AddDoc 与 ImportLocalFile 共用同一套校验，保证规则一致。
func (s *KnowledgeService) importCopy(srcPath, id string) (dst, mime string, size int64, rerr error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", "", 0, pkg.Wrap(7001, "source file not found", err)
	}
	if info.IsDir() {
		return "", "", 0, pkg.New(7001, "directory is not supported, pick a file", "")
	}
	if info.Size() > MaxManagedDocBytes {
		return "", "", 0, pkg.New(7002, "file exceeds 60MB limit", "")
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if !managedDocExts[ext] {
		return "", "", 0, domain.ErrKnowledgeUnsupported
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", "", 0, pkg.Wrap(7006, "mkdir knowledge dir failed", err)
	}
	dst = filepath.Join(s.dir, id+ext)
	if err := copyFile(srcPath, dst); err != nil {
		_ = os.Remove(dst)
		return "", "", 0, pkg.Wrap(7006, "copy file into knowledge dir failed", err)
	}
	return dst, rag.MIMEFromPath(srcPath), info.Size(), nil
}

// startIndex 后台索引（10 分钟超时）；失败状态落在文档 status/error_msg 上。
func (s *KnowledgeService) startIndex(id string) {
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = s.ix.Index(bctx, id)
	}()
}

// ImportLocalFile 导入本地文件到受管知识库：
// 1) 白名单 + 大小校验；2) 复制进 {home}/knowledge/{id}.{ext}（不再裸引用用户任意路径）；
// 3) 注册文档（source = 受管路径）并后台索引。返回即注册完成，索引进度轮询 GetDoc 状态。
func (s *KnowledgeService) ImportLocalFile(ctx context.Context, name, srcPath string, folderID *string) (*domain.KnowledgeDocRESP, error) {
	if strings.TrimSpace(srcPath) == "" {
		return nil, pkg.New(7001, "source path is required", "")
	}
	id := pkg.NewID(domain.IDKnowledgeDoc)
	dst, mime, size, err := s.importCopy(srcPath, id)
	if err != nil {
		return nil, err
	}

	row := &domain.KnowledgeDocDO{
		ID:         id,
		FolderID:   folderID,
		Name:       strings.TrimSpace(name),
		Source:     dst,
		SourceType: domain.KnowledgeSourceFile,
		MIME:       mime,
		SizeBytes:  size,
		Status:     domain.KnowledgeStatusPending,
	}
	if row.Name == "" {
		row.Name = filepath.Base(srcPath)
	}
	if err := s.docRepo.Create(ctx, row); err != nil {
		_ = os.Remove(dst)
		return nil, err
	}
	s.startIndex(id)
	return toKnowledgeRESP(row), nil
}

// copyFile 逐块复制（避免大文件整读内存）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ListDocs 文档列表。
func (s *KnowledgeService) ListDocs(ctx context.Context) ([]domain.KnowledgeDocRESP, error) {
	rows, err := s.docRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeDocRESP, 0, len(rows))
	for i := range rows {
		out = append(out, *toKnowledgeRESP(&rows[i]))
	}
	return out, nil
}

// GetDoc 单个文档（前端轮询索引状态用）。
func (s *KnowledgeService) GetDoc(ctx context.Context, id string) (*domain.KnowledgeDocRESP, error) {
	row, err := s.docRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toKnowledgeRESP(row), nil
}

// DeleteDoc 软删文档并清空分块与 FTS 行；受管来源文件（{home}/knowledge/）级联删除。
func (s *KnowledgeService) DeleteDoc(ctx context.Context, id string) error {
	if row, err := s.docRepo.GetByID(ctx, id); err == nil && row != nil {
		if s.dir != "" && strings.HasPrefix(row.Source, s.dir+string(os.PathSeparator)) {
			_ = os.Remove(row.Source)
		}
	}
	return s.docRepo.Delete(ctx, id)
}

// ReindexDoc 重新索引（源文件变更 / 上次失败重试）。
func (s *KnowledgeService) ReindexDoc(ctx context.Context, id string) error {
	if _, err := s.docRepo.GetByID(ctx, id); err != nil {
		return err
	}
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = s.ix.Index(bctx, id)
	}()
	return nil
}

// UpdateDoc 更新文档名称（及可选来源）；name 为空保留原值；source 非空且变化时触发重新索引。
func (s *KnowledgeService) UpdateDoc(ctx context.Context, id string, req *domain.KnowledgeDocREQ) (*domain.KnowledgeDocRESP, error) {
	row, err := s.docRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		row.Name = strings.TrimSpace(req.Name)
	}
	// 先记下旧来源：row.Source 下面会被赋成 req.Source，若用更新后的值判断
	// 「是否换源」会恒为 false，导致改了 text/URL 的文档永远不再重索引（状态卡 pending）。
	oldSource := row.Source
	sourceChanged := req.Source != "" && req.Source != oldSource
	if sourceChanged {
		if st, err := normalizeSourceType(req.SourceType, req.Source); err == nil {
			row.SourceType = st
		}
		if row.SourceType == domain.KnowledgeSourceFile {
			// file 型换源 → 也走受管复制（旧受管文件级联清理），DB 不裸引用用户路径
			if underManagedDir(s.dir, req.Source) {
				// 已是受管目录内路径（如编辑回显），直接沿用
				row.Source = req.Source
			} else {
				if err := validateSource(domain.KnowledgeSourceFile, req.Source); err != nil {
					return nil, err
				}
				dst, mime, size, cerr := s.importCopy(req.Source, row.ID)
				if cerr != nil {
					return nil, cerr
				}
				if underManagedDir(s.dir, row.Source) {
					_ = os.Remove(row.Source)
				}
				row.Source = dst
				row.MIME = mime
				row.SizeBytes = size
			}
		} else {
			row.Source = req.Source
			row.MIME = docMIME(row.SourceType, req.Source)
		}
		row.Status = domain.KnowledgeStatusPending
	}
	if err := s.docRepo.Update(ctx, row); err != nil {
		return nil, err
	}
	// 来源变化 → 后台重索引（与上面用同一个 oldSource 比较，不能用已更新的 row.Source）
	if sourceChanged {
		go func(docID string) {
			bctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			_ = s.ix.Index(bctx, docID)
		}(row.ID)
	}
	return toKnowledgeRESP(row), nil
}

// underManagedDir 判定路径是否真正落在受管知识库目录内。
//
// 不能用 HasPrefix：`{dir}\..\..\任意文件.txt` 能通过前缀检查却被当作受管源读取，
// 是标准的 .. 穿越。先 Abs 归一，再用 Rel 判定不逃逸。
func underManagedDir(dir, p string) bool {
	if dir == "" || p == "" {
		return false
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// ListGroups 返回全部分组（按 source_type 去重；前端 group 下拉用）。
func (s *KnowledgeService) ListGroups(ctx context.Context) ([]string, error) {
	rows, err := s.docRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[domain.KnowledgeSourceType]bool{}
	var out []string
	for i := range rows {
		st := rows[i].SourceType
		if st == "" {
			st = domain.KnowledgeSourceText
		}
		if seen[st] {
			continue
		}
		seen[st] = true
		out = append(out, string(st))
	}
	return out, nil
}

// ListByGroup 按来源类型过滤文档。
func (s *KnowledgeService) ListByGroup(ctx context.Context, group string) ([]domain.KnowledgeDocRESP, error) {
	rows, err := s.docRepo.ListBySourceType(ctx, domain.KnowledgeSourceType(group))
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeDocRESP, 0, len(rows))
	for i := range rows {
		out = append(out, *toKnowledgeRESP(&rows[i]))
	}
	return out, nil
}

// Search 知识库检索。
func (s *KnowledgeService) Search(ctx context.Context, query string, topK int) ([]domain.KnowledgeHitRESP, error) {
	hits, err := s.rt.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeHitRESP, 0, len(hits))
	for _, h := range hits {
		out = append(out, domain.KnowledgeHitRESP{
			DocID:   h.DocID,
			DocName: h.DocName,
			ChunkID: h.ChunkID,
			Content: h.Content,
			Score:   h.Score,
			Source:  h.Source,
			Meta:    h.Meta,
		})
	}
	return out, nil
}

// normalizeSourceType 空值时按 Source 形态推断。
func normalizeSourceType(raw, source string) (domain.KnowledgeSourceType, error) {
	switch domain.KnowledgeSourceType(raw) {
	case "":
		if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
			return domain.KnowledgeSourceURL, nil
		}
		if strings.Contains(source, "\n") || len(source) > 512 {
			return domain.KnowledgeSourceText, nil
		}
		return domain.KnowledgeSourceFile, nil
	case domain.KnowledgeSourceFile:
		return domain.KnowledgeSourceFile, nil
	case domain.KnowledgeSourceURL:
		return domain.KnowledgeSourceURL, nil
	case domain.KnowledgeSourceText:
		return domain.KnowledgeSourceText, nil
	default:
		return "", pkg.New(7001, "unknown source type", raw)
	}
}

// validateSource 源形态校验（file 需真实存在，url 需 http(s)）。
func validateSource(st domain.KnowledgeSourceType, source string) error {
	switch st {
	case domain.KnowledgeSourceFile:
		if _, err := os.Stat(source); err != nil {
			return pkg.Wrap(7001, "file not found", err)
		}
	case domain.KnowledgeSourceURL:
		if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
			return pkg.New(7001, "url must start with http(s)://", source)
		}
	}
	return nil
}

// docMIME 落库用 MIME：file 按扩展名推断，url 交给 URLLoader 按响应头判断，text 是纯文本。
func docMIME(st domain.KnowledgeSourceType, source string) string {
	switch st {
	case domain.KnowledgeSourceFile:
		return rag.MIMEFromPath(source)
	case domain.KnowledgeSourceURL:
		return "text/html"
	default:
		return "text/plain"
	}
}

// inferName 名称缺省值：文件名 / URL 路径 / 原文前缀。
func inferName(st domain.KnowledgeSourceType, source string) string {
	switch st {
	case domain.KnowledgeSourceFile:
		return filepath.Base(source)
	case domain.KnowledgeSourceURL:
		u, err := url.Parse(source)
		if err != nil {
			return "未命名文档"
		}
		name := strings.Trim(u.Path, "/")
		if name == "" {
			name = u.Host
		}
		return name
	default:
		r := []rune(strings.TrimSpace(source))
		if len(r) == 0 {
			return "未命名文档"
		}
		if len(r) > 30 {
			return string(r[:30]) + "…"
		}
		return string(r)
	}
}

func toKnowledgeRESP(row *domain.KnowledgeDocDO) *domain.KnowledgeDocRESP {
	return &domain.KnowledgeDocRESP{
		ID:         row.ID,
		FolderID:   row.FolderID,
		Name:       row.Name,
		Source:     row.Source,
		SourceType: row.SourceType,
		MIME:       row.MIME,
		SizeBytes:  row.SizeBytes,
		ChunkCount: row.ChunkCount,
		Status:     row.Status,
		ErrorMsg:   row.ErrorMsg,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}
