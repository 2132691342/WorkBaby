//go:build !windows

package runtime

import "os/exec"

// hideWindow 在非 Windows 平台无操作。
func hideWindow(cmd *exec.Cmd) {}
