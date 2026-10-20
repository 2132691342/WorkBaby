// 内置 Python 运行时：官方 Windows embeddable zip 随二进制内嵌，首次启动解压到数据目录。
// 选 zip 而不是 tar.gz：Windows 上 zip 是原生格式，且官方只发 zip，少一次格式转换。
package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// 内置运行时版本与体积上限：体积上限防压缩包炸弹。
const (
	pythonVersion = "3.12.8"
	maxTotalBytes = 512 << 20
)

// pythonArchiveName 是内嵌归档的文件名；与 bundled/ 下的实际文件、SHA 常量一一对应。
func pythonArchiveName() string { return "python-" + pythonVersion + "-embed-amd64.zip" }

// PythonArchiveSHA256 是内嵌归档的 SHA-256，换归档时同步更新。
// 它必须与版本常量同时改：否则版本标记命中会跳过解压，机器上永远用旧运行时。
const PythonArchiveSHA256 = "8d3f33be9eb810f23c102f08475af2854e50484b8e4e06275e937be61ce3d2fb"

// PythonStatus 报告内置运行时的可用状态与失败原因，供启动日志与设置页展示。
type PythonStatus struct {
	Exe     string // 非空表示内置 Python 可用
	Err     string // 非空表示内置不可用时的原因（已内置到日志里）
	Source  string // "bundled" | "system" | ""
	Version string
}

// Status 探测内置 Python 并带上失败原因。首次调用会真解压归档（装配期即由它完成首装），
// 已解压则按 .version 直接命中。
func Status(p Paths) PythonStatus {
	exe, err := EnsurePython(p)
	if err != nil {
		return PythonStatus{Err: err.Error()}
	}
	return PythonStatus{Exe: exe, Source: "bundled", Version: pythonVersion}
}

// PythonSystemExe 返回系统 PATH 上的 Python（内置不可用时的兜底）。
func PythonSystemExe() string {
	for _, name := range []string{"python.exe", "python"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return ""
}

// EnsurePython 保证内置 Python 已解压并返回其路径。
func EnsurePython(p Paths) (string, error) {
	exe := filepath.Join(p.PythonDir, "python.exe")
	if versionMatches(p) && pkg.FileExists(exe) {
		return exe, nil
	}
	data, err := archiveBytes(pythonArchiveName())
	if err != nil {
		return "", err
	}
	// 版本变了先清场：旧版本的 DLL 与新 python.exe 混放，本身就是一台坏运行时。
	if err := dropStaleRuntime(p.PythonDir, pythonVersion); err != nil {
		return "", err
	}
	reader, err := newZipReader(data)
	if err != nil {
		return "", err
	}
	if err := extractZip(reader, p.PythonDir); err != nil {
		return "", err
	}
	enableSite(p.PythonDir, pythonVersion, p.SitePackages)
	if err := pkg.WriteText(filepath.Join(p.PythonDir, ".version"), pythonVersion); err != nil {
		return "", err
	}
	if !pkg.FileExists(exe) {
		return "", pkg.New("7001", "内置 Python 解压后找不到可执行文件", exe)
	}
	return exe, nil
}

// enableSite 打开 embeddable 包默认关掉的 site，并把 site-packages 写进 ._pth。
// embeddable 版只按这个文件决定 sys.path，不认 PYTHONPATH——写进去才 import 得到。
func enableSite(dir, version, packages string) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return
	}
	path := filepath.Join(dir, "python"+parts[0]+parts[1]+"._pth")
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := strings.Replace(string(raw), "#import site", "import site", 1)
	if packages != "" && !strings.Contains(text, packages) {
		text = strings.TrimRight(text, "\r\n") + "\n" + packages + "\n"
	}
	_ = os.WriteFile(path, []byte(text), 0o644)
}

// dropStaleRuntime 清掉旧版本现场：只在版本标记存在且不一致时动手，
// 目录缺失或残留半份解压时不删——覆盖解压本身就能修好。
func dropStaleRuntime(dir, want string) error {
	raw, err := os.ReadFile(filepath.Join(dir, ".version"))
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(raw)) == want {
		return nil
	}
	return os.RemoveAll(dir)
}

// versionMatches 比对版本标记，版本一致就跳过解压。
func versionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(p.PythonDir, ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == pythonVersion
}
