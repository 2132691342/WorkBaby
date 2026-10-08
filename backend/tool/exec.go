package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 默认与上限超时：上限 30 分钟（构建 / 安装 / 批量转码的量级），
// 防止死循环命令长期占用进程树。
const (
	defaultTimeout = 120 * time.Second
	maxTimeout     = 1800 * time.Second
)

// powershellTool 在 Windows 上执行 PowerShell 命令，是模型真正「干活」的出口。
type powershellTool struct{}

func (powershellTool) Name() string                { return "powershell" }
func (powershellTool) Label() string               { return "跑命令" }
func (powershellTool) ExecutionMode() ExecutionMode { return ExecutionSequential }
func (powershellTool) RequiresApproval() bool      { return true }
func (powershellTool) Description() string {
	return "执行一条 PowerShell 命令。用于文件批量操作、压缩解压、调用系统工具等。"
}
func (powershellTool) PromptSnippet() string { return "执行 PowerShell 命令" }
func (powershellTool) PromptGuidelines() []string {
	return []string{
		"能用 read / write / edit / ls / find / grep 解决的，不要跑命令。",
		"删除、覆盖类命令要先说清楚影响范围，等用户确认。",
	}
}

func (powershellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "PowerShell 命令"},
			"cwd":        map[string]any{"type": "string", "description": "工作目录，默认当前工作目录"},
			"timeout_ms": map[string]any{"type": "integer", "description": "超时毫秒，默认 120000，上限 1800000"},
		},
		"required": []string{"command"},
	}
}

func (t powershellTool) Execute(ctx context.Context, in Input) (*Result, error) {
	command := Str(in.Args, "command")
	if command == "" {
		return nil, ErrMissingArg
	}
	cwd := in.Workspace
	if v := Str(in.Args, "cwd"); v != "" {
		full, err := pkg.SafeJoin(in.Workspace, v)
		if err != nil {
			return nil, err
		}
		cwd = full
	}
	timeout := time.Duration(Int(in.Args, "timeout_ms", int(defaultTimeout.Milliseconds()))) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	// 内置 pwsh 优先（Deps 注入），兜底系统 powershell；flags 两代通用。
	shell := in.Deps.PowerShellExe
	if shell == "" {
		shell = "powershell.exe"
	}
	cmd := exec.CommandContext(runCtx, shell, "-NoProfile", "-NonInteractive", "-Command", command)
	hideConsole(cmd)
	cmd.Dir = cwd
	cmd.Cancel = func() error { return killTree(cmd.Process.Pid) }
	cmd.WaitDelay = 3 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = runtimePathEnv(in.Deps.PythonExe)

	err := cmd.Run()
	// 命令输出保留结尾：失败原因、堆栈尾巴、构建结论都在最后几行。
	// 同时洗掉控制字符：一段带 \x1b[?… 的输出能把整条 SSE 帧打崩。
	combined := Sanitize(normalizeEncoding(stdout.Bytes()) + normalizeEncoding(stderr.Bytes()))
	elapsed := Since(start)

	if runCtx.Err() == context.DeadlineExceeded {
		return &Result{
			Content: fmt.Sprintf("命令超时（%d 秒）已终止。\n%s", int(timeout.Seconds()), CutTail(combined, in.Deps.TmpDir, "exec")),
			Title:   "命令超时",
			Detail:  combined,
			IsError: true,
		}, nil
	}
	if runCtx.Err() == context.Canceled {
		return &Result{Content: "命令已被停止。", Title: "命令已停止", Detail: combined, IsError: true}, nil
	}
	// 非零退出码不算工具错误：grep 无匹配、git diff --exit-code 都返回非零，
	// 成功与否由模型自己解读。只有执行层失败（找不到 shell）才算工具错误。
	var exitErr *exec.ExitError
	switch {
	case err != nil && errors.As(err, &exitErr):
		return &Result{
			Content: fmt.Sprintf("退出码 %d\n%s", exitErr.ExitCode(), CutTail(combined, in.Deps.TmpDir, "exec")),
			Title:   "命令非零退出",
			Detail:  combined,
		}, nil
	case err != nil:
		return &Result{
			Content: fmt.Sprintf("命令没有跑起来：%v", err),
			Title:   "命令没跑起来",
			Detail:  combined,
			IsError: true,
		}, nil
	}
	if strings.TrimSpace(combined) == "" {
		combined = "（命令执行成功，没有输出）"
	}
	return &Result{
		Content: CutTail(combined, in.Deps.TmpDir, "exec"),
		Title:   fmt.Sprintf("跑了一条命令（%.1fs）", float64(elapsed)/1000),
		Detail:  combined,
	}, nil
}

// normalizeEncoding 把命令输出归一为 UTF-8：Windows 命令行默认 GBK，
// PowerShell 重定向又常产出 UTF-16，统一交给 pkg.DecodeText。
func normalizeEncoding(b []byte) string {
	return pkg.DecodeText(b)
}

// RiskOf 给命令定风险等级，只用于 UI 展示，不改变审批判定。
func RiskOf(command string) string {
	lower := strings.ToLower(command)
	for _, k := range []string{"remove-item", "del ", "rm ", "format-", "reg ", "net ", "stop-process", "taskkill"} {
		if strings.Contains(lower, k) {
			return domain.RiskHigh
		}
	}
	for _, k := range []string{"set-content", "out-file", "move-item", "rename-item", "new-item"} {
		if strings.Contains(lower, k) {
			return domain.RiskMedium
		}
	}
	return domain.RiskLow
}
