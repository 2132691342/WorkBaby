package capability

import "strings"

// GateResult 门控判定结果；Why 供事件/诊断说明放行或关闭的根由。
type GateResult struct {
	Open bool
	Why  string // opt_in / keyword / off
}

// RelevanceGate 重型 prompt 段门控：显式 opt-in（@引用）> 会话关键词命中 > 默认关闭。
//
// 目的：让「空闲会话上下文精简」成为结构性默认，而不是把全部重型段注入后
// 靠 Priority 裁剪兜底——裁剪发生在预算溢出时，已经付了 token 代价。
type RelevanceGate struct {
	tokens   []string // 显式 opt-in 触发串（小写，含 @）
	keywords []string // 会话关键词（小写）
}

// NewRelevanceGate 构造门控；tokens 为 @ 引用串，keywords 为命中关键词。
func NewRelevanceGate(tokens, keywords []string) *RelevanceGate {
	return &RelevanceGate{tokens: lowerAll(tokens), keywords: lowerAll(keywords)}
}

// Evaluate 判定本轮是否放行该重型段。
func (g *RelevanceGate) Evaluate(input string) GateResult {
	if g == nil {
		return GateResult{Open: true, Why: "ungated"}
	}
	low := strings.ToLower(input)
	for _, t := range g.tokens {
		if strings.Contains(low, t) {
			return GateResult{Open: true, Why: "opt_in"}
		}
	}
	for _, k := range g.keywords {
		if strings.Contains(low, k) {
			return GateResult{Open: true, Why: "keyword"}
		}
	}
	return GateResult{Open: false, Why: "off"}
}

// Clean 去掉输入里的 opt-in 触发串，避免 @引用 污染下游检索词。
func (g *RelevanceGate) Clean(input string) string {
	if g == nil {
		return input
	}
	out := input
	for _, t := range g.tokens {
		out = replaceFold(out, t)
	}
	return strings.TrimSpace(out)
}

// replaceFold 大小写不敏感地删除 sub（空串直接返回）。
func replaceFold(s, sub string) string {
	if sub == "" {
		return s
	}
	for {
		i := strings.Index(strings.ToLower(s), sub)
		if i < 0 {
			return s
		}
		s = s[:i] + " " + s[i+len(sub):]
	}
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// 记忆自动召回：只在用户指向前文/历史时放行（长程记忆正文仍由 MEMORY.md 段常驻承载）。
var memoryGateTokens = []string{"@memory", "@记忆", "@recall"}
var memoryGateKeywords = []string{"之前", "上次", "以前", "回忆", "还记得", "记不记得", "提到过", "remember", "previously", "last time"}

// 知识库自动召回：只在用户明确指向资料/文档时放行（需要更多资料可主动调 knowledge_search）。
var knowledgeGateTokens = []string{"@knowledge", "@知识", "@kb"}
var knowledgeGateKeywords = []string{"知识库", "资料库", "文档库", "参考资料", "knowledge", "documentation"}
