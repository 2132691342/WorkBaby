// 内置运行时链路：内嵌归档与 SHA 常量一致、解压路径穿越防护、真实解压布局。
// 坏了的表现：内嵌运行时在用户机器上凭空消失，或解压把文件写到数据目录之外。
package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
)

// hasEmbeddedArchive 取内嵌归档；缺失或仍是 LFS pointer 时跳过本条链路。
func hasEmbeddedArchive(t *testing.T, name string) []byte {
	t.Helper()
	data, err := bundledFS.ReadFile("bundled/" + name)
	if err != nil {
		t.Skipf("内嵌归档未放入 bundled/（应随仓库经 Git LFS 托管，克隆后先 git lfs pull）: %s", name)
	}
	if bytes.HasPrefix(data, lfsPointerPrefix) {
		t.Skipf("运行时包未随 LFS 拉取，跳过: %s", name)
	}
	return data
}

// TestBundledRuntimeChain 是 runtime 包唯一的测试入口：归档字节、解压安全与真实布局
// 都在同一份归档上，一条链验完。
func TestBundledRuntimeChain(t *testing.T) {
	// 换归档忘了同步 SHA 常量、构建机没拉 LFS（嵌进去的是指针文本）、归档里的 `..\`
	// 与绝对路径把文件写到数据目录之外、官方 zip 布局变了——同一份归档字节上一条链全验。
	t.Run("归档完整性、解压安全与真实解压布局", func(t *testing.T) {
		for _, c := range []struct{ name, want string }{
			{nodeArchiveName(), NodeArchiveSHA256},
			{gitBashArchiveName(), GitBashArchiveSHA256},
		} {
			data := hasEmbeddedArchive(t, c.name)
			if c.want == "" || c.want == "PENDING" {
				t.Fatalf("SHA 常量未填：用 sha256sum 实测 %s 后填进 Go 常量", c.name)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != c.want {
				t.Fatalf("%s 归档与常量不一致：归档 %s，常量 %s", c.name, got, c.want)
			}
		}

		dest := t.TempDir()
		for _, name := range []string{`..\evil.exe`, `/abs/evil.exe`, `a/../../evil.exe`} {
			if _, err := safeExtractPath(dest, name); err == nil {
				t.Fatalf("穿越路径未被拒绝: %q", name)
			}
		}
		if _, err := safeExtractPath(dest, `usr/bin/bash.exe`); err != nil {
			t.Fatalf("正常路径被误拒: %v", err)
		}

		// 真实解压两份归档：全仓最慢的一步，-short 只跳过它。Node 剥顶层目录平铺；
		// MinGit 走 EnsureGitBash 全链（它只带 sh.exe，bash.exe 是解压后补出来的）。
		if testing.Short() {
			t.Skip("真实解压：-short 跳过，碰运行时归档时跑全量")
		}
		zr, err := newZipReader(hasEmbeddedArchive(t, nodeArchiveName()))
		if err != nil {
			t.Fatalf("%s 归档格式不正确: %v", nodeArchiveName(), err)
		}
		nodeDir := t.TempDir()
		if err := extractZipStripped(zr, nodeDir, nodeArchiveRooted); err != nil {
			t.Fatalf("%s 解压失败: %v", nodeArchiveName(), err)
		}
		for _, w := range []string{"node.exe", "npm.cmd"} {
			if !pkg.FileExists(filepath.Join(nodeDir, w)) {
				t.Fatalf("%s 解压后根目录下缺少 %s——官方 zip 布局变了？", nodeArchiveName(), w)
			}
		}
		gitDir := t.TempDir()
		bashExe, err := EnsureGitBash(Paths{GitBashDir: gitDir})
		if err != nil {
			t.Fatalf("EnsureGitBash 失败: %v", err)
		}
		if !pkg.FileExists(bashExe) || !pkg.FileExists(filepath.Join(gitDir, `cmd\git.exe`)) {
			t.Fatalf("内置 Git Bash 解压布局不对: bash=%s", bashExe)
		}
		if again, err := EnsureGitBash(Paths{GitBashDir: gitDir}); err != nil || again != bashExe {
			t.Fatalf("EnsureGitBash 第二次应命中标记直接返回: %s != %s, err=%v", again, bashExe, err)
		}
	})
}
