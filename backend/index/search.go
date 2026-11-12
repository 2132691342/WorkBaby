package index

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Hit 是一条检索结果。
type Hit struct {
	// HitDoc 命中的文档
	HitDoc Doc
	// HitScore 排序分；越大越靠前
	HitScore float64
	// HitReason 为什么命中（给界面看）
	HitReason string
}

// Frecency 是「频次 × 时近」的排序分。
// 常用的、且最近用过的文件排在前面——
// 只按名字排序会让一堆从没用过的文件压过正在改的那个。
type Frecency struct {
	// hits 文档 ID → 命中次数
	hits map[string]int
	// lastSeen 文档 ID → 最近命中时刻（毫秒）
	lastSeen map[string]int64
}

// NewFrecency 造一份频次账。
func NewFrecency() *Frecency {
	return &Frecency{
		hits:     map[string]int{},
		lastSeen: map[string]int64{},
	}
}

// Record 记一次命中。
func (f *Frecency) Record(docID string, at int64) {
	f.hits[docID]++
	f.lastSeen[docID] = at
}

// Score 给一个文档算分。
// 频次取对数：被搜过 100 次的文件不该是搜过 2 次的 50 倍好，
// 对数压平之后差距才合理。时近用指数衰减：今天用过比上周用过值钱得多。
func (f *Frecency) Score(docID string, now int64) float64 {
	count := f.hits[docID]
	if count == 0 {
		return 0
	}
	frequency := 1.0 + math.Log(float64(count))
	age := max(now-f.lastSeen[docID], 0)
	// 半衰期 7 天：七天前用过的东西，权重只剩一半。
	decay := 0.5
	days := float64(age) / float64(24*60*60*1000)
	recency := 1.0 / (1.0 + days/7.0)
	return frequency * (1.0 + recency) * decay
}

// Hits 取命中次数。
func (f *Frecency) Hits(docID string) int { return f.hits[docID] }

// Search 检索索引。
// 匹配分三档：符号名精确命中最高、路径包含次之、正文包含最低。
// 排完再叠上 frecency：文本相关度决定「像不像」，frecency 决定「该不该优先」。
func Search(docs []Doc, query string, frecency *Frecency, limit int) []Hit {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	now := time.Now().UnixMilli()
	hits := make([]Hit, 0, limit)
	for _, doc := range docs {
		score, reason := matchScore(doc, needle)
		if score <= 0 {
			continue
		}
		if frecency != nil {
			score += frecency.Score(doc.DocID, now)
		}
		hits = append(hits, Hit{HitDoc: doc, HitScore: score, HitReason: reason})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].HitScore != hits[j].HitScore {
			return hits[i].HitScore > hits[j].HitScore
		}
		// 同分时按路径排：结果顺序要稳定，否则每次搜都跳来跳去。
		return hits[i].HitDoc.DocPath < hits[j].HitDoc.DocPath
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// matchScore 算一条文档的文本相关度。
func matchScore(doc Doc, needle string) (float64, string) {
	symbol := strings.ToLower(doc.DocSymbol)
	path := strings.ToLower(doc.DocPath)
	base := strings.ToLower(path)
	if index := strings.LastIndex(base, "/"); index >= 0 {
		base = base[index+1:]
	}
	switch {
	case symbol == needle:
		return 100, "符号名完全匹配"
	case strings.Contains(symbol, needle):
		return 60, "符号名包含"
	case base == needle:
		return 50, "文件名完全匹配"
	case strings.HasPrefix(base, needle):
		return 40, "文件名前缀匹配"
	case strings.Contains(path, needle):
		return 30, "路径包含"
	case doc.DocBody != "" && strings.Contains(strings.ToLower(doc.DocBody), needle):
		return 10, "正文包含"
	}
	return 0, ""
}
