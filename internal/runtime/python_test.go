package runtime

import (
	"path/filepath"
	"testing"

	"WorkBaby/internal/pkg"
)

// 覆盖内置 Python 解压链路：真实归档 + 顶层目录剥离 + 穿越拒绝。
// 归档不在仓库时跳过——CI 可以选择不下载 46MB 的运行时包。

func TestExtractBundledPython(t *testing.T) {
	archive := filepath.Join("..", "..", "runtimes", "python-"+pythonVersion+"-win-x64.tar.gz")
	if !pkg.FileExists(archive) {
		t.Skipf("运行时包不存在: %s", archive)
	}
	dest := t.TempDir()
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if !pkg.FileExists(filepath.Join(dest, "python.exe")) {
		t.Fatal("解压后 PythonDir 根下没有 python.exe——stripComponents 没生效")
	}
	if !pkg.FileExists(filepath.Join(dest, "DLLs")) && !pkg.DirExists(filepath.Join(dest, "DLLs")) {
		t.Fatal("解压内容不完整：缺少 DLLs 目录")
	}
}

func TestSafeExtractPathRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{`..\evil.exe`, `/abs/evil.exe`, `a/../../evil.exe`} {
		if _, err := safeExtractPath(dest, name); err == nil {
			t.Fatalf("穿越路径未被拒绝: %q", name)
		}
	}
	if _, err := safeExtractPath(dest, `python/tools/x.exe`); err != nil {
		t.Fatalf("正常路径被误拒: %v", err)
	}
}
