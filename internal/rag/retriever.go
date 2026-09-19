package rag

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// Hit 检索命中项。ChunkIdx 是块在文档内的序号，与 DocID 一起构成可溯源坐标。
type Hit struct {
	DocID    string            `json:"docID"`
	DocName  string            `json:"docName"`
	ChunkID  string            `json:"chunkID"`
	ChunkIdx int               `json:"chunkIdx"`
	Content  string            `json:"content"`
	Score    float64           `json:"score"`
	Source   string            `json:"source"`
	Meta     map[string]string `json:"meta"`
}

// Retriever 统一检索接口；上层不感知底层实现。
type Retriever interface {
	Search(ctx context.Context, query string, topK int) ([]Hit, error)
}

// FTS5Retriever 检索实现：unicode61 tokenizer + BM25 排序；token 全部过短时 LIKE 兜底。
type FTS5Retriever struct {
	db *gorm.DB
}

// NewFTS5Retriever 构造检索器。
func NewFTS5Retriever(db *gorm.DB) *FTS5Retriever { return &FTS5Retriever{db: db} }

// BuildMatchQuery 自由文本 → FTS5 MATCH 表达式。
// 实现收口在 pkg：知识库与记忆共用同一套切词 / OR 口径，避免两处漂移。
func BuildMatchQuery(query string) string { return pkg.BuildMatchQuery(query) }

// Search BM25 检索；Score 取负翻转为「越大越相关」（bm25 值越小越相关）。
func (r *FTS5Retriever) Search(ctx context.Context, query string, topK int) ([]Hit, error) {
	if topK <= 0 {
		topK = 5
	}
	if topK > 50 {
		topK = 50
	}
	match := BuildMatchQuery(query)
	if match == "" {
		// 所有 token 都短于 trigram 最小窗口：走带打分子串兜底，而不是静默无结果
		return r.searchLike(ctx, query, topK)
	}

	var rows []struct {
		ChunkID  string  `gorm:"column:chunk_id"`
		DocID    string  `gorm:"column:doc_id"`
		DocName  string  `gorm:"column:doc_name"`
		ChunkIdx int     `gorm:"column:chunk_idx"`
		Source   string  `gorm:"column:source"`
		Content  string  `gorm:"column:content"`
		MetaJSON string  `gorm:"column:meta_json"`
		Score    float64 `gorm:"column:score"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT kc.id AS chunk_id, kc.doc_id AS doc_id, kd.name AS doc_name,
		       kc.sequence AS chunk_idx,
		       kd.source AS source, kc.content AS content, kc.meta_json AS meta_json,
		       bm25(knowledge_chunks_fts) AS score
		FROM knowledge_chunks_fts
		JOIN knowledge_chunks kc ON kc.id = knowledge_chunks_fts.id
		JOIN knowledge_docs kd ON kd.id = kc.doc_id
		WHERE knowledge_chunks_fts MATCH ? AND kd.deleted_at IS NULL
		ORDER BY score
		LIMIT ?`, match, topK).Scan(&rows).Error
	if err != nil {
		return nil, pkg.Wrap(7004, "fts5 search failed", err)
	}
	// MATCH 零命中（query 混有 2 字短词、长词在库中不存在等）→ LIKE 兜底续查，
	// 而不是静默空结果——静默空会让上层判定「知识库没有相关内容」，模型转而编造。
	if len(rows) == 0 {
		return r.searchLike(ctx, query, topK)
	}

	out := make([]Hit, 0, len(rows))
	for i := range rows {
		out = append(out, Hit{
			DocID:    rows[i].DocID,
			DocName:  rows[i].DocName,
			ChunkID:  rows[i].ChunkID,
			ChunkIdx: rows[i].ChunkIdx,
			Content:  rows[i].Content,
			Score:    -rows[i].Score,
			Source:   rows[i].Source,
			Meta:     parseMeta(rows[i].MetaJSON),
		})
	}
	return out, nil
}

// searchLike FTS5 兜底：MATCH 对过短 token 与生僻专名零命中时退化为 LIKE 子串扫描；
// 命中条件为整串 LIKE 或任一 token LIKE。Go 侧按相关度打分（整串命中 > token 命中计数），不按时间排序。
func (r *FTS5Retriever) searchLike(ctx context.Context, query string, topK int) ([]Hit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	toks := pkg.SplitTokens(q)
	seen := make(map[string]bool, len(toks))
	clauses := make([]string, 0, len(toks)+1)
	args := make([]any, 0, len(toks)+2)
	clauses = append(clauses, "kc.content LIKE ? ESCAPE '\\'")
	args = append(args, "%"+pkg.EscapeLike(q)+"%")
	for _, t := range toks {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		clauses = append(clauses, "kc.content LIKE ? ESCAPE '\\'")
		args = append(args, "%"+pkg.EscapeLike(t)+"%")
	}
	// 候选放大若干倍取回，Go 侧按相关性重排后再截 topK
	const likeCandidateScale = 4
	const likeCandidateMax = 200
	limit := topK * likeCandidateScale
	if limit > likeCandidateMax {
		limit = likeCandidateMax
	}
	args = append(args, limit)

	sql := `
		SELECT kc.id AS chunk_id, kc.doc_id AS doc_id, kd.name AS doc_name,
		       kc.sequence AS chunk_idx,
		       kd.source AS source, kc.content AS content, kc.meta_json AS meta_json
		FROM knowledge_chunks kc
		JOIN knowledge_docs kd ON kd.id = kc.doc_id
		WHERE kd.deleted_at IS NULL AND (` + strings.Join(clauses, " OR ") + `)`
	var rows []struct {
		ChunkID  string `gorm:"column:chunk_id"`
		DocID    string `gorm:"column:doc_id"`
		DocName  string `gorm:"column:doc_name"`
		ChunkIdx int    `gorm:"column:chunk_idx"`
		Source   string `gorm:"column:source"`
		Content  string `gorm:"column:content"`
		MetaJSON string `gorm:"column:meta_json"`
	}
	err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error
	if err != nil {
		return nil, pkg.Wrap(7004, "like search failed", err)
	}

	// 相关性打分：与记忆侧共用 pkg.ScoreSubstring 口径；同分保持 SQL 返回序（稳定排序）
	tokens := make([]string, 0, len(seen))
	for t := range seen {
		tokens = append(tokens, t)
	}
	out := make([]Hit, 0, len(rows))
	for i := range rows {
		score := pkg.ScoreSubstring(rows[i].Content, q, tokens)
		out = append(out, Hit{
			DocID:    rows[i].DocID,
			DocName:  rows[i].DocName,
			ChunkID:  rows[i].ChunkID,
			ChunkIdx: rows[i].ChunkIdx,
			Content:  rows[i].Content,
			Score:    score,
			Source:   rows[i].Source,
			Meta:     parseMeta(rows[i].MetaJSON),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func parseMeta(metaJSON string) map[string]string {
	if metaJSON == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(metaJSON), &m); err != nil {
		return nil
	}
	return m
}

// FormatHits 把命中项格式化为便于模型引用出处的文本。
func FormatHits(hits []Hit) string {
	if len(hits) == 0 {
		return "（知识库中没有找到相关内容）"
	}
	var sb strings.Builder
	for i, h := range hits {
		sb.WriteString("[")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("] ")
		sb.WriteString(h.DocName)
		if h.Meta != nil && h.Meta["title"] != "" {
			sb.WriteString(" · ")
			sb.WriteString(h.Meta["title"])
		}
		sb.WriteString(" · 段")
		sb.WriteString(strconv.Itoa(h.ChunkIdx))
		sb.WriteString("\n")
		sb.WriteString(h.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}
