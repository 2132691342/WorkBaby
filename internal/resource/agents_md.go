package resource

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/pkg"
)

// AgentsMdMaxRunes 单份 AGENTS.md 注入上限：指令文件应写稳定约定而非长文，
// 超限截断并标注，避免一个失控的指令文件吃掉 system 段预算。
const AgentsMdMaxRunes = 8000

// agentsMdPiece 读一份 AGENTS.md 并装配为 system 段；文件不存在/读失败返回 ok=false
// （指令文件是可选约定，缺失不算错误、不落日志噪声）。
func agentsMdPiece(key, title, path string) (agent.Section, bool) {
	bs, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(bs))) == 0 {
		return agent.Section{}, false
	}
	body := strings.TrimSpace(string(bs))
	if runeLen := len([]rune(body)); runeLen > AgentsMdMaxRunes {
		body = pkg.TruncateRunes(body, AgentsMdMaxRunes) + "\n\n（指令文件过长，已截断；请精简 " + filepath.Base(path) + "）"
	}
	return agent.Section{Key: key, Title: title, Body: body, Order: AgentsMdOrder, Priority: agent.PriorityHigh}, true
}

// AgentsMdOrder AGENTS.md 在 system 段中的拼接顺序（persona → env → workspace → agents.md → memory → knowledge）。
// 暴露为公开常量便于测试 / 引用。
const AgentsMdOrder = 75

// LoadAgentsMd 加载项目指令段：用户全局（{home}/AGENTS.md）在前、工作区（{ws}/AGENTS.md）在后。
// 文件缺失静默跳过，不扫描子目录、不展开 import。
// home 为空时不加载全局；wsPath 为空时不加载工作区；都为空返回空切片。
func LoadAgentsMd(home, wsPath string) []agent.Section {
	pieces := make([]agent.Section, 0, 2)
	if home = strings.TrimSpace(home); home != "" {
		if p, ok := agentsMdPiece("agents_md_global", "项目指令（用户全局）", filepath.Join(home, "AGENTS.md")); ok {
			pieces = append(pieces, p)
		}
	}
	if wsPath = strings.TrimSpace(wsPath); wsPath != "" {
		if p, ok := agentsMdPiece("agents_md_workspace", "项目指令（工作区）", filepath.Join(wsPath, "AGENTS.md")); ok {
			pieces = append(pieces, p)
		}
	}
	return pieces
}