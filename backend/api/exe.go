package api

import (
	"os"

	"WorkBaby/backend/pkg"
)

// currentExePath 取当前可执行文件路径（开机自启要写它）。
func currentExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", pkg.WrapErr(pkg.CodeInternal, "读不到可执行文件路径", err)
	}
	return exe, nil
}
