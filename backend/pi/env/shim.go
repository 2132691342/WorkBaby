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

// resolvePiExe 把 PI 入口解析成 (exe, args)：shim 还原成 node + 入口脚本，.exe 原样用。
// nodeExe 是上层选定的绝对 node 路径；解析不出任何可执行文件时返回空串，由调用方报错。
func resolvePiExe(exe, nodeExe string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(exe))
	if ext == ".exe" {
		return exe, nil
	}
	// .cmd / .bat / .ps1 与无扩展名的 shim 都是 node 脚本的包装。
	dir := filepath.Dir(exe)
	script := shimScript(exe)
	if script == "" {
		// 抽不到脚本时退回原入口：跑错比不跑好，错误信息里带着真实原因。
		return exe, nil
	}
	return shimNode(dir, nodeExe), []string{script}
}

// shimNode 取上层选定的绝对 node；没有才用 shim 同目录的，最后才问 PATH。
// 绝不能退化成字面量 "node"：exec.Command 只查本进程的 PATH，内置 node 不在上面就是一句 not found。
func shimNode(dir, nodeExe string) string {
	// 顺序不能反：系统那份 pi.cmd 旁边的 node 往往比 PI 要求的版本旧。
	if nodeExe != "" && pkg.FileExists(nodeExe) {
		return nodeExe
	}
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
	return ""
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
