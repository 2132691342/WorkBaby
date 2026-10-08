// Package runtimetest 给装配类测试预置「运行时已就绪」标记，跳过上百 MB 的归档解压。
// 只被测试引用，不进主程序；解压本身的真实性由 runtime 包的真实解压测试守着。
package runtimetest

import (
	"os"
	"path/filepath"
	"testing"
)

// SeedMarkers 写出 Python / PowerShell 的版本标记与可执行文件占位，
// 让 EnsurePython / EnsurePowerShell 命中「已解压」分支跳过真解压。
func SeedMarkers(t *testing.T, root string) {
	t.Helper()
	for _, r := range []struct{ dir, ver, exe string }{
		{"python", "3.12.8", "python.exe"},
		{"powershell", "7.4.2", "pwsh.exe"},
	} {
		dir := filepath.Join(root, "runtime", r.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".version"), []byte(r.ver), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, r.exe), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
