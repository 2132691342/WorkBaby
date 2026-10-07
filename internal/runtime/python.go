package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/internal/pkg"
)

// 内置运行时版本与体积上限：体积上限防压缩包炸弹。
const (
	pythonVersion = "3.12.13"
	maxTotalBytes = 512 << 20
	// 打包时归档带一层顶层目录（python/...），解压时剥掉，
	// 保证 PythonDir 下直接就是 python.exe。
	pythonStripComponents = 1
)

// PythonExe 返回可用的 Python 可执行文件路径；内置优先，其次系统。
// 探测结果被缓存：一次进程生命周期内解压只需做一次，
// 而每个工具调用都问一次会让「首次启动要等几秒」变成「每次调用都卡一下」。
func PythonExe(p Paths) string {
	if exe, err := resolvePython(p); err == nil {
		return exe
	}
	if sys, err := exec.LookPath("python.exe"); err == nil {
		return sys
	}
	if sys, err := exec.LookPath("python"); err == nil {
		return sys
	}
	return ""
}

// PythonStatus 报告内置运行时的可用状态与失败原因，供启动日志与设置页展示。
type PythonStatus struct {
	Exe     string // 非空表示内置 Python 可用
	Err     string // 非空表示内置不可用时的原因（已内置到日志里）
	Source  string // "bundled" | "system" | ""
	Version string
}

// Status 探测内置 Python 并带上失败原因。诊断入口：只查不装，
// 调用方想「装」应显式调 EnsurePython，避免隐式解压藏在读路径里。
func Status(p Paths) PythonStatus {
	exe, err := EnsurePython(p)
	if err != nil {
		return PythonStatus{Err: err.Error()}
	}
	return PythonStatus{Exe: exe, Source: "bundled", Version: pythonVersion}
}

var (
	pythonOnce sync.Once
	pythonPath string
	pythonErr  error
)

// resolvePython 只解析一次：并发调用共享同一个结果。
func resolvePython(p Paths) (string, error) {
	pythonOnce.Do(func() {
		pythonPath, pythonErr = EnsurePython(p)
	})
	return pythonPath, pythonErr
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

// lfsPointerPrefix 是未拉取的 LFS 文件在磁盘上的开头。
// 只判存在性会把 pointer 当成 46MB 归档喂给 gzip，报错还完全指不到真因。
var lfsPointerPrefix = []byte("version https://git-lfs.github.com/spec/v1")

// isRealArchive 判归档是否真的下载到本地，而不只是 stat 得到一个文件。
func isRealArchive(path string) bool {
	if !pkg.FileExists(path) {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(lfsPointerPrefix))
	n, err := f.Read(head)
	if err != nil && n == 0 {
		return false
	}
	return !bytes.HasPrefix(head[:n], lfsPointerPrefix)
}

// EnsurePython 保证内置 Python 已解压并返回其路径。
func EnsurePython(p Paths) (string, error) {
	exe := filepath.Join(p.PythonDir, "python.exe")
	if versionMatches(p) && pkg.FileExists(exe) {
		return exe, nil
	}
	archive := ArchivePath(p)
	if !pkg.FileExists(archive) {
		return "", pkg.New(8001, "内置 Python 运行时包缺失，请在程序目录的 runtimes 下放 "+filepath.Base(archive), archive)
	}
	if !isRealArchive(archive) {
		return "", pkg.New(8002, "运行时包是 Git LFS 指针，尚未拉取真实文件", archive)
	}
	if err := extractTarGz(archive, p.PythonDir); err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(p.PythonDir, ".version"), pythonVersion); err != nil {
		return "", err
	}
	if !pkg.FileExists(exe) {
		return "", pkg.New(8001, "内置 Python 解压后找不到可执行文件", exe)
	}
	return exe, nil
}

// ArchivePath 定位随包分发的运行时压缩包。
// 按序探测：exe 同级（打包版）→ exe 上两级（仓库内直接跑）→ 兜底同级。
func ArchivePath(p Paths) string {
	name := "python-" + pythonVersion + "-win-x64.tar.gz"
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

// versionMatches 比对版本标记，版本一致就跳过解压。
func versionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(p.PythonDir, ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == pythonVersion
}

// extractTarGz 解压 tar.gz：逐条目校验路径，拒绝 .. 与绝对路径，并限制总体积。
func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return pkg.Wrap(8001, "打开运行时压缩包失败", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return pkg.Wrap(8001, "运行时压缩包格式不正确", err)
	}
	defer gz.Close()

	if err := pkg.EnsureDir(dest); err != nil {
		return err
	}
	var total int64
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return pkg.Wrap(8001, "读取运行时压缩包失败", err)
		}
		name := header.Name
		if pythonStripComponents > 0 {
			stripped, ok := stripTopLevel(name, pythonStripComponents)
			if !ok {
				continue
			}
			name = stripped
		}
		target, err := safeExtractPath(dest, name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return pkg.Wrap(8001, "创建运行时目录失败", err)
			}
		case tar.TypeReg:
			if err := writeEntry(target, tr, &total); err != nil {
				return err
			}
		}
	}
}

// stripTopLevel 去掉归档条目的前 n 层目录；条目层数不足时返回 false（整条跳过）。
func stripTopLevel(name string, n int) (string, bool) {
	parts := strings.SplitN(filepath.ToSlash(strings.TrimPrefix(name, "./")), "/", n+1)
	if len(parts) <= n || parts[n] == "" {
		return "", false
	}
	return parts[n], true
}

// safeExtractPath 拒绝路径穿越：这是解压外部归档的唯一安全边界。
func safeExtractPath(dest, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	// Windows 上 "/x" 不算 IsAbs（缺盘符），但它是带根路径，必须一并拒绝。
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") ||
		strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, `\`) {
		return "", pkg.New(8004, "压缩包里有不安全的路径", name)
	}
	target := filepath.Join(dest, cleaned)
	if !pkg.IsInside(dest, target) {
		return "", pkg.New(8004, "压缩包里有不安全的路径", name)
	}
	return target, nil
}

func writeEntry(target string, r io.Reader, total *int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return pkg.Wrap(8001, "创建运行时目录失败", err)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return pkg.Wrap(8001, "写入运行时文件失败", err)
	}
	defer out.Close()

	n, err := io.Copy(out, io.LimitReader(r, maxTotalBytes-*total))
	*total += n
	if err != nil {
		return pkg.Wrap(8001, "写入运行时文件失败", err)
	}
	if *total >= maxTotalBytes {
		return pkg.New(8001, "运行时压缩包体积超限", "")
	}
	return nil
}
