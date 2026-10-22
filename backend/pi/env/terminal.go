// 终端里的 pi 命令：写一个固定指向内置 Node 的入口，并把它的目录挂到用户 PATH。
package env

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// TerminalStatus 是「在终端里敲 pi」这件事的可用状态。
type TerminalStatus struct {
	Ready  bool
	Shim   string
	Dir    string
	OnPath bool
	Error  string
}

// TerminalStatusOf 检查入口与 PATH，不起任何进程。
func (e *Env) TerminalStatusOf() TerminalStatus {
	dir := e.terminalDir()
	st := TerminalStatus{Dir: dir, Shim: filepath.Join(dir, "pi.cmd")}
	shimExists := pkg.FileExists(st.Shim)
	st.OnPath = pathHasEntry(userPathValue(), dir)
	st.Ready = shimExists && st.OnPath
	switch {
	case !shimExists:
		st.Error = "还没有生成终端入口"
	case !st.OnPath:
		st.Error = "入口已生成，但还没加进用户 PATH"
	}
	return st
}

// EnsureTerminal 生成 / 刷新终端入口，并确保它所在目录在用户 PATH 上。
// 入口写绝对路径：用户 PATH 上的 node 可能低于 PI 要求的下界。
func (e *Env) EnsureTerminal() (TerminalStatus, error) {
	if exe, _ := e.PiExe(); exe == "" {
		st := e.TerminalStatusOf()
		st.Error = ErrPiMissing.Error()
		return st, nil
	}
	if err := e.writeTerminalShim(); err != nil {
		return e.TerminalStatusOf(), err
	}
	dir := e.terminalDir()
	if !pathHasEntry(userPathValue(), dir) {
		if err := addUserPath(dir); err != nil {
			return e.TerminalStatusOf(), err
		}
	}
	return e.TerminalStatusOf(), nil
}

// terminalDir 是放 pi.cmd 的目录：与 npm 的前缀分开，升级 npm 包不会覆盖它。
func (e *Env) terminalDir() string { return filepath.Join(e.paths.DataDir, "bin") }

// writeTerminalShim 生成 pi.cmd：node + PI 的入口脚本，不经 npm 的 shim 中转。
func (e *Env) writeTerminalShim() error {
	exe, _ := e.PiExe()
	// 入口钉死内置 node：它随应用走，不会因为用户升级或卸载系统 node 而失效。
	node, _ := e.NodeExe()
	if bundled, err := runtime.EnsureNode(e.paths); err == nil {
		node = bundled
	}
	resolved, args := resolvePiExe(exe, node)
	if resolved == "" {
		return ErrNodeMissing
	}
	var body strings.Builder
	body.WriteString("@ECHO off\r\n")
	body.WriteString("@REM 由 WorkBaby 生成：换 PI 版本或重装后会自动刷新。\r\n")
	if len(args) > 0 && strings.EqualFold(filepath.Ext(args[0]), ".js") {
		body.WriteString("\"" + resolved + "\" \"" + args[0] + "\" %*\r\n")
	} else {
		// 抽不到脚本路径时退回原入口：跑错比不跑好，错误信息里带着真实原因。
		body.WriteString("call \"" + resolved + "\" %*\r\n")
	}
	if err := pkg.EnsureDir(e.terminalDir()); err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(e.terminalDir(), "pi.cmd"), []byte(body.String()), 0o755)
}

// pathHasEntry 判断 PATH 串里有没有某目录（Windows 不区分大小写与尾斜杠）。
func pathHasEntry(value, dir string) bool {
	target := normalizePathKey(dir)
	if target == "" {
		return false
	}
	for _, entry := range strings.Split(value, ";") {
		if normalizePathKey(entry) == target {
			return true
		}
	}
	return false
}

// normalizePathKey 把一条 PATH 项压成可比较的键；空串保持空（Clean 会把它变成 "."）。
func normalizePathKey(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}
	return strings.ToLower(strings.Trim(filepath.Clean(entry), `\/`))
}
