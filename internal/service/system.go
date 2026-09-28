package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 人设：面向办公新手，强调先看清再动手、说人话。
const persona = `你是 WorkBaby，一个能干活的桌面 AI 助手，用户是不懂命令行的办公人员。

工作原则：
- 先看清再动手：不知道文件在哪就先 ls / find，没读过就别改。
- 说人话：解释你在做什么，不要甩术语；出错时给一句「下一步可以怎么做」。
- 一次做一件事：把大任务拆成几步，做完一步说一句进展。
- 不编造：文件里没有的东西就说没有，不要用想象补齐。`

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

// 上下文窗口默认值与常见模型的识别规则。
const defaultContextWindow = 128000

// 预算缺省值：Reserve 给模型输出留余量，Keep 是裁剪后保留的近期 token 预算。
const (
	defaultContextReserve = 16384
	defaultContextKeep    = 20000
)

// contextWindowOf 粗判模型上下文窗口，用于计算压缩阈值；认不出来用默认值。
func contextWindowOf(model string) int {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "8k"):
		return 8192
	case strings.Contains(lower, "16k"):
		return 16384
	case strings.Contains(lower, "32k"):
		return 32768
	case strings.Contains(lower, "64k"):
		return 65536
	case strings.Contains(lower, "200k"):
		return 200000
	case strings.Contains(lower, "1m"):
		return 1000000
	default:
		return defaultContextWindow
	}
}

// budget 把「模型名 + 用户设置」翻译成内核要的三个整数。
// 内核不读设置表也不认识模型名，压缩策略只在这里一处。
func (c *ChatService) budget(model string) agent.Budget {
	reserve := defaultContextReserve
	if v, err := c.env.Repo.GetSetting(domain.SettingContextReserve); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			reserve = n
		}
	}
	return agent.Budget{
		Window:  contextWindowOf(model),
		Reserve: reserve,
		Keep:    defaultContextKeep,
	}
}
