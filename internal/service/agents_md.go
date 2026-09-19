package service

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
)

// agentsMdMaxRunes 单份 AGENTS.md 注入上限：指令文件应写稳定约定而非长文，
// 超限截断并标注，避免一个失控的指令文件吃掉 system 段预算。
const agentsMdMaxRunes = 8000

// agentsMdPiece 读一份 AGENTS.md 并装配为 system 段；文件不存在/读失败返回 ok=false
// （指令文件是可选约定，缺失不算错误、不落日志噪声）。
func agentsMdPiece(key, title, path string) (core.Section, bool) {
	bs, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(bs))) == 0 {
		return core.Section{}, false
	}
	body := strings.TrimSpace(string(bs))
	if runeLen := len([]rune(body)); runeLen > agentsMdMaxRunes {
		body = pkg.TruncateRunes(body, agentsMdMaxRunes) + "\n\n（指令文件过长，已截断；请精简 " + filepath.Base(path) + "）"
	}
	return core.Section{Key: key, Title: title, Body: body, Order: orderAgentsMd, Priority: core.PriorityHigh}, true
}

// agentsMdPieces 项目指令段：用户全局（%APPDATA%/WorkBaby/AGENTS.md）在前、工作区（<workspace>/AGENTS.md）在后；
// 文件缺失静默跳过，不扫描子目录、不展开 import。
func agentsMdPieces(ses *domain.ChatSessionDO) []core.Section {
	pieces := make([]core.Section, 0, 2)
	if home := runtimeHome(); home != "" {
		if p, ok := agentsMdPiece("agents_md_global", "项目指令（用户全局）", filepath.Join(home, "AGENTS.md")); ok {
			pieces = append(pieces, p)
		}
	}
	if wp := strings.TrimSpace(ses.WorkspacePath); wp != "" {
		if p, ok := agentsMdPiece("agents_md_workspace", "项目指令（工作区）", filepath.Join(wp, "AGENTS.md")); ok {
			pieces = append(pieces, p)
		}
	}
	return pieces
}

// runtimeHome 数据目录的包级间接层：便于单测替换（生产走 runtime.Resolve）。
var runtimeHome = func() string {
	paths, err := runtime.Resolve()
	if err != nil {
		return ""
	}
	return paths.Home
}
