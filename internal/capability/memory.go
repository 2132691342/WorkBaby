package capability

import (
	"context"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
	"WorkBaby/internal/tool/memorywrite"
)

// 注入上限与默认召回条数。记忆段落是全 run 常驻的，必须有硬上限——
// 没有上限的注入段会随使用时间单调增长，最终挤掉真正的对话历史。
const (
	defaultRecallLimit   = 6
	memoryInjectMaxRunes = 4000
)

// memoryCap 长期记忆：把 MEMORY.md 的相关条目注入 system，并暴露写入工具。
type memoryCap struct {
	mem     *memory.Service
	enabled func(context.Context) bool
	tools   []tool.Tool
}

// NewMemory 构造记忆能力；enabled 为全局开关回调（nil = 恒开）。
func NewMemory(mem *memory.Service, enabled func(context.Context) bool) Capability {
	if enabled == nil {
		enabled = func(context.Context) bool { return true }
	}
	return &memoryCap{mem: mem, enabled: enabled, tools: []tool.Tool{memorywrite.New(mem)}}
}

func (c *memoryCap) ID() string { return "memory" }

// Preload 注入记忆：优先按本轮输入检索命中，无命中时回退到文件全文（条目少时它本身就是索引）。
func (c *memoryCap) Preload(ctx context.Context, p *PreloadCtx) ([]agent.Section, error) {
	if c.mem == nil || !p.Def.Memory.Enabled || !c.enabled(ctx) {
		return nil, nil
	}
	limit := p.Def.Memory.RecallLimit
	if limit <= 0 {
		limit = defaultRecallLimit
	}

	body := formatHits(c.mem.Search(p.UserInput, limit))
	if body == "" {
		// 无命中不等于「没记忆」：可能是问法里没有可检索的关键词，
		// 因此回退到全文——宁可能塞一点，也比模型不知道用户是谁好。
		raw := strings.TrimSpace(c.mem.Read())
		if raw == "" {
			return nil, nil
		}
		body = pkg.TruncateRunes(raw, memoryInjectMaxRunes)
	}
	return []agent.Section{{
		Key:      "memory",
		Title:    "长期记忆（用 memory_write 追加）",
		Body:     body,
		Order:    agent.OrderMemory,
		Priority: agent.PriorityMedium,
	}}, nil
}

func (c *memoryCap) Tools() []tool.Tool { return c.tools }

// Capture 空实现：记忆由模型经 memory_write 主动写入，不做 run 后自动沉淀——
// 自动抽取的候选项需要人审才有价值，而那正是被下线的收件箱。
func (c *memoryCap) Capture(context.Context, *CaptureCtx) error { return nil }

// formatHits 渲染检索命中为注入正文。
func formatHits(hits []memory.Hit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hits {
		b.WriteString("- [" + h.Section + "] " + h.Text + "\n")
	}
	return strings.TrimSpace(pkg.TruncateRunes(b.String(), memoryInjectMaxRunes))
}
