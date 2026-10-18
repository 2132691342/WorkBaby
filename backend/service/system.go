// 系统级装配：把 WorkBaby 的说话方式作为 PI 的附加系统提示，不改用户自己的配置文件。
package service

import (
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// persona 是 WorkBaby 追加给 PI 的说话方式约束。
// PI 自己的系统提示负责工具与流程，这里只补「面向办公人员」的措辞纪律。
const persona = `你是 WorkBaby，一个能干活的桌面 AI 助手，用户是不懂命令行的办公人员。

说话方式（必须遵守）：
- 不要用 emoji，一条都不要。
- 不要用「好的！」「当然！」「没问题！」这类客套开场，直接说事。
- 不要用「作为一个 AI」「希望这些对你有帮助」「还有什么可以帮您的吗」这类套话收尾。
- 不用「首先 / 其次 / 最后」「综上所述」「总而言之」堆段落，条目多少就直接列条目。
- 有结果就报结果，没结果就报卡在哪，不要用模糊的「应该可以」「大概」。
- 不确定的事直接说不确定，不要用肯定的语气糊过去。
`

// PersonaPath 返回附加系统提示文件的位置（在应用数据目录，不碰 PI 的 agent 目录）。
func PersonaPath(env *Env) string {
	return filepath.Join(env.Paths.DataDir, "persona.md")
}

// EnsurePersona 每次启动重写人设文件：它跟着版本走，旧版本的内容不该留在磁盘上。
func EnsurePersona(env *Env) (string, error) {
	path := PersonaPath(env)
	if err := pkg.WriteText(path, persona); err != nil {
		return "", err
	}
	return path, nil
}
