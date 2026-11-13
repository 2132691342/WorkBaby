package agent

import (
	"context"
	"strings"
	"time"

	"WorkBaby/backend/pkg"
)

// bashTool 跑一条命令行。Windows 上没有 bash，统一走 PowerShell。
type bashTool struct{}

// Spec 工具声明。
func (t *bashTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "bash",
		Description: "在工作区里执行一条命令并返回输出。Windows 上走 PowerShell；" +
			"适合跑测试、构建、git 这类需要外部程序的活，不要用它改文件（用 edit / write）。",
		Schema: schemaOf(map[string]any{
			"command":    "要执行的命令",
			"timeout_ms": "可选，超时毫秒，默认 120000，上限 600000",
		}, "command"),
	}
}

// Run 执行命令。argv 直传在这里的落法：解释器路径与 -Command 参数固定死，
// 语句整体作为一个参数进去，永不拼成命令行字符串。
func (t *bashTool) Run(ctx context.Context, reg *Registry, args map[string]any) Result {
	command := strings.TrimSpace(stringArg(args, "command"))
	if command == "" {
		return errorf("command 不能为空")
	}
	workspace := strings.TrimSpace(reg.Workspace())
	if workspace == "" {
		return errorf("还没有选择工作区")
	}
	timeout := intArg(args, "timeout_ms", 120_000)
	if timeout <= 0 {
		timeout = 120_000
	}
	if timeout > 600_000 {
		timeout = 600_000
	}
	shell := pkg.LookPath("powershell")
	if shell == "" {
		return errorf("系统里找不到 powershell，无法执行命令")
	}
	// 取消要能穿透到子进程：RunProc 自带超时，ctx 提前取消时用更短的超时兜住。
	effective := timeout
	if deadline, ok := ctx.Deadline(); ok {
		if left := int(time.Until(deadline).Milliseconds()); left > 0 && left < effective {
			effective = left
		}
	}
	result, err := pkg.RunProc(shell, []string{"-NoProfile", "-NonInteractive", "-Command", command}, pkg.ProcOptions{
		Dir:       workspace,
		TimeoutMs: effective,
	})
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	out := strings.TrimSpace(result.Stdout)
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		out += "\n[stderr]\n" + stderr
	}
	if out == "" {
		out = "（没有输出）"
	}
	text := "退出码 " + itoa(result.Code) + "\n" + pkg.Truncate(out, maxShellOutput)
	return Result{
		Text:    text,
		IsError: result.Code != 0,
		Details: map[string]any{"exit_code": result.Code, "command": command},
	}
}

// maxShellOutput 是命令输出进上下文的上限。
const maxShellOutput = 40_000

// unifiedDiff 生成统一差异文本，给界面的差异面板用。
// 同一行可以既是删除又是新增时，把它们配成一对：左右并排读得出「改成了什么」。
func unifiedDiff(path, before, after string) string {
	oldLines := pkg.SplitLines(before)
	newLines := pkg.SplitLines(after)
	var out strings.Builder
	out.WriteString("--- a/" + path + "\n")
	out.WriteString("+++ b/" + path + "\n")
	prefix, suffix := commonEdges(oldLines, newLines)
	oldMid := oldLines[prefix : len(oldLines)-suffix]
	newMid := newLines[prefix : len(newLines)-suffix]
	out.WriteString("@@ -" + itoa(prefix+1) + "," + itoa(len(oldMid)) +
		" +" + itoa(prefix+1) + "," + itoa(len(newMid)) + " @@\n")
	// 共同上下文只留首尾各三行：差异面板要的是改动本身。
	for i := 0; i < prefix && i < 3; i++ {
		out.WriteString(" " + oldLines[i] + "\n")
	}
	for _, line := range oldMid {
		out.WriteString("-" + line + "\n")
	}
	for _, line := range newMid {
		out.WriteString("+" + line + "\n")
	}
	for i := len(oldLines) - suffix; i < len(oldLines); i++ {
		if len(oldLines)-i <= 3 {
			out.WriteString(" " + oldLines[i] + "\n")
		}
	}
	return out.String()
}

// commonEdges 求两段文本首尾相同的行数。
func commonEdges(oldLines, newLines []string) (int, int) {
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	return prefix, suffix
}
