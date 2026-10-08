// 内置运行时链路：归档与 SHA 常量一致、真实解压布局、解压路径穿越防护。
// 归档经 Git LFS 跟踪并内嵌进二进制；未拉取时内嵌的是 pointer 文本，
// 因此判据是内容而不是存在性——缺归档只代表内置运行时不可用，跳过而不是把 pointer 喂给解压器。
package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
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

// pythonStdlibZip 是 embeddable 包里的标准库归档名，随 Python 版本变化。
func pythonStdlibZip() string {
	parts := strings.Split(pythonVersion, ".")
	if len(parts) < 2 {
		return "python.zip"
	}
	return "python" + parts[0] + parts[1] + ".zip"
}

func TestBundledRuntimeChain(t *testing.T) {
	// 换归档忘了同步 SHA 常量，或者构建机没拉 LFS，都会一直滑到用户机器上才暴露。
	t.Run("归档与 SHA 常量一致", func(t *testing.T) {
		for _, c := range []struct{ name, want string }{
			{pythonArchiveName(), PythonArchiveSHA256},
			{powershellArchiveName(), PowerShellArchiveSHA256},
		} {
			t.Run(c.name, func(t *testing.T) {
				data := hasEmbeddedArchive(t, c.name)
				if c.want == "" {
					t.Fatalf("SHA 常量为空：用 sha256sum 实测 %s 后填进 Go 常量", c.name)
				}
				sum := sha256.Sum256(data)
				if got := hex.EncodeToString(sum[:]); got != c.want {
					t.Fatalf("归档与常量不一致：归档 %s，常量 %s", got, c.want)
				}
			})
		}
		if _, err := archiveBytes("不存在的归档.zip"); err == nil || !strings.Contains(err.Error(), "不存在的归档.zip") {
			t.Fatalf("缺失归档必须报错并带上文件名: %v", err)
		}
	})

	// 真实解压一次两份归档：官方 zip 布局变了会让内嵌运行时在用户机器上凭空消失。
	// 这是全仓最慢的一步（几千个文件落盘），用 -short 隔离——日常验证不必跑它。
	t.Run("归档解压平铺到根目录", func(t *testing.T) {
		if testing.Short() {
			t.Skip("真实解压：-short 跳过，碰运行时归档时跑全量")
		}
		for _, c := range []struct {
			name string
			want []string
		}{
			{pythonArchiveName(), []string{"python.exe", pythonStdlibZip()}},
			{powershellArchiveName(), []string{"pwsh.exe", "System.Management.Automation.dll"}},
		} {
			t.Run(c.name, func(t *testing.T) {
				zr, err := newZipReader(hasEmbeddedArchive(t, c.name))
				if err != nil {
					t.Fatalf("归档格式不正确: %v", err)
				}
				dest := t.TempDir()
				if err := extractZip(zr, dest); err != nil {
					t.Fatalf("解压失败: %v", err)
				}
				for _, w := range c.want {
					if !pkg.FileExists(filepath.Join(dest, w)) {
						t.Fatalf("解压后根目录下缺少 %s——官方 zip 布局变了？", w)
					}
				}
			})
		}
	})

	// zip-slip：归档里的 ..\ 与绝对路径必须被拒，否则解压能把文件写到数据目录之外。
	t.Run("解压路径穿越被拒绝", func(t *testing.T) {
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
}
