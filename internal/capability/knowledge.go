package capability

import (
	"context"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/rag"
	"WorkBaby/internal/tool"
	"WorkBaby/internal/tool/knowledge"
)

// knowledgeCap 知识库接入：自动召回注入 + 模型主动检索两条通道，共用同一个检索器。
// 前者解决「不知道有哪些资料、忘了搜」，后者解决「需要更深/更多资料」。
type knowledgeCap struct {
	rt       rag.Retriever
	topK     int // 自动召回条数
	maxChars int // 注入正文上限（控制上下文占用）
	minQuery int // 触发自动召回的最短输入（rune）
	gate     *RelevanceGate
	tools    []tool.Tool
}

// NewKnowledge 构造知识库能力；rt 为 nil 时整体降级为空。
func NewKnowledge(rt rag.Retriever) Capability {
	c := &knowledgeCap{
		rt: rt, topK: 3, maxChars: 3000, minQuery: 2,
		gate: NewRelevanceGate(knowledgeGateTokens, knowledgeGateKeywords),
	}
	if rt != nil {
		c.tools = []tool.Tool{knowledge.New(rt)}
	}
	return c
}

func (c *knowledgeCap) ID() string { return "knowledge" }

func (c *knowledgeCap) Tools() []tool.Tool { return c.tools }

// Preload 按本轮输入自动检索知识库并注入命中片段。
// 纯本地 FTS5 检索；RelevanceGate 未放行（未指资料）、无命中或输入过短时不注入。
func (c *knowledgeCap) Preload(ctx context.Context, p *PreloadCtx) ([]agent.Section, error) {
	if c.rt == nil {
		return nil, nil
	}
	if !c.gate.Evaluate(p.UserInput).Open {
		return nil, nil
	}
	q := strings.TrimSpace(c.gate.Clean(p.UserInput))
	if len([]rune(q)) < c.minQuery {
		return nil, nil
	}
	hits, err := c.rt.Search(ctx, q, c.topK)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}
	return []agent.Section{{
		Key:   "knowledge",
		Title: "知识库相关片段（自动召回；需要更多资料用 knowledge_search）",
		Body: "以下片段来自本地知识库，按 [n] 编号。回答引用了片段内容时必须以 [n] 标注来源文档，" +
			"片段中没有的内容不要编造：\n\n" +
			pkg.TruncateRunes(rag.FormatHits(hits), c.maxChars),
		Order: agent.OrderKnowledge,
	}}, nil
}

func (c *knowledgeCap) Capture(_ context.Context, _ *CaptureCtx) error { return nil }
