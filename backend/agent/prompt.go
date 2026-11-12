package agent

import (
	"strings"
)

// SystemPrompt 组装系统提示。
// 三段：身份与纪律、工具清单、工作区事实。工具从注册表现读，
// 提示里写的工具集绝不会与实际可调用的工具不一致。
func SystemPrompt(reg *Registry, workspace string) string {
	var out strings.Builder
	out.WriteString(identity)
	out.WriteString("\n\n## 可用工具\n\n")
	out.WriteString("你可以调用下面这些工具。需要信息就去取，不要猜：\n\n")
	for _, spec := range reg.Specs() {
		out.WriteString("- `")
		out.WriteString(spec.Name)
		out.WriteString("`：")
		out.WriteString(spec.Description)
		out.WriteString("\n")
	}
	if blocked := blockedTools(reg); len(blocked) > 0 {
		out.WriteString("\n## 计划模式\n\n")
		out.WriteString("现在是计划模式：只能读，不能改。以下工具会被拦下：")
		out.WriteString(strings.Join(blocked, "、"))
		out.WriteString("。\n")
		out.WriteString("请先用读类工具把现状看清楚，再把方案写清楚。")
		out.WriteString("不要反复尝试被拦下的工具——它们不会成功。\n")
	}
	if workspace != "" {
		out.WriteString("\n## 工作区\n\n当前工作区根目录：`")
		out.WriteString(workspace)
		out.WriteString("`\n工具里的相对路径都相对这个目录解析，路径不能越出它。\n")
	}
	return out.String()
}

// blockedTools 列出当前会被拦下的工具名；不在计划模式时给空。
func blockedTools(reg *Registry) []string {
	if reg == nil || reg.Policy == nil || !reg.Policy.ReadOnly() {
		return nil
	}
	out := make([]string, 0, 4)
	for _, spec := range reg.Specs() {
		if IsMutatingTool(spec.Name) {
			out = append(out, spec.Name)
		}
	}
	return out
}

// identity 是固定的身份与行为纪律。
const identity = `你是 WorkBaby，一个在用户电脑上干活的 AI 助手。

工作方式：
- 动手前先看清楚现状。要改文件就先 read，要跑命令就先确认环境。
- 一次只做用户要求的这件事，不顺手大改无关代码。
- 改完要说清楚改了什么文件、为什么这么改。
- 工具报错时读错误信息再纠正，不要用同样的参数重试。
- 不确定的事情直说不确定，不要编造文件内容或命令输出。
- 回复用用户使用的语言，简洁、具体，不要复述工具输出。`
