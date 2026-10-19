// Package piptest 给测试提供可编程的假 PI 进程与仓库定位，只被测试引用，不进主程序。
// 夹具本体在 backend/pi/pitest（package main）：它必须编译成独立 exe 才能真的起子进程。
package piptest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RepoRoot 从测试工作目录往上找 go.mod；测试要在仓库根编译夹具、拼相对路径。
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到仓库根目录（go.mod）")
		}
		dir = parent
	}
}

// Build 编译假 PI 到临时目录，返回 exe 路径。
// 一次编译就能撑起整条链路：每条子测试各编一次会把总时长做乘法。
func Build(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "fakepi.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./backend/pi/pitest")
	cmd.Dir = RepoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译假 PI 失败: %v\n%s", err, out)
	}
	return exe
}
