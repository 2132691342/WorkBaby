package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
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

// 上下文预算的策略量：窗口本身来自模型能力目录（domain.ModelCapabilityOf）。
// 余量缺省跟随真实下发的输出预算，用户设置只作为覆写。
const (
	defaultContextKeep = 20000
	// 压缩后保留量的上限：保留区跟着窗口涨，但不能让一次裁剪仍留下大半窗口，
	// 否则压缩完立刻又超预算，来回裁剪。
	maxContextKeep = 200000
)

// capabilityOf 取模型能力画像：用户配置 > 全局窗口设置 > 内置目录。
// 改窗口后输出预算要跟着重算，画像里的两个数永远一致。
func (c *ChatService) capabilityOf(providerID, model string) domain.ModelCapability {
	cap := domain.ModelCapabilityOf(model)
	if cfg, err := c.env.Repo.GetModelConfig(providerID, model); err == nil && cfg != nil {
		if cfg.ContextWindow > 0 {
			cap.ContextWindow = cfg.ContextWindow
			cap.Known = true
			cap.Note = ""
		}
		cap.Vision = cfg.Vision
		cap.ToolCall = cfg.ToolCall
		cap.MaxOutput = cap.OutputBudget()
		return cap
	}
	if raw, err := c.env.Repo.GetSetting(domain.SettingContextWindow); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(raw)); e == nil && n > 0 {
			cap.ContextWindow = n
			cap.Known = true
			cap.Note = ""
		}
	}
	cap.MaxOutput = cap.OutputBudget()
	return cap
}

// samplingOf 取采样参数与输出预算。cap 由调用方统一解析，一次装配只查一次库。
// 采样参数只在用户显式配置过时才下发（SamplingUnset 表示跟随上游默认）：
// 拿一个拍脑袋的默认温度覆盖所有模型，会在推理型与思考型模型上撞上游约束。
// 输出预算 = 窗口 1/8 与厂商硬上限取小（见 domain.ModelCapability.OutputBudget）。
func (c *ChatService) samplingOf(cap domain.ModelCapability, providerID, model string) (temp, topP *float64, maxOutput int) {
	maxOutput = cap.OutputBudget()
	cfg, err := c.env.Repo.GetModelConfig(providerID, model)
	if err != nil || cfg == nil {
		return nil, nil, maxOutput
	}
	if cfg.Temperature != domain.SamplingUnset {
		t := cfg.Temperature
		temp = &t
	}
	if cfg.TopP != domain.SamplingUnset {
		p := cfg.TopP
		topP = &p
	}
	return temp, topP, maxOutput
}

// maxTurns 读取轮数上限：设置表可覆写（前端暂不暴露，留给高级用户直接改库）。
func (c *ChatService) maxTurns() int {
	return c.intSetting(domain.SettingMaxTurns, domain.DefaultMaxTurns)
}

// toolParallel 读取工具并发度上限。
func (c *ChatService) toolParallel() int {
	return c.intSetting(domain.SettingToolParallel, domain.DefaultToolParallel)
}

// intSetting 读一个正整数设置，缺失或不合法时用缺省值。
func (c *ChatService) intSetting(key string, def int) int {
	if v, err := c.env.Repo.GetSetting(key); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			return n
		}
	}
	return def
}

// fmtPtr 把可空采样参数打成人话，日志里「默认」与「0」必须能分开。
func fmtPtr(v *float64) string {
	if v == nil {
		return "默认"
	}
	return strconv.FormatFloat(*v, 'g', -1, 64)
}

// applyStreamIdle 把「上游空闲超时」设置写进 llm 层：设置运行期可改，
// 每次 run 装配时同步一次（一次原子写，成本可忽略）；夹取 5 秒 ~ 30 分钟。
// 低于 5 秒会把推理模型的正常思考误判成断线，高于 30 分钟用户体感就是卡死。
func (c *ChatService) applyStreamIdle() {
	sec := c.intSetting(domain.SettingStreamIdleSec, 300)
	if sec < 5 {
		sec = 5
	}
	if sec > 1800 {
		sec = 1800
	}
	llm.SetStreamIdleTimeout(time.Duration(sec) * time.Second)
}

// budget 把「模型能力 + 用户设置」翻译成内核要的几个整数。
// 内核不读设置表也不认识模型名，压缩策略只在这里一处。
func (c *ChatService) budget(cap domain.ModelCapability, system string) agent.Budget {
	// 余量缺省 = 真正下发的输出预算：拿小余量去请大输出，压缩算出来的
	// 「还能塞多少」会虚高，总占用可能顶破窗口。用户设置只作为显式覆写。
	reserve := cap.OutputBudget()
	if v, err := c.env.Repo.GetSetting(domain.SettingContextReserve); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			reserve = n
		}
	}
	if reserve >= cap.ContextWindow {
		// 留的余量比整个窗口还大时预算恒为负，压缩会退化成什么都不裁。
		reserve = cap.ContextWindow / 4
	}
	// 保留区跟着窗口走：1M 窗口配 2 万保留量，压缩一次就把前面几十万 token
	// 的记忆全丢了，长任务表现为「它怎么把前提忘了」。
	keep := cap.ContextWindow / 4
	if keep < defaultContextKeep {
		keep = defaultContextKeep
	}
	if keep > maxContextKeep {
		keep = maxContextKeep
	}
	return agent.Budget{
		Window:      cap.ContextWindow,
		WindowKnown: cap.Known,
		Reserve:     reserve,
		Keep:        keep,
		// 估算口径必须与内核一致：走 agent.EstimateTokens 而不是再写一个 len/4，
		// 中文 system 提示词按字节算会低估约 1/4，压缩判断因此偏乐观。
		SystemTokens: agent.EstimateTokens(system, nil),
	}
}
