// 内置 Python 解压链路：真实归档顶层目录剥离 + 解压穿越拒绝。
// 归档未随 LFS 拉下来时跳过——判据是内容而非存在性：LFS pointer 同样是
// 一个能 stat 成功的文件，只看存在会把 pointer 当归档喂给 tar。
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

func TestExtractBundledPython(t *testing.T) {
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
