// 运行时提级：把组件目录挂到系统级（机器 PATH）或降回用户级。
// 提级前先确认不会遮住机器上已有的那份——PATH 上多出一份 node 比没有更糟。
package env

import (
	"path/filepath"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// 可提级的组件与 PATH 级别：组件名与前端约定一致，别改字面量。
const (
	ComponentNode = "node"
	ComponentGit  = "git"
	ComponentPi   = "pi"

	PathLevelSystem = "system"
	PathLevelUser   = "user"
	PathLevelNone   = "none"
)

// LinkStatus 是一个组件在 PATH 上的落地情况与提级可行性。
type LinkStatus struct {
	Component string
	Dir       string
	Dirs      []string
	// Level 是目录挂在哪一级 PATH：system / user / both / none。
	Level      string
	Promotable bool
	// Conflict 是提级（系统级）会遮住的已有安装目录，空表示不冲突。
	Conflict string
}

// LinkOf 返回组件当前的 PATH 落地状态；只读注册表与文件，不起进程。
func (e *Env) LinkOf(component string) LinkStatus {
	dirs := e.promotionDirs(component)
	st := LinkStatus{Component: component, Dirs: dirs, Promotable: len(dirs) > 0}
	if len(dirs) == 0 {
		st.Level = PathLevelNone
		return st
	}
	st.Dir = dirs[0]
	machine, user := machinePathValue(), userPathValue()
	st.Level = pathLevelOf(machine, user, st.Dir)
	if component != ComponentPi {
		st.Conflict = e.linkConflict(component, dirs, PathLevelSystem)
	}
	return st
}

// Promote 把组件挪到指定 PATH 级别：system / user / none。
// 写系统级由 UAC 提权的本程序子进程完成；取消授权时 PATH 保持原样。
func (e *Env) Promote(component, level string) (LinkStatus, error) {
	switch component {
	case ComponentNode, ComponentGit, ComponentPi:
	default:
		return LinkStatus{}, pkg.New("9726", "只有 node / git / pi 三类运行时可以提级", component)
	}
	switch level {
	case PathLevelSystem, PathLevelUser, PathLevelNone:
	default:
		return LinkStatus{}, pkg.New("9726", "PATH 级别只有 system / user / none 三种", level)
	}
	if component == ComponentPi {
		// pi 的入口有自己的判定（还带「PATH 上另一份 pi 抢先」），别在提级里再走一遍。
		if _, err := e.SwitchTerminalLevel(level); err != nil {
			return e.LinkOf(component), err
		}
		return e.LinkOf(component), nil
	}
	dirs := e.promotionDirs(component)
	if len(dirs) == 0 {
		return e.LinkOf(component), pkg.New("9726", "这个组件还没有可提级的目录", component)
	}
	if level != PathLevelNone {
		if conflict := e.linkConflict(component, dirs, level); conflict != "" {
			return e.LinkOf(component), pkg.New("9725", "提级会遮住已经在用的那份 "+component, conflict)
		}
		if level == PathLevelSystem && pathHasEntry(machinePathValue(), dirs[0]) {
			return e.LinkOf(component), nil
		}
	}
	if err := setPathLevel(level, dirs); err != nil {
		return e.LinkOf(component), err
	}
	// 解释器目录变了：终端入口里钉的是绝对路径，得跟着重建。
	if pkg.FileExists(filepath.Join(e.terminalDir(), "pi.cmd")) {
		if err := e.writeTerminalShim(); err != nil {
			pkg.Warnf("env: 刷新终端入口失败: %v", err)
		}
	}
	e.resetProbes()
	return e.LinkOf(component), nil
}

// EnableBundledGit 解压内置 MinGit 并把它设为当前 git：机器上一份 git 都没有时才走这条路。
func (e *Env) EnableBundledGit() (GitPick, error) {
	if _, err := runtime.EnsureGitBash(e.paths); err != nil {
		return GitPick{}, err
	}
	e.resetProbes()
	pick := e.Toolchain().Git
	if pick.Exe == "" {
		return pick, pkg.New("7001", "内置 Git 解压后找不到 git.exe", e.paths.GitBashDir)
	}
	return pick, nil
}

// promotionDirs 返回组件要挂进 PATH 的目录：空表示这个组件当前没有可挂的目录。
func (e *Env) promotionDirs(component string) []string {
	switch component {
	case ComponentNode:
		if exe := e.Toolchain().Node.Exe; exe != "" {
			return []string{filepath.Dir(exe)}
		}
	case ComponentGit:
		if exe := e.Toolchain().Git.Exe; exe != "" {
			return []string{filepath.Dir(exe)}
		}
	case ComponentPi:
		if pkg.FileExists(filepath.Join(e.terminalDir(), "pi.cmd")) {
			return []string{e.terminalDir()}
		}
	}
	return nil
}

// linkConflict 返回提级后会遮住的已有目录，空表示不冲突。
// node 只有在「已有那份也不合规」时才允许提级：合规的那份不该被内置顶掉。
func (e *Env) linkConflict(component string, dirs []string, level string) string {
	// 系统级 PATH 排在用户级之前：提级到系统级会遮住两级里的全部同名安装；
	// 提到用户级只会遮住用户级的其余条目。
	scan := effectivePathDirs()
	if level == PathLevelUser {
		scan = pathDirsOf(userPathValue())
	}
	for _, dir := range scan {
		if dirIn(dirs, dir) {
			continue
		}
		switch component {
		case ComponentNode:
			exe := filepath.Join(dir, "node.exe")
			if pkg.FileExists(exe) && nodeSatisfies(nodeVersionOf(exe)) {
				return filepath.Clean(dir)
			}
		case ComponentGit:
			if pkg.FileExists(filepath.Join(dir, "git.exe")) {
				return filepath.Clean(dir)
			}
		}
	}
	return ""
}

// setPathLevel 把一组目录挪到指定级别：纯用户级的增删不走提权，否则一律经 UAC。
func setPathLevel(level string, dirs []string) error {
	switch level {
	case PathLevelSystem:
		return elevatePathAdmin(PathLevelSystem, dirs)
	case PathLevelUser:
		if anyInMachinePath(dirs) {
			return elevatePathAdmin(PathLevelUser, dirs)
		}
		for _, dir := range dirs {
			if err := prependUserPath(dir); err != nil {
				return err
			}
		}
		return nil
	case PathLevelNone:
		if anyInMachinePath(dirs) {
			return elevatePathAdmin(PathLevelNone, dirs)
		}
		for _, dir := range dirs {
			if err := removeUserPathEntry(dir); err != nil {
				return err
			}
		}
		return nil
	default:
		return pkg.New("9726", "PATH 级别只有 system / user / none 三种", level)
	}
}

// anyInMachinePath 判断目录在不在系统级 PATH 上：在就得提权才摘得掉。
func anyInMachinePath(dirs []string) bool {
	machine := machinePathValue()
	for _, dir := range dirs {
		if pathHasEntry(machine, dir) {
			return true
		}
	}
	return false
}

// pathDirsOf 把一条 PATH 串切成目录清单（去空项）。
func pathDirsOf(value string) []string {
	out := []string{}
	for _, dir := range filepath.SplitList(value) {
		if dir = trimQuotes(dir); dir != "" {
			out = append(out, filepath.Clean(dir))
		}
	}
	return out
}

// trimQuotes 去掉 PATH 条目里允许出现的包裹引号。
func trimQuotes(dir string) string {
	for len(dir) >= 2 && dir[0] == '"' && dir[len(dir)-1] == '"' {
		dir = dir[1 : len(dir)-1]
	}
	return dir
}

// dirIn 判断目录是否在给定清单里（大小写与尾斜杠不敏感）。
func dirIn(dirs []string, target string) bool {
	key := normalizePathKey(target)
	for _, dir := range dirs {
		if normalizePathKey(dir) == key {
			return true
		}
	}
	return false
}
