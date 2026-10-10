package repo

import (
	"WorkBaby/backend/domain"

	"gorm.io/gorm"
)

// UpsertDoc 新增或更新知识库文档。
func (r *Repo) UpsertDoc(d *domain.KnowledgeDocDO) error {
	return wrapDB("保存知识文档", r.db.Save(d).Error)
}

// ListDocs 列出全部文档。
func (r *Repo) ListDocs() ([]domain.KnowledgeDocDO, error) {
	var list []domain.KnowledgeDocDO
	if err := r.db.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, wrapDB("列出知识文档", err)
	}
	return list, nil
}

// DeleteDoc 删除文档及其分块（FTS 由触发器同步清理）。
func (r *Repo) DeleteDoc(id string) error {
	return wrapDB("删除知识文档", r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("doc_id = ?", id).Delete(&domain.KnowledgeChunkDO{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.KnowledgeDocDO{}).Error
	}))
}

// ReplaceChunks 用新分块整体替换文档旧分块，保证重建索引幂等。
func (r *Repo) ReplaceChunks(docID string, chunks []domain.KnowledgeChunkDO) error {
	return wrapDB("重建知识分块", r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("doc_id = ?", docID).Delete(&domain.KnowledgeChunkDO{}).Error; err != nil {
			return err
		}
		if len(chunks) == 0 {
			return nil
		}
		return tx.CreateInBatches(chunks, 200).Error
	}))
}

// SearchChunks 走 FTS5 检索；短于 3 字的查询由调用方做子串兜底。
func (r *Repo) SearchChunks(query string, limit int) ([]domain.SearchHitVO, error) {
	if limit <= 0 {
		limit = 5
	}
	type row struct {
		ChunkID string  `gorm:"column:chunk_id"`
		DocID   string  `gorm:"column:doc_id"`
		Content string  `gorm:"column:content"`
		Score   float64 `gorm:"column:score"`
		Title   string  `gorm:"column:title"`
		Path    string  `gorm:"column:path"`
		Seq     int     `gorm:"column:seq"`
	}
	var rows []row
	err := r.db.Raw(`
		SELECT f.chunk_id AS chunk_id, f.doc_id AS doc_id, f.content AS content,
		       bm25(knowledge_chunks_fts) AS score,
		       d.title AS title, d.path AS path, c.seq AS seq
		FROM knowledge_chunks_fts f
		JOIN knowledge_chunks c ON c.id = f.chunk_id
		JOIN knowledge_docs d ON d.id = f.doc_id
		WHERE knowledge_chunks_fts MATCH ?
		ORDER BY score ASC
		LIMIT ?`, query, limit).Scan(&rows).Error
	if err != nil {
		return nil, wrapDB("检索知识分块", err)
	}
	hits := make([]domain.SearchHitVO, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, domain.SearchHitVO{
			DocID: r.DocID, Title: r.Title, Path: r.Path,
			Seq: r.Seq, Content: r.Content, Score: r.Score,
		})
	}
	return hits, nil
}
