//go:build windows

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// newShellCmd 构造外壳子进程：Windows 下走 SysProcAttr.CmdLine 原样透传命令行。
// os/exec 的参数转义会破坏用户命令里的引号（`cmd /c "echo ok"`、`node "C:\Program Files\x.js"` 直接执行失败），
// 故绕过转义，由 cmd 自己解析。
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	exe := resolveCmdExe()
	c := exec.CommandContext(ctx, exe)
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: exe + " /c " + command}
	return c
}

// resolveCmdExe 定位 cmd.exe：优先 PATH，其次 System32 固定路径。
// PATH 不健全（被上层进程改写 / 精简）时退回固定路径——钩子子进程拉不起来会让所有钩子静默失效，
// 而「钩子没反应」在现场几乎无法归因。
func resolveCmdExe() string {
	if p, err := exec.LookPath("cmd"); err == nil {
		return p
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "cmd.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "cmd"
}
