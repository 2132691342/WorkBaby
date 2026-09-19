package rag

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// Indexer 文档 → 分块 → FTS5 落库。
// 状态机：pending → parsing → indexed | failed；失败可由 ReindexDoc 重试。
type Indexer struct {
	db      *gorm.DB
	chunker *Chunker
}

// NewIndexer 构造索引器。
func NewIndexer(db *gorm.DB, c *Chunker) *Indexer {
	if c == nil {
		c = DefaultChunker()
	}
	return &Indexer{db: db, chunker: c}
}

// Index 加载 → 切分 → 事务内重建该文档的全部分块与 FTS 行。
func (ix *Indexer) Index(ctx context.Context, docID string) error {
	var doc domain.KnowledgeDocDO
	if err := ix.db.WithContext(ctx).First(&doc, "id = ?", docID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.ErrKnowledgeDocNotFound
		}
		return pkg.Wrap(7003, "query doc failed", err)
	}
	pkg.L.Info("knowledge index start", "docID", docID, "doc", doc.Name, "sourceType", doc.SourceType)
	indexStart := time.Now()
	ix.setStatus(ctx, docID, domain.KnowledgeStatusParsing, "")

	text, err := ix.loadText(ctx, &doc)
	if err != nil {
		ix.setStatus(ctx, docID, domain.KnowledgeStatusFailed, err.Error())
		pkg.L.Error("knowledge index load failed", "docID", docID, "err", err.Error())
		return err
	}
	chunks := ix.chunker.Chunk(text)
	if len(chunks) == 0 {
		err := pkg.New(7002, "chunker produced no chunks", doc.Name)
		ix.setStatus(ctx, docID, domain.KnowledgeStatusFailed, err.Error())
		pkg.L.Error("knowledge index no chunks", "docID", docID, "err", err.Error())
		return err
	}

	rows := make([]domain.KnowledgeChunkDO, 0, len(chunks))
	for i, ch := range chunks {
		rows = append(rows, domain.KnowledgeChunkDO{
			ID:        pkg.NewID(domain.IDKnowledgeChunk),
			DocID:     docID,
			Sequence:  i,
			Content:   ch.Content,
			Tokens:    pkg.EstimateTextTokens(ch.Content),
			MetaJSON:  marshalMeta(ch.Meta),
			CreatedAt: time.Now().UnixMilli(),
		})
	}

	err = ix.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("doc_id = ?", docID).Delete(&domain.KnowledgeChunkDO{}).Error; err != nil {
			return err
		}
		// FTS5 虚拟表 AutoMigrate 不覆盖，代码同事务维护
		if err := tx.Exec(`DELETE FROM knowledge_chunks_fts WHERE doc_id = ?`, docID).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(rows, 100).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Exec(
				`INSERT INTO knowledge_chunks_fts (id, doc_id, chunk_idx, title, content) VALUES (?, ?, ?, ?, ?)`,
				row.ID, row.DocID, row.Sequence, metaTitle(row.MetaJSON), row.Content,
			).Error; err != nil {
				return err
			}
		}
		return tx.Model(&domain.KnowledgeDocDO{}).Where("id = ?", docID).Updates(map[string]any{
			"status":      string(domain.KnowledgeStatusIndexed),
			"chunk_count": len(rows),
			"error_msg":   "",
			"size_bytes":  int64(len(text)),
		}).Error
	})
	if err != nil {
		e := pkg.Wrap(7003, "index doc failed", err)
		ix.setStatus(ctx, docID, domain.KnowledgeStatusFailed, e.Error())
		pkg.L.Error("knowledge index failed", "docID", docID, "latencyMs", time.Since(indexStart).Milliseconds(), "err", e.Error())
		return e
	}
	pkg.L.Info("knowledge index done", "docID", docID, "chunks", len(chunks),
		"latencyMs", time.Since(indexStart).Milliseconds(), "sizeBytes", len(text))
	return nil
}

// loadText 按 sourceType 分发：text 直接用原文；url 抓取；file 按 MIME 选 loader。
func (ix *Indexer) loadText(ctx context.Context, doc *domain.KnowledgeDocDO) (string, error) {
	switch doc.SourceType {
	case domain.KnowledgeSourceText:
		return doc.Source, nil
	case domain.KnowledgeSourceURL:
		return URLLoader{}.Load(ctx, doc.Source)
	case domain.KnowledgeSourceFile:
		mime := doc.MIME
		if mime == "" {
			mime = MIMEFromPath(doc.Source)
		}
		l := PickLoader(mime)
		if l == nil {
			return "", pkg.New(domain.ErrKnowledgeUnsupported.Code, domain.ErrKnowledgeUnsupported.Message, mime)
		}
		return l.Load(ctx, doc.Source)
	default:
		return "", pkg.New(7001, "unknown source type", string(doc.SourceType))
	}
}

// setStatus 更新文档索引状态；失败只记录，不打断主流程（状态本身就是结果）。
func (ix *Indexer) setStatus(ctx context.Context, docID string, status domain.KnowledgeDocStatus, msg string) {
	_ = ix.db.WithContext(ctx).Model(&domain.KnowledgeDocDO{}).Where("id = ?", docID).
		Updates(map[string]any{"status": string(status), "error_msg": msg}).Error
}

func marshalMeta(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	bs, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return string(bs)
}

func metaTitle(metaJSON string) string {
	if metaJSON == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(metaJSON), &m); err != nil {
		return ""
	}
	return m["title"]
}
