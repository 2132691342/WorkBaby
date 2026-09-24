package memory

import (
	"strings"
	"sync"

	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// Hit 检索命中。
type Hit struct {
	Section string  `json:"section"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

// minTrigramQuery FTS5 trigram 分词按 3 字符窗口切分，短于 3 字符的查询必然零命中。
const minTrigramQuery = 3

// index 记忆条目的派生索引。全量重建而非增量同步：条目量级是几十条，重建立即完成，
// 增量同步要处理「文件被用户手工编辑」的各种边界，收益与成本不成比例。
type index struct {
	db *gorm.DB
	mu sync.Mutex
}

func newIndex(db *gorm.DB) *index {
	i := &index{db: db}
	i.ensureTable()
	return i
}

// ensureTable 幂等建索引表；无 db 时空操作。
func (i *index) ensureTable() {
	if i == nil || i.db == nil {
		return
	}
	_ = i.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(section, content, tokenize='trigram')`).Error
}

// rebuild 用当前条目全量重建索引；失败只告警（记忆本身已在文件里，索引丢了可重建）。
func (i *index) rebuild(entries []Entry) {
	if i == nil || i.db == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	// 走事务：逐条 Exec 各自提交时，每次 INSERT 都要落一次盘，
	// 条目上百后重建会从毫秒级劣化到秒级。
	err := i.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM memory_fts`).Error; err != nil {
			return err
		}
		for _, e := range entries {
			if err := tx.Exec(`INSERT INTO memory_fts(section, content) VALUES(?, ?)`, e.Section, e.Text).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		pkg.L.Warn("rebuild memory index failed", "entries", len(entries), "err", err.Error())
	}
}

// search 检索：优先 FTS5，短查询或索引不可用时退化为子串扫描。
func (i *index) search(query string, k int, all []Entry) []Hit {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if k <= 0 {
		k = 5
	}
	if i == nil || i.db == nil || len([]rune(query)) < minTrigramQuery {
		return substringHits(query, k, all)
	}

	i.mu.Lock()
	type row struct {
		Section string
		Content string
	}
	var rows []row
	err := i.db.Raw(
		`SELECT section, content FROM memory_fts WHERE memory_fts MATCH ? ORDER BY bm25(memory_fts) LIMIT ?`,
		ftsQuery(query), k,
	).Scan(&rows).Error
	i.mu.Unlock()

	if err != nil || len(rows) == 0 {
		// FTS5 对中文短词与特殊字符容易零命中：退化为子串扫描，宁可慢也不漏。
		return substringHits(query, k, all)
	}
	out := make([]Hit, 0, len(rows))
	for idx, r := range rows {
		out = append(out, Hit{Section: r.Section, Text: r.Content, Score: float64(len(rows) - idx)})
	}
	return out
}

// ftsQuery 把用户输入转成 FTS5 查询串：每个词加引号，避免 - / : 之类被当语法。
func ftsQuery(query string) string {
	fields := strings.Fields(query)
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		if f != "" {
			parts = append(parts, `"`+f+`"`)
		}
	}
	if len(parts) == 0 {
		return `""`
	}
	return strings.Join(parts, " OR ")
}

// substringHits 子串扫描兜底：按命中次数与位置排序。
func substringHits(query string, k int, all []Entry) []Hit {
	lower := strings.ToLower(query)
	out := make([]Hit, 0, k)
	type scored struct {
		hit   Hit
		count int
	}
	var scoredList []scored
	for _, e := range all {
		text := strings.ToLower(e.Text)
		n := strings.Count(text, lower)
		if n == 0 {
			continue
		}
		score := float64(n)*10 - float64(len(e.Text))/100
		scoredList = append(scoredList, scored{hit: Hit{Section: e.Section, Text: e.Text, Score: score}, count: len(scoredList)})
	}
	// 命中次数多的优先；同分保持文件顺序（稳定，便于人工核对）。
	for i := 1; i < len(scoredList); i++ {
		for j := i; j > 0 && scoredList[j].hit.Score > scoredList[j-1].hit.Score; j-- {
			scoredList[j], scoredList[j-1] = scoredList[j-1], scoredList[j]
		}
	}
	for _, s := range scoredList {
		if len(out) >= k {
			break
		}
		out = append(out, s.hit)
	}
	return out
}
