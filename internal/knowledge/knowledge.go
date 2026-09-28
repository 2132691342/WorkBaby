// Package knowledge 提供本地文档检索：加载 → 切分 → FTS5 索引 → 检索。
package knowledge

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// 切分参数：块目标 300~800 字，相邻块保留一段重叠以免切断语义。
const (
	chunkTarget = 600
	chunkMax    = 1200
	maxChunks   = 2000
)

// Service 是知识库服务；索引写入串行，检索并发安全。
type Service struct {
	repo *repo.Repo
	mu   sync.Mutex
}

// New 构造知识库服务。
func New(r *repo.Repo) *Service { return &Service{repo: r} }

// List 列出全部文档。
func (s *Service) List() ([]domain.KnowledgeDocVO, error) {
	docs, err := s.repo.ListDocs()
	if err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeDocVO, 0, len(docs))
	for _, d := range docs {
		out = append(out, docVO(&d))
	}
	return out, nil
}

// Add 添加文档并立即建索引；目录会展开成其下的受支持文件。
func (s *Service) Add(paths []string) (int, error) {
	added := 0
	for _, p := range paths {
		files := expand(p)
		for _, f := range files {
			if err := s.addOne(f); err != nil {
				pkg.Warnf("knowledge: 添加 %s 失败: %v", f, err)
				continue
			}
			added++
		}
	}
	return added, nil
}

// Delete 删除文档及其分块。
func (s *Service) Delete(id string) error { return s.repo.DeleteDoc(id) }

// Reindex 全量重建索引：清掉旧分块后按当前文档重新切分。
func (s *Service) Reindex() (int, error) {
	docs, err := s.repo.ListDocs()
	if err != nil {
		return 0, err
	}
	ok := 0
	for i := range docs {
		if err := s.index(&docs[i]); err != nil {
			pkg.Warnf("knowledge: 重建 %s 失败: %v", docs[i].Path, err)
			continue
		}
		ok++
	}
	return ok, nil
}

// Search 检索：先走 FTS5，短查询（trigram 零命中）退化为子串兜底。
func (s *Service) Search(ctx context.Context, query string, limit int) ([]domain.SearchHitVO, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, domain.ErrQueryEmpty
	}
	if limit <= 0 {
		limit = 5
	}
	hits, err := s.repo.SearchChunks(escapeFTS(query), limit)
	if err != nil || len(hits) == 0 {
		return s.fallbackSearch(query, limit)
	}
	return hits, nil
}

// escapeFTS 把查询包成 FTS5 能吃的短语表达式，避免特殊字符让 MATCH 直接报错。
func escapeFTS(q string) string {
	q = strings.ReplaceAll(q, `"`, `""`)
	return `"` + q + `"`
}

// fallbackSearch 短查询兜底：trigram 对少于 3 字的查询零命中，用 LIKE 子串匹配补上。
func (s *Service) fallbackSearch(query string, limit int) ([]domain.SearchHitVO, error) {
	var rows []struct {
		ChunkID string `gorm:"column:chunk_id"`
		DocID   string `gorm:"column:doc_id"`
		Content string `gorm:"column:content"`
		Title   string `gorm:"column:title"`
		Path    string `gorm:"column:path"`
		Seq     int    `gorm:"column:seq"`
	}
	err := s.repo.DB().Raw(`
		SELECT c.id AS chunk_id, c.doc_id AS doc_id, c.content AS content,
		       c.seq AS seq, d.title AS title, d.path AS path
		FROM knowledge_chunks c
		JOIN knowledge_docs d ON d.id = c.doc_id
		WHERE c.content LIKE ?
		LIMIT ?`, "%"+query+"%", limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SearchHitVO, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.SearchHitVO{
			DocID: r.DocID, Title: r.Title, Path: r.Path,
			Seq: r.Seq, Content: r.Content, Score: 0,
		})
	}
	return out, nil
}

// addOne 落文档并建索引。
func (s *Service) addOne(path string) error {
	if !pkg.FileExists(path) {
		return domain.ErrDocNotFound
	}
	st, err := statFile(path)
	if err != nil {
		return err
	}
	doc := &domain.KnowledgeDocDO{
		ID:     pkg.NewID(domain.PrefixDoc),
		Path:   path,
		Title:  baseName(path),
		Ext:    pkg.Ext(path),
		Size:   st.size,
		Status: domain.DocPending,
	}
	if err := s.repo.UpsertDoc(doc); err != nil {
		return err
	}
	if err := s.index(doc); err != nil {
		doc.Status = domain.DocFailed
		doc.Error = err.Error()
		_ = s.repo.UpsertDoc(doc)
		return err
	}
	return nil
}

// index 解析 → 切分 → 去重 → 写分块（FTS 由触发器同步）。
func (s *Service) index(doc *domain.KnowledgeDocDO) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	text, err := LoadText(doc.Path)
	if err != nil {
		return err
	}
	chunks := chunk(text)
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}
	rows := make([]domain.KnowledgeChunkDO, 0, len(chunks))
	seen := map[string]bool{}
	for i, c := range chunks {
		hash := chunkHash(c)
		if seen[hash] {
			continue
		}
		seen[hash] = true
		rows = append(rows, domain.KnowledgeChunkDO{
			ID:      pkg.NewID(domain.PrefixChunk),
			DocID:   doc.ID,
			Seq:     i,
			Content: c,
			Hash:    hash,
		})
	}
	if err := s.repo.ReplaceChunks(doc.ID, rows); err != nil {
		return err
	}
	doc.Chunks = len(rows)
	doc.Status = domain.DocIndexed
	doc.Error = ""
	return s.repo.UpsertDoc(doc)
}

// chunk 按段落累积到目标长度；相邻块保留最后一段作为重叠。
func chunk(text string) []string {
	paras := strings.Split(text, "\n")
	out := []string{}
	var b strings.Builder
	last := ""
	for _, p := range paras {
		p = strings.TrimRight(p, " \t\r")
		if p == "" {
			continue
		}
		if b.Len()+len(p) > chunkMax && b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
			if last != "" {
				b.WriteString(last)
				b.WriteString("\n")
			}
		}
		b.WriteString(p)
		b.WriteString("\n")
		if b.Len() >= chunkTarget {
			out = append(out, b.String())
			last = p
			b.Reset()
			b.WriteString(last)
			b.WriteString("\n")
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// expand 把目录展开成受支持的文件列表（只取一层，避免把整个磁盘塞进索引）。
func expand(path string) []string {
	if pkg.FileExists(path) {
		return []string{path}
	}
	if !pkg.DirExists(path) {
		return nil
	}
	entries, err := listDir(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if !supported[pkg.Ext(e)] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func docVO(d *domain.KnowledgeDocDO) domain.KnowledgeDocVO {
	return domain.KnowledgeDocVO{
		ID: d.ID, Path: d.Path, Title: d.Title, Ext: d.Ext,
		Size: d.Size, Chunks: d.Chunks, Status: d.Status,
		Error: d.Error, CreatedAt: d.CreatedAt,
	}
}
