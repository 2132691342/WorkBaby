package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/tool"
)

// 人设：面向办公新手，强调先看清再动手、说人话。
const persona = `你是 WorkBaby，一个能干活的桌面 AI 助手，用户是不懂命令行的办公人员。

工作原则：
- 先看清再动手：不知道文件在哪就先 ls / find，没读过就别改。
- 说人话：解释你在做什么，不要甩术语；出错时给一句「下一步可以怎么做」。
- 一次做一件事：把大任务拆成几步，做完一步说一句进展。
- 不编造：文件里没有的东西就说没有，不要用想象补齐。

说话方式（必须遵守）：
- 不要用 emoji，一条都不要。
- 不要用「好的！」「当然！」「没问题！」这类客套开场，直接说事。
- 不要用「作为一个 AI」「希望这些对你有帮助」「还有什么可以帮您的吗」这类套话收尾。
- 不用「首先 / 其次 / 最后」「综上所述」「总而言之」堆段落，条目多就直接列条目。
- 有结果就报结果，没结果就报卡在哪，不要用模糊的「应该可以」「大概」。
- 不确定的事直接说不确定，不要用肯定的语气糊过去。`

// BuildSystem 组装系统提示。工具清单与准则由工具集驱动，换工具时提示词自动跟着变。
func BuildSystem(env *Env, tools []tool.Tool, workspace string) string {
	var b strings.Builder
	b.WriteString(persona)
	b.WriteString("\n\n")

	b.WriteString("## 可用工具\n")
	if len(tools) == 0 {
		b.WriteString("(none)\n")
	}
	for _, t := range tools {
		b.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.PromptSnippet()))
	}

	guidelines := buildGuidelines(tools)
	if guidelines != "" {
		b.WriteString("\n## 工具使用准则\n")
		b.WriteString(guidelines)
	}

	if env.Skills != nil {
		if xml := env.Skills.Render(); xml != "" {
			b.WriteString("\n## 可用技能\n")
			b.WriteString(xml)
		}
	}

	if workspace != "" {
		b.WriteString("\n当前工作目录：" + filepath.ToSlash(workspace) + "\n")
	}
	if doc := projectDoc(workspace); doc != "" {
		b.WriteString("\n" + doc)
	}
	return b.String()
}

// buildGuidelines 按当前启用工具拼准则：工具集变化会让准则自动跟着变。
func buildGuidelines(tools []tool.Tool) string {
	has := map[string]bool{}
	for _, t := range tools {
		has[t.Name()] = true
	}
	var b strings.Builder
	if has["powershell"] && !has["grep"] && !has["find"] && !has["ls"] {
		b.WriteString("- 没有文件浏览工具时，用 powershell 做 ls / find / 搜索。\n")
	}
	for _, t := range tools {
		for _, g := range t.PromptGuidelines() {
			b.WriteString("- " + g + "\n")
		}
	}
	b.WriteString("- 回答简洁，文件路径写清楚。\n")
	return b.String()
}

// projectDoc 读工作目录下的项目说明，让模型遵守用户自己定的规矩。
func projectDoc(workspace string) string {
	if workspace == "" {
		return ""
	}
	for _, name := range []string{"AGENTS.md", "AGENTS.MD", "CLAUDE.md"} {
		p := filepath.Join(workspace, name)
		if !pkg.FileExists(p) {
			continue
		}
		raw, err := pkg.ReadText(p)
		if err != nil {
			continue
		}
		return "<project_context>\n" + strings.TrimSpace(raw) + "\n</project_context>\n"
	}
	return ""
}

// 上下文预算的缺省值。窗口本身来自模型能力目录（domain.ModelCapabilityOf），
// 这里只管「留多少给输出」和「裁剪后保留多少」这两个策略量；
// 前者是下限，实际会抬到该模型真正下发的输出预算（见 budget）。
const (
	defaultContextReserve = 16384
	defaultContextKeep    = 20000
)

// capabilityOf 取模型能力画像。优先级：按「服务 + 模型」的用户配置 →
// 全局窗口设置（兼容旧数据）→ 内置目录。私有部署与新模型不在目录里时，
// 只有用户自己知道真实数字，所以模型设置页可以手填。
func (c *ChatService) capabilityOf(providerID, model string) domain.ModelCapability {
	cap := domain.ModelCapabilityOf(model)
	if cfg, err := c.env.Repo.GetModelConfig(providerID, model); err == nil && cfg != nil {
		if cfg.ContextWindow > 0 {
			cap.ContextWindow = cfg.ContextWindow
			cap.Known = true
			cap.Note = ""
		}
		if cfg.MaxOutput > 0 {
			cap.MaxOutput = cfg.MaxOutput
		}
		cap.Vision = cfg.Vision
		cap.ToolCall = cfg.ToolCall
		return cap
	}
	if raw, err := c.env.Repo.GetSetting(domain.SettingContextWindow); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(raw)); e == nil && n > 0 {
			cap.ContextWindow = n
			cap.Known = true
			cap.Note = ""
		}
	}
	return cap
}

// samplingOf 取温度、top_p 与最大输出：用户配置优先，其余回退到能力目录。
// 最大输出必须始终有值——留给推理模型的预算太小，它会「想完就没词」，
// 正文与工具调用一起断在 finish_reason=length。
func (c *ChatService) samplingOf(providerID, model string) (temp, topP *float64, maxOutput int) {
	t, p := domain.DefaultTemperature, domain.DefaultTopP
	maxOutput = domain.ModelCapabilityOf(model).MaxOutput
	if cfg, err := c.env.Repo.GetModelConfig(providerID, model); err == nil && cfg != nil {
		if cfg.Temperature > 0 {
			t = cfg.Temperature
		}
		if cfg.TopP > 0 {
			p = cfg.TopP
		}
		if cfg.MaxOutput > 0 {
			maxOutput = cfg.MaxOutput
		}
	}
	return &t, &p, maxOutput
}

// budget 把「模型能力 + 用户设置」翻译成内核要的几个整数。
// 内核不读设置表也不认识模型名，压缩策略只在这里一处。
// system 提示词由调用方传入：它是每次请求都会占窗口的部分。
func (c *ChatService) budget(providerID, model, system string) agent.Budget {
	reserve := defaultContextReserve
	if v, err := c.env.Repo.GetSetting(domain.SettingContextReserve); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			reserve = n
		}
	}
	cap := c.capabilityOf(providerID, model)
	// 余量必须容得下真正下发的输出预算：拿 16k 的余量去请 32k 的输出，
	// 压缩算出来的「还能塞多少」会虚高，总占用可能顶破窗口。
	if _, _, maxOutput := c.samplingOf(providerID, model); maxOutput > reserve {
		reserve = maxOutput
	}
	if reserve >= cap.ContextWindow {
		// 留的余量比整个窗口还大时预算恒为负，压缩会退化成什么都不裁。
		reserve = cap.ContextWindow / 4
	}
	return agent.Budget{
		Window:       cap.ContextWindow,
		WindowKnown:  cap.Known,
		Reserve:      reserve,
		Keep:         defaultContextKeep,
		SystemTokens: len(system) / 4,
	}
}
