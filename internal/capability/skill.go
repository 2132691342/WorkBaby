package capability

import (
	"context"
	"strings"

	"WorkBaby/internal/core"
	"WorkBaby/internal/tool"
)

// SkillHit 一次 Skill 命中的完整信息：上下文注入与前端时间线回放共用同一份。
type SkillHit struct {
	Name        string
	Body        string
	Tools       []string // 工具白名单；空 = 不限制
	Source      string   // builtin / download / custom
	Version     string
	Description string
}

// SkillSummary 单个可用 Skill 的轻量索引项（名字 + 一句话用途）。
type SkillSummary struct {
	Name        string
	Description string
}

// SkillSource Skill 检索源（service.SkillService 实现）。
type SkillSource interface {
	// Match 按用户输入匹配 Skill 名；无命中返回空。
	Match(input string) string
	// Hit 取命中详情；未找到返回 ok=false。
	Hit(name string) (hit SkillHit, ok bool)
}

// SkillLister 可选能力：提供可用 Skill 的轻量清单。
// 未实现时未命中即零注入，模型完全不知道有哪些技能可用（只能靠 skill_run 盲试）。
type SkillLister interface {
	SkillSummaries() []SkillSummary
}

// skillCap 按用户输入触发 Skill 并把它的指导正文注入上下文。
//
// 脚本执行工具（skillrun）由装配方注册，此处不重复暴露。
type skillCap struct{ src SkillSource }

// NewSkill 构造 Skill 能力。
func NewSkill(src SkillSource) Capability { return &skillCap{src: src} }

// funcSource 函数适配器。
type funcSource struct {
	match     func(input string) string
	hit       func(name string) (SkillHit, bool)
	summaries func() []SkillSummary
}

// NewSkillSource 用函数构造 SkillSource：service 侧已有 Match/Get 实现时免写适配类型。
// list 可为 nil（未命中时不做索引注入）。
func NewSkillSource(match func(input string) string, hit func(name string) (SkillHit, bool), list func() []SkillSummary) SkillSource {
	return &funcSource{match: match, hit: hit, summaries: list}
}

// SkillSummaries 实现 SkillLister。
func (f *funcSource) SkillSummaries() []SkillSummary {
	if f.summaries == nil {
		return nil
	}
	return f.summaries()
}

func (f *funcSource) Match(input string) string {
	if f.match == nil {
		return ""
	}
	return f.match(input)
}

func (f *funcSource) Hit(name string) (SkillHit, bool) {
	if f.hit == nil {
		return SkillHit{}, false
	}
	return f.hit(name)
}

func (c *skillCap) ID() string { return "skill" }

func (c *skillCap) Tools() []tool.Tool { return nil }

func (c *skillCap) Preload(_ context.Context, p *PreloadCtx) ([]core.Section, error) {
	if c.src == nil {
		return nil, nil
	}
	name := c.src.Match(p.UserInput)
	if name == "" {
		// 未命中不是「没有 Skill」：注入一份轻量索引（名 + 一句话用途，≤800 rune），
		// 让模型知道有哪些能力可用、可主动引导用户触发，而不是对着空白上下文硬猜。
		return c.skillIndex(), nil
	}
	hit, ok := c.src.Hit(name)
	if !ok || hit.Body == "" {
		return nil, nil
	}
	// 正文预算双保险：本段自身先截断（超长 SKILL.md 会一口吃掉小窗口模型的全部预算），
	// 再声明低优先级交给装配器在总预算超限时整体丢弃。
	body := hit.Body
	if r := []rune(body); len(r) > maxSkillBodyRunes {
		body = string(r[:maxSkillBodyRunes]) +
			"\n…(技能正文超预算已截断；需要完整细节时向用户说明或查看技能原文)"
	}
	// 命中详情经运行态回传装配方：工具白名单约束本轮可见工具，其余字段供前端时间线回放
	if p.State != nil {
		p.State.SkillTools = hit.Tools
		p.State.SkillName = hit.Name
		p.State.SkillSource = hit.Source
		p.State.SkillVersion = hit.Version
		p.State.SkillDescription = hit.Description
		p.State.SkillInjectedLen = len(body)
	}
	return []core.Section{{
		Key:      "skill",
		Title:    "触发 Skill「" + name + "」，请严格按其指导执行",
		Body:     body,
		Priority: core.PriorityLow,
	}}, nil
}

// skillIndex 未命中时的可用 Skill 索引段；无可用 Skill 或无清单能力返回 nil。
func (c *skillCap) skillIndex() []core.Section {
	lister, ok := c.src.(SkillLister)
	if !ok {
		return nil
	}
	sums := lister.SkillSummaries()
	if len(sums) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("本轮未触发技能。若用户需求与下列技能相符，可直接按其用途作答或提示用户明确调用：\n")
	for _, s := range sums {
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if s.Description != "" {
			sb.WriteString("：")
			sb.WriteString(s.Description)
		}
		sb.WriteString("\n")
		if len([]rune(sb.String())) > maxSkillIndexRunes {
			break
		}
	}
	body := []rune(sb.String())
	if len(body) > maxSkillIndexRunes {
		body = body[:maxSkillIndexRunes]
	}
	return []core.Section{{
		Key:      "skill_index",
		Title:    "可用技能索引",
		Body:     string(body),
		Priority: core.PriorityLowest,
	}}
}

// maxSkillBodyRunes Skill 正文的注入上限（rune）。
const maxSkillBodyRunes = 6000

// maxSkillIndexRunes 未命中时的技能索引上限（rune）：只是「有什么可用」，不能抢占预算。
const maxSkillIndexRunes = 800

func (c *skillCap) Capture(_ context.Context, _ *CaptureCtx) error { return nil }
