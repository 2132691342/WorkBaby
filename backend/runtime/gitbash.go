// 内置 Git Bash（MinGit）：本机没有任何可用 bash 时才解压，给 PI 的 bash 工具一个能跑命令的环境。
// 这里只管解压与定位；什么时候解压（懒解压）、写不写进 PI 配置由 pi/env 的 shell 探测决定。
package runtime

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// gitBashVersion 与归档在 bundled/ 下的文件名一一对应。
const gitBashVersion = "2.56.0.3"

// gitBashArchiveName 是内嵌 MinGit 归档的文件名。
func gitBashArchiveName() string { return "MinGit-" + gitBashVersion + "-64-bit.zip" }

// GitBashArchiveSHA256 是内嵌归档的 SHA-256，换归档时同步更新；与 gitBashVersion
// 必须同时改，否则版本标记命中会跳过解压，机器上永远用旧运行时。
const GitBashArchiveSHA256 = "2e5619966e588fde5f8122c5557cad50c0fbafcaed736689f40ec1dfb1d78ea7"

// GitBashExe 返回已解压的内置 bash.exe 路径；没解压过返回空串，不触发解压。
func GitBashExe(p Paths) string {
	if !gitBashVersionMatches(p) {
		return ""
	}
	for _, rel := range []string{`bin\bash.exe`, `usr\bin\bash.exe`} {
		candidate := filepath.Join(p.GitBashDir, rel)
		if pkg.FileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// EnsureGitBash 保证内置 Git Bash 已解压并返回 bash.exe 的路径。
func EnsureGitBash(p Paths) (string, error) {
	if exe := GitBashExe(p); exe != "" {
		return exe, nil
	}
	data, err := archiveBytes(gitBashArchiveName())
	if err != nil {
		return "", err
	}
	if err := dropStaleRuntime(p.GitBashDir, gitBashVersion); err != nil {
		return "", err
	}
	reader, err := newZipReader(data)
	if err != nil {
		return "", err
	}
	// MinGit 是平铺布局（cmd/ usr/ ucrt64/ 直接在根下），不剥层。
	if err := extractZip(reader, p.GitBashDir); err != nil {
		return "", err
	}
	// 2.56 起 MinGit 只带 sh.exe：它就是 bash 的 POSIX 形态（同一二进制），
	// 复制一份命名 bash.exe，按 argv[0] 即回到正常 bash 模式。
	if !pkg.FileExists(filepath.Join(p.GitBashDir, `usr\bin\bash.exe`)) {
		sh := filepath.Join(p.GitBashDir, `usr\bin\sh.exe`)
		body, err := os.ReadFile(sh)
		if err != nil {
			return "", pkg.Wrap("7001", "内置 Git Bash 缺少 shell 入口", err)
		}
		if err := os.WriteFile(filepath.Join(p.GitBashDir, `usr\bin\bash.exe`), body, 0o755); err != nil {
			return "", pkg.Wrap("7001", "补内置 bash.exe 失败", err)
		}
	}
	// MSYS 启动会找根下的 tmp；缺了它每条命令都带一行告警。
	if err := pkg.EnsureDir(filepath.Join(p.GitBashDir, "tmp")); err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(p.GitBashDir, ".version"), gitBashVersion); err != nil {
		return "", err
	}
	exe := GitBashExe(p)
	if exe == "" {
		return "", pkg.New("7001", "内置 Git Bash 解压后找不到 bash.exe", p.GitBashDir)
	}
	return exe, nil
}

// gitBashVersionMatches 比对版本标记，一致才认为已解压。
func gitBashVersionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(p.GitBashDir, ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == gitBashVersion
}
