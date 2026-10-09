// 内置 PowerShell 7 运行时：官方 zip 随二进制内嵌，首次使用解压到数据目录。
// 官方 zip 是平铺的（pwsh.exe 在压缩包根），解压不需要剥层。
package runtime

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

const powershellVersion = "7.4.2"

// PowerShellArchiveSHA256 是内嵌 PowerShell 归档的 SHA-256（与 Python 同一套守护：
// 换归档必须同步版本常量，否则旧解压目录会被版本标记一直当作有效）。
const PowerShellArchiveSHA256 = "1e43548e1000ef8220a24da3ea5113b140dd1b2301db03d732b48b980a887656"

// powershellArchiveName 是内嵌归档的文件名。
func powershellArchiveName() string { return "PowerShell-" + powershellVersion + "-win-x64.zip" }

// PowerShellBin 返回内置 pwsh 的可执行文件路径。
func PowerShellBin(p Paths) string { return filepath.Join(p.PowerShellDir, "pwsh.exe") }

// PowerShellExe 返回可用的 PowerShell 可执行文件路径；内置 pwsh 优先，其次系统。
// 与 Python 同一套缓存策略：一次进程生命周期内只解析一次，工具调用不再反复探测。
func PowerShellExe(p Paths) string {
	if exe, err := resolvePowerShell(p); err == nil {
		return exe
	}
	return PowerShellSystemExe()
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

// resolvePowerShell 只解析一次：并发调用共享同一个结果；缓存可被 ResetProbe 清空。
// 真正的解压在 probeFlight 闸门下做，缓存命中的读路径不被它阻塞。
func resolvePowerShell(p Paths) (string, error) {
	return probeOnce(&pwshProbed, &powershellPath, &powershellErr, func() (string, error) {
		return EnsurePowerShell(p)
	})
}

// EnsurePowerShell 保证内置 PowerShell 已解压并返回 pwsh 路径。
func EnsurePowerShell(p Paths) (string, error) {
	exe := PowerShellBin(p)
	if powershellVersionMatches(p) && pkg.FileExists(exe) {
		return exe, nil
	}
	data, err := archiveBytes(powershellArchiveName())
	if err != nil {
		return "", err
	}
	if err := dropStaleRuntime(p.PowerShellDir, powershellVersion); err != nil {
		return "", err
	}
	reader, err := newZipReader(data)
	if err != nil {
		return "", err
	}
	if err := extractZip(reader, p.PowerShellDir); err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(p.PowerShellDir, ".version"), powershellVersion); err != nil {
		return "", err
	}
	if !pkg.FileExists(exe) {
		return "", pkg.New(7001, "内置 PowerShell 解压后找不到 pwsh.exe", exe)
	}
	return exe, nil
}

// powershellVersionMatches 比对版本标记，一致就跳过解压。
func powershellVersionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(p.PowerShellDir, ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == powershellVersion
}
