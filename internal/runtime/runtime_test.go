// 内置运行时链路：归档解压（顶层剥离 / 穿越拒绝 / LFS 指针跳过）与可用性探测。
// 归档未随 LFS 拉下来时跳过解压断言——判据是内容而非存在性：LFS pointer
// 同样是一个能 stat 成功的文件，只看存在会把 pointer 当归档喂给解压器。
package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/internal/pkg"
)

// lfsPointer 是未拉取的 LFS 文件在磁盘上的样子。
var lfsPointer = []byte("version https://git-lfs.github.com/spec/v1\n")

// hasRealArchive 判断归档是否真的下载到本地。
func hasRealArchive(t *testing.T, path string) bool {
	t.Helper()
	if !pkg.FileExists(path) {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(lfsPointer))
	if _, err := f.Read(head); err != nil {
		return false
	}
	return !bytes.HasPrefix(head, lfsPointer)
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// Python 从归档到「探测可用」的整条链路。
func TestPythonRuntimeChain(t *testing.T) {
	t.Run("解压剥离顶层目录", func(t *testing.T) {
		archive := filepath.Join("..", "..", "runtimes", "python-"+pythonVersion+"-win-x64.tar.gz")
		if !hasRealArchive(t, archive) {
			t.Skipf("运行时包未随 LFS 拉取，跳过: %s", archive)
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
	})

	t.Run("解压穿越路径被拒绝", func(t *testing.T) {
		dest := t.TempDir()
		for _, name := range []string{`..\evil.exe`, `/abs/evil.exe`, `a/../../evil.exe`} {
			if _, err := safeExtractPath(dest, name); err == nil {
				t.Fatalf("穿越路径未被拒绝: %q", name)
			}
		}
		if _, err := safeExtractPath(dest, `python/tools/x.exe`); err != nil {
			t.Fatalf("正常路径被误拒: %v", err)
		}
	})

	t.Run("路径解析在数据目录下", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("WORKBABY_HOME", root)
		p, err := Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if p.PythonDir != filepath.Join(root, "runtime", "python") {
			t.Fatalf("PythonDir 不在数据目录下: %s", p.PythonDir)
		}
	})

	// 探测必须认得「解压成功 + 版本标记已写」的内置 Python，
	// 只查系统 PATH 的话，内置解压成功也认不出来。
	t.Run("解压后探测命中", func(t *testing.T) {
		dir := t.TempDir()
		pythonDir := filepath.Join(dir, "runtime", "python")
		if err := os.MkdirAll(pythonDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pythonDir, "python.exe"), []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pythonDir, ".version"), []byte(pythonVersion), 0o644); err != nil {
			t.Fatal(err)
		}
		p := Paths{PythonDir: pythonDir, DataDir: dir}
		if got := EnsurePythonOrEmpty(p); got != filepath.Join(pythonDir, "python.exe") {
			t.Fatalf("解压并写标记后仍探测不到内置 Python，实际返回 %q", got)
		}
	})

	// 归档不在 exe 旁边时必须明确报错并带上归档文件名，
	// 静默返回空串会让上层以为「没装」，用户无从下手。
	t.Run("归档缺失报错带路径", func(t *testing.T) {
		dir := t.TempDir()
		p := Paths{PythonDir: filepath.Join(dir, "runtime", "python")}
		_, err := EnsurePython(p)
		if err == nil {
			t.Fatal("归档缺失时必须报错")
		}
		if want := ArchivePath(p); want != "" && !contains(err.Error(), filepath.Base(want)) {
			t.Fatalf("错误信息没有带上归档路径，用户无从下手: %v", err)
		}
	})
}

// PowerShell 官方 zip 平铺直解 + 归档探测位。
func TestPowerShellRuntimeChain(t *testing.T) {
	t.Run("解压平铺到根目录", func(t *testing.T) {
		archive := filepath.Join("..", "..", "runtimes", "PowerShell-"+powershellVersion+"-win-x64.zip")
		if !hasRealArchive(t, archive) {
			t.Skipf("运行时包未随 LFS 拉取，跳过: %s", archive)
		}
		dest := t.TempDir()
		if err := extractZip(archive, dest); err != nil {
			t.Fatalf("解压失败: %v", err)
		}
		if !pkg.FileExists(filepath.Join(dest, "pwsh.exe")) {
			t.Fatal("解压后 PowerShellDir 根下没有 pwsh.exe——官方 zip 平铺布局变了？")
		}
		if !pkg.FileExists(filepath.Join(dest, "System.Management.Automation.dll")) {
			t.Fatal("解压内容不完整：缺少 PowerShell 核心程序集")
		}
	})

	t.Run("归档探测兜底形状", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "WorkBaby.exe")
		if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("WORKBABY_TEST_EXE", exe) // 仅文档化探测序；真实探测走 os.Executable
		name := "PowerShell-" + powershellVersion + "-win-x64.zip"
		got := PowerShellArchivePath(Paths{})
		if filepath.Base(got) != name {
			t.Fatalf("兜底路径文件名不符: %s", got)
		}
	})
}
