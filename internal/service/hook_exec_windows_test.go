//go:build windows

package service

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewShellCmdQuotedArgs 回归用例：用户命令含引号时不能被参数转义破坏
// （`cmd /c "echo ok"` 曾转义成 `\"echo ok\"` 而报「不是内部或外部命令」）。
func TestNewShellCmdQuotedArgs(t *testing.T) {
	for _, tc := range []struct {
		command string
		wantOut string
	}{
		{`echo ok`, "ok"},
		{`cmd /c "echo ok"`, "ok"},
		{`cmd /c ver`, "Windows"},
	} {
		cmd := newShellCmd(t.Context(), tc.command)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		require.NoError(t, cmd.Run(), tc.command)
		require.Contains(t, buf.String(), tc.wantOut, tc.command)
	}
}
