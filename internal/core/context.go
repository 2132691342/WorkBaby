package core

import (
	"sort"
	"strings"
	"unicode/utf8"

	"WorkBaby/internal/llm"
)

// Section 上下文片段。Order 决定拼接顺序（小者在前），Priority 决定裁剪顺序（大者先丢）。
//
// 这是「能力三通道 + 五档优先级 + RelevanceGate」的替代品：那段机制的全部产出
// 只是一段 system 文本，因此收敛为一个纯函数。
type Section struct {
	Key      string // 语义标识（裁剪回传用）；空则用 Title
	Title    string
	Body     string
	Order    int // 拼接顺序：小者先
	Priority int // 裁剪优先级：大者先丢；0 = 取 Order
}

// 裁剪优先级。数值 <= PriorityEssential 的段视为常驻，预算再紧也不丢。
const (
	PriorityEssential = 10
	PriorityHigh      = 30
	PriorityMedium    = 50
	PriorityLow       = 70
	PriorityLowest    = 80
)

// 拼接顺序位：人格与环境最先，重型召回段最后。
const (
	OrderPersona   = 10
	OrderEnv       = 20
	OrderWorkspace = 30
	OrderMemory    = 40
	OrderKnowledge = 50
	OrderSkill     = 60
	OrderTodo      = 70
)

// priority 生效裁剪优先级：未显式声明时取 Order（拼接序即重要性序）。
func (s Section) priority() int {
	if s.Priority > 0 {
		return s.Priority
	}
	return s.Order
}

// label 裁剪回传用的段标识。
func (s Section) label() string {
	if s.Key != "" {
		return s.Key
	}
	return s.Title
}

// BuildSystem 装配 system 正文：按 Order 升序拼接；超预算时按 Priority 降序丢段，
// 常驻段（Priority <= PriorityEssential）永不丢。
//
// 返回正文与被丢弃段的标识——被丢段必须回传前端，否则「上下文里少了什么」不可解释。
func BuildSystem(sections []Section, maxRunes int) (string, []string) {
	kept := make([]Section, len(sections))
	copy(kept, sections)
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Order < kept[j].Order })

	var dropped []string
	if maxRunes > 0 {
		for totalRunes(kept) > maxRunes {
			idx, worst := -1, PriorityEssential
			for i, s := range kept {
				if p := s.priority(); p > worst {
					worst, idx = p, i
				}
			}
			if idx < 0 {
				break // 只剩常驻段：预算再紧也不丢人格与环境
			}
			dropped = append(dropped, kept[idx].label())
			kept = append(kept[:idx], kept[idx+1:]...)
		}
	}
	return joinSections(kept), dropped
}

// joinSections 按序拼成正文；空正文段跳过。
func joinSections(sections []Section) string {
	var b strings.Builder
	for _, s := range sections {
		if s.Body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if s.Title != "" {
			b.WriteString("## " + s.Title + "\n")
		}
		b.WriteString(s.Body)
	}
	return b.String()
}

// EstimateTokens 粗估消息序列 token 数（rune 近似口径），供上下文占用透视使用。
func EstimateTokens(msgs []*llm.Message) int { return totalTokens(msgs) }

// totalRunes 估算段集合的 rune 占用（标题 + 正文 + 分隔）。
func totalRunes(sections []Section) int {
	n := 0
	for _, s := range sections {
		if s.Body == "" {
			continue
		}
		n += utf8.RuneCountInString(s.Body) + 2
		if s.Title != "" {
			n += utf8.RuneCountInString(s.Title) + 4
		}
	}
	return n
}

// RebuildHistory 清洗历史消息后再交给模型：
// 剔除空 assistant 占位、剔除孤儿 tool 结果（无前置 assistant 配对）、空结果补占位。
//
// 上游对「有 tool_calls 却无对应 tool 结果」会直接报 400，因此清洗必须在发请求前完成。
func RebuildHistory(msgs []*llm.Message) []*llm.Message {
	kept := make([]bool, len(msgs))
	for i, m := range msgs {
		// 空 assistant 占位：无正文也无工具调用，对模型无意义且易触发上游校验。
		kept[i] = !(m.Role == llm.RoleAssistant && m.Content == "" && len(m.ToolCalls) == 0)
	}

	known := make(map[string]bool)
	for i, m := range msgs {
		if !kept[i] || m.Role != llm.RoleAssistant {
			continue
		}
		for _, c := range m.ToolCalls {
			known[c.ID] = true
		}
	}

	out := make([]*llm.Message, 0, len(msgs))
	for i, m := range msgs {
		if !kept[i] {
			continue
		}
		if m.Role == llm.RoleTool {
			if !known[m.ToolCallID] {
				continue
			}
			if m.Content == "" {
				cp := *m
				cp.Content = "(empty)"
				m = &cp
			}
		}
		out = append(out, m)
	}
	return out
}
