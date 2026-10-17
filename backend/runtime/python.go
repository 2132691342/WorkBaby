// 内置 Python 运行时：官方 Windows embeddable zip 随二进制内嵌，首次启动解压到数据目录。
// 选 zip 而不是 tar.gz：Windows 上 zip 是原生格式，且官方只发 zip，少一次格式转换。
package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

// PythonExe 返回可用的 Python 可执行文件路径；内置优先，其次系统。
// 结果被缓存：每次工具调用都问一遍，会把「首次启动等几秒」变成「每次调用都卡一下」。
func PythonExe(p Paths) string {
	if exe, err := resolvePython(p); err == nil {
		return exe
	}
	return PythonSystemExe()
}

// PythonStatus 报告内置运行时的可用状态与失败原因，供启动日志与设置页展示。
type PythonStatus struct {
	Exe     string // 非空表示内置 Python 可用
	Err     string // 非空表示内置不可用时的原因（已内置到日志里）
	Source  string // "bundled" | "system" | ""
	Version string
}

// Status 探测内置 Python 并带上失败原因。它内部走 EnsurePython：首次调用会
// 真解压归档（装配期即由它完成首装），已解压则按 .version 直接命中。
func Status(p Paths) PythonStatus {
	exe, err := EnsurePython(p)
	if err != nil {
		return PythonStatus{Err: err.Error()}
	}
	return PythonStatus{Exe: exe, Source: "bundled", Version: pythonVersion}
}

// resolvePython 只解析一次：并发调用共享同一个结果；缓存可被 ResetProbe 清空。
// 真正的解压在 probeFlight 闸门下做，缓存命中的读路径不被它阻塞。
func resolvePython(p Paths) (string, error) {
	return probeOnce(&pythonProbed, &pythonPath, &pythonErr, func() (string, error) {
		return EnsurePython(p)
	})
}

// EnsurePythonOrEmpty 是 EnsurePython 的静默版本，供只想拿到路径、
// 不关心失败原因的调用方使用。
func EnsurePythonOrEmpty(p Paths) string {
	exe, err := EnsurePython(p)
	if err != nil {
		return ""
	}
	return exe
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

// pipBootstrapURL 引导 pip 用的脚本地址。
const pipBootstrapURL = "https://bootstrap.pypa.io/get-pip.py"

// EnsurePip 保证这个解释器上能跑 -m pip：官方 embeddable 包不带 pip，
// 有就直接返回，没有就下载引导脚本装一次（需联网）。
func EnsurePip(exe string) error {
	if exe == "" {
		return pkg.New("7004", "没有可用的 Python", "内置运行时没解压，系统里也没找到")
	}
	if err := exec.Command(exe, "-m", "pip", "--version").Run(); err == nil {
		return nil
	}
	dir, err := os.MkdirTemp("", "workbaby-pip")
	if err != nil {
		return pkg.Wrap("7004", "准备 pip 引导目录失败", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	script := filepath.Join(dir, "get-pip.py")
	if err := downloadFile(pipBootstrapURL, script); err != nil {
		return pkg.Wrap("7004", "下载 pip 引导脚本失败（需要联网）", err)
	}
	out, err := exec.Command(exe, script, "--no-warn-script-location").CombinedOutput()
	if err != nil {
		return pkg.New("7004", "引导 pip 失败", clip(string(out), 400))
	}
	return nil
}

// clip 截断到 n 个 rune：外部命令的输出可能很长，错误详情只放得下关键几行。
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// downloadFile 下载到指定路径，30 秒超时。
func downloadFile(url, dest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
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
