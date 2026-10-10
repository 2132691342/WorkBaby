// Package knowledge 提供本地文档检索：加载 → 切分 → FTS5 索引 → 检索。
package knowledge

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// 切分参数：块目标 300~800 字，相邻块保留一段重叠以免切断语义。
const (
	chunkTarget = 600
	chunkMax    = 1200
	maxChunks   = 2000
	// 索引读入上限：解析是把整文件读进内存再切块，一个几百 MB 的日志
	// 会先把进程内存顶满，再得到一堆没有检索价值的块。
	maxDocBytes = 64 << 20
)

// Service 是知识库服务；索引写入串行，检索并发安全。
type Service struct {
	repo  *repo.Repo
	mu    sync.Mutex
	jobMu sync.Mutex
	cur   job
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

// Reindex 同步重建全部文档并重建索引，返回成功篇数。
// 走 HTTP 的路径请用 StartReindex（后台任务 + 进度 + 可取消）；
// 这个方法留给装配期与测试——小库一次跑完，不需要任务状态机。
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
	if err != nil {
		// FTS 出错与「没命中」是两回事：留一行日志，否则索引损坏会表现为永久静默降级。
		pkg.Warnf("knowledge: FTS 检索失败，退化为子串匹配: %v", err)
		return s.fallbackSearch(query, limit)
	}
	if len(hits) == 0 {
		// FTS 零命中分两种：真正的「没有」和 trigram 对短查询无能为力。后者如实说清，
		// 免得用户以为资料没进库。
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
	// 超大门槛在解析之前：解析一旦开始，整文件已经在内存里了。
	if st.size > maxDocBytes {
		doc.Status = domain.DocFailed
		doc.Error = fmt.Sprintf("文件超过 %d MB，没有索引：先拆成小文件，或只把要查的那部分加进来", maxDocBytes>>20)
		_ = s.repo.UpsertDoc(doc)
		return pkg.New(6104, doc.Error, "")
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
// 锁只罩住最后的写入：解析是纯 CPU + 文件 IO，占着锁做会让「批量重建索引」
// 期间连一次检索都排不上。
func (s *Service) index(doc *domain.KnowledgeDocDO) error {
	text, err := LoadText(doc.Path)
	if err != nil {
		return err
	}
	chunks := chunk(text)
	truncated := false
	if len(chunks) > maxChunks {
		// 超长文档只索引前一段，但必须留痕：静默截断会让「后半本永远搜不到」
		// 变成用户无法理解的检索失灵，文档还显示「已索引」。
		chunks = chunks[:maxChunks]
		truncated = true
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
	doc.Chunks = len(rows)
	doc.Status = domain.DocIndexed
	doc.Error = ""
	if truncated {
		doc.Error = fmt.Sprintf("文档太长，只索引了前 %d 段（约 %d 字），后面部分搜不到", maxChunks, maxChunks*chunkTarget)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repo.ReplaceChunksAndDoc(doc.ID, rows, doc)
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
