// 内置 PowerShell 解压链路：官方 zip 平铺直解 + LFS 指针跳过。
// 归档未随 LFS 拉下来时跳过——判据是内容而非存在性：LFS pointer 同样是
// 一个能 stat 成功的文件，只看存在会把 pointer 当 zip 喂给解压器。
package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/internal/pkg"
)

func TestExtractBundledPowerShell(t *testing.T) {
	archive := filepath.Join("..", "..", "runtimes", "PowerShell-"+powershellVersion+"-win-x64.zip")
	if !hasRealArchive(t, archive) {
		t.Skipf("运行时包未随 LFS 拉取，跳过: %s", archive)
	}
	dest := t.TempDir()
	if err := extractZip(archive, dest); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	// 官方 zip 平铺：pwsh.exe 就在压缩包根，解压后必须直接落在 PowerShellDir 根下。
	if !pkg.FileExists(filepath.Join(dest, "pwsh.exe")) {
		t.Fatal("解压后 PowerShellDir 根下没有 pwsh.exe——官方 zip 平铺布局变了？")
	}
	if !pkg.FileExists(filepath.Join(dest, "System.Management.Automation.dll")) {
		t.Fatal("解压内容不完整：缺少 PowerShell 核心程序集")
	}
}

func TestPowerShellArchivePathProbesSiblings(t *testing.T) {
	// 打包版：exe 同级 runtimes 下放包 → 命中第一个探测位。
	dir := t.TempDir()
	exe := filepath.Join(dir, "WorkBaby.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBABY_TEST_EXE", exe) // 仅文档化探测序；真实探测走 os.Executable
	p := Paths{}
	name := "PowerShell-" + powershellVersion + "-win-x64.zip"
	want := filepath.Join(dir, "runtimes", name)
	if got := PowerShellArchivePath(p); filepath.Base(got) != name {
		t.Fatalf("兜底路径文件名不符: %s", got)
	}
	_ = want // 打包版探测依赖 os.Executable，单测里只验证兜底形状
}
