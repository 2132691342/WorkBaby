package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
)

// pythonTool 用内置 Python 运行时执行脚本：办公新手最常需要的能力。
type pythonTool struct{}

func (pythonTool) Name() string                { return "python" }
func (pythonTool) Label() string               { return "跑脚本" }
func (pythonTool) ExecutionMode() ExecutionMode { return ExecutionSequential }
func (pythonTool) RequiresApproval() bool      { return true }
func (pythonTool) Description() string {
	return "执行一段 Python 代码，用于计算、处理表格与文本、批量处理文件。工作目录即当前会话工作目录。"
}
func (pythonTool) PromptSnippet() string { return "执行 Python 脚本" }
func (pythonTool) PromptGuidelines() []string {
	return []string{
		"算数、处理表格、批量改名这类事用 python，别手写一堆命令。",
		"能用 read / write / edit 解决的，不要跑脚本。",
		"只用标准库；需要第三方库时先告诉用户怎么装。",
	}
}

func (pythonTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code":       map[string]any{"type": "string", "description": "Python 代码"},
			"timeout_ms": map[string]any{"type": "integer", "description": "超时毫秒，默认 120000，上限 600000"},
		},
		"required": []string{"code"},
	}
}

func (t pythonTool) Execute(ctx context.Context, in Input) (*Result, error) {
	code := Str(in.Args, "code")
	if code == "" {
		return nil, ErrMissingArg
	}
	exe := in.Deps.PythonExe
	if exe == "" {
		return nil, pkg.New(8002, "没有可用的 Python", "内置运行时未解压，系统里也没找到")
	}
	tmpDir := in.Deps.TmpDir
	if tmpDir == "" {
		tmpDir = filepath.Join(os.TempDir(), "workbaby")
	}
	if err := pkg.EnsureDir(tmpDir); err != nil {
		return nil, err
	}
	script := filepath.Join(tmpDir, pkg.TempName("script", ".py"))
	if err := pkg.WriteText(script, code); err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(script) }()

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
	cmd := exec.CommandContext(runCtx, exe, script)
	cmd.Dir = in.Workspace
	cmd.Cancel = func() error { return killTree(cmd.Process.Pid) }
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = append(runtimePathEnv(exe), "PYTHONDONTWRITEBYTECODE=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	combined := normalizeEncoding(stdout.Bytes()) + normalizeEncoding(stderr.Bytes())
	elapsed := Since(start)

	if runCtx.Err() == context.DeadlineExceeded {
		return &Result{
			Content: fmt.Sprintf("脚本超时（%d 秒）已终止。\n", int(timeout.Seconds())) + Cut(combined, tmpDir, "python"),
			Title:   "脚本超时",
			Detail:  combined,
			IsError: true,
		}, nil
	}
	if err != nil {
		if strings.TrimSpace(combined) == "" {
			combined = err.Error()
		}
		return &Result{
			Content: Cut(combined, tmpDir, "python"),
			Title:   "脚本报错了",
			Detail:  combined,
			IsError: true,
		}, nil
	}
	if strings.TrimSpace(combined) == "" {
		combined = "（脚本执行成功，没有输出）"
	}
	return &Result{
		Content: Cut(combined, tmpDir, "python"),
		Title:   fmt.Sprintf("跑了一段 Python（%.1fs）", float64(elapsed)/1000),
		Detail:  combined,
	}, nil
}
