// 系统提示与运行期参数：WorkBaby 不再拼系统提示（那在 PI 里），
// 这里只负责把「用户自定义指令 + 记忆 + 技能清单」写进 PI 能读到的位置。
package service

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// persona 是 WorkBaby 追加给 PI 的说话方式约束。
// PI 自己的系统提示负责工具与流程，这里只补「面向办公新手」的措辞纪律。
const persona = `你是 WorkBaby，一个能干活的桌面 AI 助手，用户是不懂命令行的办公人员。

说话方式（必须遵守）：
- 不要用 emoji，一条都不要。
- 不要用「好的！」「当然！」「没问题！」这类客套开场，直接说事。
- 不要用「作为一个 AI」「希望这些对你有帮助」「还有什么可以帮您的吗」这类套话收尾。
- 不用「首先 / 其次 / 最后」「综上所述」「总而言之」堆段落，条目多少就直接列条目。
- 有结果就报结果，没结果就报卡在哪，不要用模糊的「应该可以」「大概」。
- 不确定的事直接说不确定，不要用肯定的语气糊过去。
`

// appendBegin / appendEnd 是 WorkBaby 受控片段的边界。
// 只改标记块内的内容：块外是用户自己写的，整体覆盖会静默删掉他的东西。
const (
	appendBegin = "<!-- workbaby:begin -->"
	appendEnd   = "<!-- workbaby:end -->"
)

// SyncAppendSystem 重写 APPEND_SYSTEM.md：保住块外内容，只替换受控块。
func SyncAppendSystem(env *Env) error {
	path := env.PiEnv.AppendSystemPath()
	existing, err := readFileOrEmpty(path)
	if err != nil {
		return err
	}
	outer := stripControlledBlock(existing)

	block := buildAppendBody(env)
	var b strings.Builder
	b.WriteString(outer)
	if strings.TrimSpace(outer) != "" && !strings.HasSuffix(outer, "\n") {
		b.WriteString("\n")
	}
	if strings.TrimSpace(outer) != "" {
		b.WriteString("\n")
	}
	b.WriteString(appendBegin + "\n")
	b.WriteString(block)
	b.WriteString(appendEnd + "\n")
	return writeFileAtomic(path, []byte(b.String()))
}

// buildAppendBody 组装受控块内容：人设 + 记忆 + 工作区项目说明。
func buildAppendBody(env *Env) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(persona))
	b.WriteString("\n")
	if env.Memory != nil {
		if block := env.Memory.RenderForPrompt(); block != "" {
			b.WriteString("\n" + block + "\n")
		}
	}
	if env.Skills != nil {
		if xml := env.Skills.Render(); xml != "" {
			b.WriteString("\n## 可用技能\n")
			b.WriteString(xml)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// stripControlledBlock 去掉已有的受控块，留下用户自己的内容。
func stripControlledBlock(s string) string {
	start := strings.Index(s, appendBegin)
	if start < 0 {
		return s
	}
	end := strings.Index(s[start:], appendEnd)
	if end < 0 {
		return s[:start]
	}
	rest := s[start+end+len(appendEnd):]
	return s[:start] + rest
}

// projectDoc 读工作目录下的项目说明，让助手遵守用户自己定的规矩。
// 它写进 APPEND_SYSTEM 之外的独立文件（PI 自己也会扫 AGENTS.md），
// 这里只在需要显式提及时返回内容。
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

// capabilityOf 取模型能力画像：用户配置 > 全局窗口设置 > 内置目录。
func capabilityOf(env *Env, providerID, model string) domain.ModelCapability {
	cap := domain.ModelCapabilityOf(model)
	if cfg, err := env.Repo.GetModelConfig(providerID, model); err == nil && cfg != nil {
		merged := cap.WithConfig(cfg)
		merged.MaxOutput = merged.OutputBudget()
		return merged
	}
	if raw, err := env.Repo.GetSetting(domain.SettingContextWindow); err == nil {
		if n, e := parseInt(raw); e == nil && n > 0 {
			cap = cap.WithWindow(n)
		}
	}
	cap.MaxOutput = cap.OutputBudget()
	return cap
}

// ModelCapability 查模型能力画像。
func (c *ChatService) ModelCapability(providerID, model string) domain.ModelCapability {
	return capabilityOf(c.env, providerID, model)
}

// ModelCapabilities 批量查能力画像；空模型名直接跳过。
func (c *ChatService) ModelCapabilities(providerID string, models []string) []domain.ModelCapability {
	out := make([]domain.ModelCapability, 0, len(models))
	for _, m := range models {
		if strings.TrimSpace(m) == "" {
			continue
		}
		out = append(out, capabilityOf(c.env, providerID, m))
	}
	return out
}
