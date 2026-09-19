//go:build !windows

package service

import (
	"context"
	"os/exec"
)

// newShellCmd 非 Windows：POSIX shell 执行用户命令。
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
