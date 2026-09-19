package pkg

import (
	"regexp"
	"strings"
)

// nonWordRe 用非字母数字作分隔切词；保证 token 内不含引号等 FTS5 语法字符（结构性防注入）。
var nonWordRe = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// MinGramLen trigram tokenizer 的最小可查长度；短于它的 token 匹配不到任何 3-gram。
//
// 这是 FTS5 trigram 分词器的硬约束（窗口固定 3 字符，不可配置）：换 unicode61
// 会让整句中文退化成单 token，中文检索会全面失效，故 2 字查询只能由带打分的
// 子串兜底承担——各检索入口都必须实现兜底，禁止静默返回空结果。
const MinGramLen = 3

// MatchTokens 自由文本切词：按非字母数字分隔，丢弃不足 3 字的 token，去重保序。
// 供按列构造 MATCH 表达式的调用方使用。
func MatchTokens(query string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, tok := range nonWordRe.Split(strings.TrimSpace(query), -1) {
		if len([]rune(tok)) < MinGramLen || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// BuildMatchQuery 把自由文本转成 FTS5 MATCH 表达式：token 双引号包裹后 OR 连接。
// 知识库与记忆共用同一套切词 / OR 口径，避免两处漂移。
// 返回空串表示所有 token 都过短，调用方应走 LIKE 兜底而非静默无结果。
func BuildMatchQuery(query string) string {
	toks := MatchTokens(query)
	parts := make([]string, 0, len(toks))
	for _, t := range toks {
		parts = append(parts, `"`+t+`"`)
	}
	return strings.Join(parts, " OR ")
}

// SplitTokens 自由文本切词：按非字母/数字分隔，不过滤长度、不去重。
//
// 与 MatchTokens 的区别：保留 2 字中文短词（trigram MATCH 用不上它们，
// 但带打分的子串兜底与召回场景正需要）；调用方按需去重。
func SplitTokens(query string) []string {
	return nonWordRe.Split(strings.TrimSpace(query), -1)
}

// SubstringFullHitScore 整串命中在子串兜底里的权重：比零散 token 命中强得多，
// 保证「部署手册」这类精确查询排在同 token 命中的噪声之前。
const SubstringFullHitScore = 10

// ScoreSubstring 子串兜底召回的相关性打分：整串命中 + token 命中计数。
//
// LIKE 用不上 bm25，故各检索入口（知识库 / 记忆）共用这一口径排序，
// 避免同一查询在不同来源里排序规则不一致、甚至退化成按创建时间排序。
func ScoreSubstring(text, query string, tokens []string) float64 {
	lower := strings.ToLower(text)
	score := 0.0
	if lq := strings.ToLower(strings.TrimSpace(query)); lq != "" && strings.Contains(lower, lq) {
		score += SubstringFullHitScore
	}
	for _, t := range tokens {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && strings.Contains(lower, t) {
			score++
		}
	}
	return score
}

// EscapeLike 转义 LIKE 通配符（配合 ESCAPE '\' 使用），防用户输入里的 %/_ 撑大匹配面。
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}
