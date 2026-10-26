// 终端里的 pi 命令：写一个指向当前生效 Node 与 PI 的入口，并把它的目录挂到用户 PATH。
package env

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// TerminalStatus 是「在终端里敲 pi」这件事的可用状态。
type TerminalStatus struct {
	Ready bool
	Shim  string
	Dir   string
	// Path 是终端里敲 pi 实际会跑起来的那份；PiOnPath 时为 PATH 上已有的 pi。
	Path   string
	OnPath bool
	// PiOnPath 表示 pi 本来就在 PATH 上（用户自己的 npm 前缀），不需要转发入口。
	PiOnPath bool
	// Level 是入口目录挂在哪一级 PATH：system / user / both / none。
	Level string
	// PathFirst 表示 PATH 上排在入口目录之前的其它地方没有 pi：
	// 有的话终端会敲到那份，界面与终端就不是同一个内核。
	PathFirst bool
	// ShadowedBy 是排在入口目录之前、且真的含 pi 入口的目录（空表示没有）。
	ShadowedBy string
	// Others 是 PATH 上所有其它含 pi 入口的目录，用于提示「你机器上有几份 pi」。
	Others []string
	Error  string
}

// TerminalStatusOf 检查入口与 PATH，不起任何进程。
func (e *Env) TerminalStatusOf() TerminalStatus {
	dir := e.terminalDir()
	st := TerminalStatus{
		Dir: dir, Shim: filepath.Join(dir, "pi.cmd"), Path: filepath.Join(dir, "pi.cmd"),
		Others: []string{},
	}
	shimExists := pkg.FileExists(st.Shim)
	// 新开的终端走的是「系统级 PATH + 用户级 PATH」，只看进程 PATH 会漏掉后者。
	machine, user := machinePathValue(), userPathValue()
	effective := joinPathValues(machine, user)
	st.OnPath = pathHasEntry(effective, dir)
	st.Level = pathLevelOf(machine, user, dir)
	shadow, others := scanPiShims(effective, dir)
	st.ShadowedBy = shadow
	st.Others = others
	st.PathFirst = shadow == ""
	// pi 本来就在 PATH 上（装回了用户自己的 npm 前缀）：终端本来就敲得到，不需要入口。
	st.PiOnPath = e.PiOnPath()
	if st.PiOnPath && !shimExists {
		st.Ready, st.Path = true, firstNonEmpty(e.PiExe())
		return st
	}
	st.Ready = shimExists && st.OnPath && st.PathFirst
	switch {
	case !shimExists:
		st.Error = "还没有生成终端入口"
	case !st.OnPath:
		st.Error = "入口已生成，但还没加进 PATH"
	case !st.PathFirst:
		st.Error = "PATH 上还有排在前面、带着另一份 pi 的目录：" + shadow
	}
	return st
}

// firstNonEmpty 取第一个非空值（终端展示路径的兜底）。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// SwitchTerminalLevel 把终端入口在三档 PATH 之间挪动：system / user / none。
// 写系统级需要管理员，经 UAC 提权的本程序子进程完成；取消授权时保持原样返回。
func (e *Env) SwitchTerminalLevel(level string) (TerminalStatus, error) {
	dir := e.terminalDir()
	st := e.TerminalStatusOf()
	if !pkg.FileExists(st.Shim) {
		return st, pkg.New("9723", "先生成终端入口，再切换 PATH 级别", "")
	}
	switch level {
	case PathLevelSystem, PathLevelUser, PathLevelNone:
	default:
		return st, pkg.New("9723", "PATH 级别只有 system / user / none 三种", level)
	}
	if st.Level == level {
		return st, nil
	}
	if err := setPathLevel(level, []string{dir}); err != nil {
		return e.TerminalStatusOf(), err
	}
	return e.TerminalStatusOf(), nil
}

// pathLevelOf 判定目录挂在哪一级 PATH：system / user / both / none。
func pathLevelOf(machine, user, dir string) string {
	inMachine, inUser := pathHasEntry(machine, dir), pathHasEntry(user, dir)
	switch {
	case inMachine && inUser:
		return "both"
	case inMachine:
		return "system"
	case inUser:
		return "user"
	default:
		return "none"
	}
}

// joinPathValues 拼出新开终端会看到的那条 PATH：系统级在前，用户级在后。
func joinPathValues(machine, user string) string {
	switch {
	case machine != "" && user != "":
		return strings.TrimRight(machine, ";") + ";" + strings.TrimLeft(user, ";")
	case machine != "":
		return machine
	default:
		return user
	}
}

// scanPiShims 扫 PATH，返回排在 wantDir 之前的 pi 目录与全部其它 pi 目录。
// 判定一个目录「有 pi」只看有没有 pi / pi.cmd / pi.exe / pi.ps1 这几个名字。
func scanPiShims(pathValue, wantDir string) (shadow string, others []string) {
	target := normalizePathKey(wantDir)
	seen := map[string]bool{}
	reached := false
	for _, entry := range strings.Split(pathValue, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key := normalizePathKey(entry)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if key == target {
			reached = true
			continue
		}
		if !dirHasPi(entry) {
			continue
		}
		others = append(others, filepath.Clean(entry))
		if !reached && shadow == "" {
			shadow = filepath.Clean(entry)
		}
	}
	return shadow, others
}

