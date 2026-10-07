package runtime

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/internal/pkg"
)

// PowerShell 官方 zip 是平铺的（pwsh.exe 在压缩包根），解压不需要剥层。
const powershellVersion = "7.4.2"

// PowerShellDir 返回内置 PowerShell 的解压目录：运行时根下与 Python 同级。
// 不在 Paths 上加字段——运行时根就是 PythonDir 的父目录，少一处要同步的地方。
func PowerShellDir(p Paths) string {
	return filepath.Join(filepath.Dir(p.PythonDir), "powershell")
}

// PowerShellBin 返回内置 pwsh 的可执行文件路径。
func PowerShellBin(p Paths) string { return filepath.Join(PowerShellDir(p), "pwsh.exe") }

// PowerShellExe 返回可用的 PowerShell 可执行文件路径；内置 pwsh 优先，其次系统。
// 与 Python 同一套缓存策略：一次进程生命周期内只解析一次，工具调用不再反复探测。
func PowerShellExe(p Paths) string {
	if exe, err := resolvePowerShell(p); err == nil {
		return exe
	}
	for _, name := range []string{"pwsh.exe", "pwsh", "powershell.exe", "powershell"} {
		if sys, err := exec.LookPath(name); err == nil {
			return sys
		}
	}
	return ""
}

// PowerShellStatus 报告内置 PowerShell 的可用状态与失败原因。
type PowerShellStatus struct {
	Exe     string // 非空表示内置 PowerShell 可用
	Err     string // 非空表示内置不可用时的原因（已写入日志）
	Source  string // "bundled" | "system" | ""
	Version string
}

// StatusPowerShell 探测内置 PowerShell，只查不装（与 Python 的 Status 同语义）。
func StatusPowerShell(p Paths) PowerShellStatus {
	exe, err := EnsurePowerShell(p)
	if err != nil {
		return PowerShellStatus{Err: err.Error()}
	}
	return PowerShellStatus{Exe: exe, Source: "bundled", Version: powershellVersion}
}

var (
	powershellOnce sync.Once
	powershellPath string
	powershellErr  error
)

// resolvePowerShell 只解析一次：并发调用共享同一个结果。
func resolvePowerShell(p Paths) (string, error) {
	powershellOnce.Do(func() {
		powershellPath, powershellErr = EnsurePowerShell(p)
	})
	return powershellPath, powershellErr
}

// EnsurePowerShellOrEmpty 是 EnsurePowerShell 的静默版本，供只要路径的调用方使用。
func EnsurePowerShellOrEmpty(p Paths) string {
	exe, err := EnsurePowerShell(p)
	if err != nil {
		return ""
	}
	return exe
}

// EnsurePowerShell 保证内置 PowerShell 已解压并返回 pwsh 路径。
func EnsurePowerShell(p Paths) (string, error) {
	exe := PowerShellBin(p)
	if powershellVersionMatches(p) && pkg.FileExists(exe) {
		return exe, nil
	}
	archive := PowerShellArchivePath(p)
	if !pkg.FileExists(archive) {
		return "", pkg.New(8011, "内置 PowerShell 运行时包缺失，请在程序目录的 runtimes 下放 "+filepath.Base(archive), archive)
	}
	if !isRealArchive(archive) {
		return "", pkg.New(8012, "运行时包是 Git LFS 指针，尚未拉取真实文件", archive)
	}
	if err := extractZip(archive, PowerShellDir(p)); err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(PowerShellDir(p), ".version"), powershellVersion); err != nil {
		return "", err
	}
	if !pkg.FileExists(exe) {
		return "", pkg.New(8011, "内置 PowerShell 解压后找不到 pwsh.exe", exe)
	}
	return exe, nil
}

// PowerShellArchivePath 定位随包分发的 PowerShell zip，探测序与 Python 一致：
// exe 同级（打包版）→ exe 上两级（仓库内直接跑）→ 兜底同级。
func PowerShellArchivePath(p Paths) string {
	name := "PowerShell-" + powershellVersion + "-win-x64.zip"
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, candidate := range []string{
		filepath.Join(dir, "runtimes", name),
		filepath.Join(dir, "..", "..", "runtimes", name),
	} {
		if pkg.FileExists(candidate) {
			return filepath.Clean(candidate)
		}
	}
	return filepath.Join(dir, "runtimes", name)
}

// powershellVersionMatches 比对版本标记，一致就跳过解压。
func powershellVersionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(PowerShellDir(p), ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == powershellVersion
}

// extractZip 解压 zip：逐条目走 safeExtractPath 拒绝穿越。
// 官方 zip 平铺无顶层目录，不剥层；体积上限复用 writeEntry 的总字节闸。
func extractZip(archive, dest string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return pkg.Wrap(8011, "打开 PowerShell 运行时压缩包失败", err)
	}
	defer reader.Close()

	if err := pkg.EnsureDir(dest); err != nil {
		return err
	}
	var total int64
	for _, item := range reader.File {
		target, err := safeExtractPath(dest, item.Name)
		if err != nil {
			return err
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return pkg.Wrap(8011, "创建运行时目录失败", err)
			}
			continue
		}
		rc, err := item.Open()
		if err != nil {
			return pkg.Wrap(8011, "读取 PowerShell 压缩项失败", err)
		}
		err = writeEntry(target, rc, &total)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
