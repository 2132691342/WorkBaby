// 内置 Python 的可用性判定：路径解析与「拿不到就降级」的行为。
// 这条链路在真实使用中反复出问题——归档解压成功、日志却一直说
// 「没有可用的 Python」，用户以为 python 工具坏了，其实是探测时机不对。

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPythonDirResolvedUnderDataDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKBABY_HOME", root)
	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.PythonDir != filepath.Join(root, "runtime", "python") {
		t.Fatalf("PythonDir 不在数据目录下: %s", p.PythonDir)
	}
}

// 解压完成并写入版本标记后，探测必须立刻返回可用路径。
// 曾经的 bug：探测只在系统 PATH 里找 python.exe，内置解压成功也认不出来。
func TestPythonExeFoundAfterExtraction(t *testing.T) {
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
}

// 归档不在 exe 旁边时（开发模式 / 免安装运行）必须明确报错，
// 而不是静默返回空串让上层以为「没装」。
func TestEnsurePythonReportsMissingArchiveWithPath(t *testing.T) {
	dir := t.TempDir()
	p := Paths{PythonDir: filepath.Join(dir, "runtime", "python")}
	_, err := EnsurePython(p)
	if err == nil {
		t.Fatal("归档缺失时必须报错")
	}
	if want := ArchivePath(p); want != "" && !contains(err.Error(), filepath.Base(want)) {
		t.Fatalf("错误信息没有带上归档路径，用户无从下手: %v", err)
	}
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
