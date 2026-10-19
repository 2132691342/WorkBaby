// PI 入口解析：把 npm 的 Windows shim（pi.cmd / pi.ps1）还原成 node + JS 入口。
// 经 cmd.exe 起进程会闪控制台窗口，且 CREATE_NO_WINDOW 只作用于 cmd.exe 本身。
package env

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// resolvePiExe 把 PI 入口解析成 (exe, args)：
// shim 走 node + 入口脚本，已经是 .exe 就原样用。
// 解析不出脚本时退回原入口：跑错比不跑好，错误信息里带着真实原因。
func resolvePiExe(exe string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(exe))
	switch ext {
	case ".exe":
		return exe, nil
	case ".cmd", ".bat":
		// npm 的 shim 只做一件事：找到同目录的 node 与包里的 cli.js。
		dir := filepath.Dir(exe)
		script := shimScript(exe)
		if script == "" {
			return exe, nil
		}
		return shimNode(dir), []string{script}
	default:
		// .ps1 与无扩展名的 shim 同样是 node 脚本的包装。
		dir := filepath.Dir(exe)
		script := shimScript(exe)
		if script == "" {
			return exe, nil
		}
		return shimNode(dir), []string{script}
	}
}

// shimNode 取 shim 同目录的 node.exe，没有则用 PATH 上的 node。
func shimNode(dir string) string {
	local := filepath.Join(dir, "node.exe")
	if pkg.FileExists(local) {
		return local
	}
	if exe, err := exec.LookPath("node.exe"); err == nil {
		return exe
	}
	if exe, err := exec.LookPath("node"); err == nil {
		return exe
	}
	return "node"
}

// shimScript 从 shim 文本里抽出入参脚本路径。
// npm 的 shim 把入口写成 "%dp0%\node_modules\...\cli.js"，按行找 .js 即可。
func shimScript(shim string) string {
	raw, err := os.ReadFile(shim)
	if err != nil {
		return ""
	}
	text := pkg.DecodeText(raw)
	dir := filepath.Dir(shim)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "\r"))
		if !strings.HasSuffix(strings.ToLower(line), ".js") {
			continue
		}
		// shim 里的 %dp0% / $basedir 指的是 shim 自身所在目录。
		replacer := strings.NewReplacer("%dp0%", dir, "%dp0", dir, "$basedir", dir, "${dp0}", dir)
		path := replacer.Replace(strings.Trim(line, `"'`))
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if pkg.FileExists(path) {
			return path
		}
	}
	// shim 不含脚本路径时，按 npm 全局布局再猜一次：<prefix>/node_modules/<pkg>/dist/bundle/cli.js
	return guessBundleScript(dir)
}

// guessBundleScript 按 npm 全局包的固定布局找 PI 的入口脚本。
func guessBundleScript(dir string) string {
	const rel = `node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js`
	for _, base := range []string{dir, filepath.Dir(dir)} {
		candidate := filepath.Join(base, rel)
		if pkg.FileExists(candidate) {
			return candidate
		}
	}
	return ""
}