// dirHasPi 判断一个目录里有没有 pi 的入口文件。
func dirHasPi(dir string) bool {
	for _, name := range []string{"pi.cmd", "pi.exe", "pi.ps1", "pi"} {
		if pkg.FileExists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// EnsureTerminal 生成 / 刷新终端入口，并把入口目录放到用户 PATH 最前面：
// 入口写绝对 node，目录必须前置，否则系统里另一份 pi 会抢先。
func (e *Env) EnsureTerminal() (TerminalStatus, error) {
	if exe, _ := e.PiExe(); exe == "" {
		st := e.TerminalStatusOf()
		st.Error = ErrPiMissing.Error()
		return st, nil
	}
	if err := e.writeTerminalShim(); err != nil {
		return e.TerminalStatusOf(), err
	}
	if err := prependUserPath(e.terminalDir()); err != nil {
		return e.TerminalStatusOf(), err
	}
	return e.TerminalStatusOf(), nil
}

// terminalDir 是放 pi.cmd 的目录：与 npm 的前缀分开，升级 npm 包不会覆盖它。
func (e *Env) terminalDir() string { return filepath.Join(e.paths.DataDir, "bin") }

// writeTerminalShim 生成 pi.cmd（CMD / PowerShell）与 pi（Git Bash 的 sh 脚本），两者都钉
// 当前生效的 node 与 PI（漂移时启动会刷新）。不写 .ps1：Restricted 执行策略下 `pi` 会直接报错。
func (e *Env) writeTerminalShim() error {
	exe, _ := e.PiExe()
	node, _ := e.NodeExe()
	resolved, args := resolvePiExe(exe, node)
	// 直接指定脚本文件时补上解释器，否则「入口」等于让系统去执行 .js。
	if strings.EqualFold(filepath.Ext(resolved), ".js") {
		resolved, args = node, append([]string{exe}, args...)
	}
	if resolved == "" {
		return ErrNodeMissing
	}
	if err := pkg.EnsureDir(e.terminalDir()); err != nil {
		return err
	}
	if err := WriteFileAtomic(filepath.Join(e.terminalDir(), "pi.cmd"), []byte(terminalShimBody(resolved, args)), 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(e.terminalDir(), "pi"), []byte(piShBody(resolved, args)), 0o755)
}

// terminalShimBody 生成 pi.cmd 的正文；args 非空且是 .js 时走 node + 脚本。
func terminalShimBody(resolved string, args []string) string {
	scripted := len(args) > 0 && strings.EqualFold(filepath.Ext(args[0]), ".js")
	var b strings.Builder
	b.WriteString("@ECHO off\r\n")
	b.WriteString("@REM 由 WorkBaby 生成：换 PI 版本或重装后会自动刷新。\r\n")
	if scripted {
		b.WriteString("\"" + resolved + "\" \"" + args[0] + "\" %*\r\n")
	} else {
		// 抽不到脚本路径时退回原入口：跑错比不跑好，错误信息里带着真实原因。
		b.WriteString("call \"" + resolved + "\" %*\r\n")
	}
	return b.String()
}

// piShBody 生成 Git Bash 敲 `pi` 走的 sh 脚本；路径转正斜杠，反斜杠在 sh 双引号里是转义符。
func piShBody(resolved string, args []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# 由 WorkBaby 生成：换 PI 版本或重装后会自动刷新。\n")
	if len(args) > 0 && strings.EqualFold(filepath.Ext(args[0]), ".js") {
		b.WriteString("exec \"" + filepath.ToSlash(resolved) + "\" \"" + filepath.ToSlash(args[0]) + "\" \"$@\"\n")
	} else {
		b.WriteString("exec \"" + filepath.ToSlash(resolved) + "\" \"$@\"\n")
	}
	return b.String()
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

// withPathFirst 返回「dir 在最前、其余条目保持原顺序、重复项去掉」的 PATH 串。
func withPathFirst(current, dir string) string {
	rest := splitPathEntries(current, dir)
	if len(rest) == 0 {
		return dir
	}
	return dir + ";" + strings.Join(rest, ";")
}

// withPathLast 返回「dir 追加在末尾、重复项去掉」的 PATH 串：
// 系统级整段排在用户级之前，末尾也已领先任何用户级条目。
func withPathLast(current, dir string) string {
	rest := splitPathEntries(current, dir)
	if len(rest) == 0 {
		return dir
	}
	return strings.Join(rest, ";") + ";" + dir
}

// withoutPathEntry 返回去掉 dir（含重复项）后的 PATH 串。
func withoutPathEntry(current, dir string) string {
	return strings.Join(splitPathEntries(current, dir), ";")
}

// splitPathEntries 把 PATH 切成条目并去掉目标目录与空项；移除和重排共用一个口径。
func splitPathEntries(current, dir string) []string {
	target := normalizePathKey(dir)
	rest := make([]string, 0, 24)
	for _, entry := range strings.Split(current, ";") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if target != "" && normalizePathKey(entry) == target {
			continue
		}
		rest = append(rest, entry)
	}
	return rest
}

// normalizePathKey 把一条 PATH 项压成可比较的键；空串保持空（Clean 会把它变成 "."）。
func normalizePathKey(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}
	return strings.ToLower(strings.Trim(filepath.Clean(entry), `\/`))
}

// expandPathValue 展开 %VAR% 形式的引用：系统级 PATH 常写成 %SystemRoot%\system32。
// 认不出的变量原样保留，不影响其余条目的判定。
func expandPathValue(v string) string {
	if !strings.ContainsRune(v, '%') {
		return v
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); {
		if v[i] == '%' {
			if j := strings.IndexByte(v[i+1:], '%'); j > 0 {
				if value, ok := os.LookupEnv(v[i+1 : i+1+j]); ok {
					b.WriteString(value)
					i += j + 2
					continue
				}
			}
		}
		b.WriteByte(v[i])
		i++
	}
	return b.String()
}
