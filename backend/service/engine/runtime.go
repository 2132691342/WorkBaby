// Package engine 管引擎子进程：内置运行时探测、npm 装载。
// 它不知道业务语义，只回答「内置 node 在哪」「怎么用内置 npm 装包」。
package engine

import (
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// Runtime 是内置运行时的路径与版本探测入口。
type Runtime struct {
	// Layout 沙箱布局
	Layout pkg.Layout
}

// New 造一个运行时门面。
func New(layout pkg.Layout) *Runtime {
	return &Runtime{Layout: layout}
}

// NodeExe 内置 node.exe；没解压就报错。
// 绝不回退系统 node——那等于拿用户环境跑包，正是本壳要避免的事。
func (r *Runtime) NodeExe() (string, error) {
	exe := r.Layout.NodeExe()
	if !pkg.Exists(exe) {
		return "", pkg.Err(pkg.CodeRuntimeDown, "内置 Node.js 尚未就绪，稍等片刻再试")
	}
	return exe, nil
}

// NpmCli 内置 npm-cli.js。
func (r *Runtime) NpmCli() string { return r.Layout.NpmCli() }

// NpmCommand 是 pi 的 settings.json 里 npmCommand 的调用形状：node + npm-cli.js。
func (r *Runtime) NpmCommand() []string {
	return []string{r.Layout.NodeExe(), r.Layout.NpmCli()}
}

// GitExe 内置 git.exe；没有就报错，不回退系统 git。
func (r *Runtime) GitExe() (string, error) {
	exe := r.Layout.GitExe()
	if !pkg.Exists(exe) {
		return "", pkg.Err(pkg.CodeGitMissing, "内置 PortableGit 尚未就绪，稍等片刻再试")
	}
	return exe, nil
}

// PythonExe 内置 python.exe；没有就报错，不回退系统 python。
func (r *Runtime) PythonExe() (string, error) {
	exe := r.Layout.PythonExe()
	if !pkg.Exists(exe) {
		return "", pkg.Err(pkg.CodeRuntimeDown, "内置 Python 尚未就绪，稍等片刻再试")
	}
	return exe, nil
}

// PiEntry pi 的 CLI 入口。
func (r *Runtime) PiEntry() string { return r.Layout.PiEntry() }

// PiInstalled pi 本体是否已装。
func (r *Runtime) PiInstalled() bool { return r.Layout.PiInstalled() }

// npmArgs 组装一次 npm 调用；node 参数在前，npm-cli.js 在后。
func (r *Runtime) npmArgs(args ...string) []string {
	return append([]string{r.Layout.NpmCli()}, args...)
}

// InstallRootOf npm 包安装到哪个前缀：一律 data\pi-app。
func (r *Runtime) InstallRootOf() string { return r.Layout.PiAppDir }

// PackageDirOf 包在前缀下的目录；scope 包多一层。
func PackageDirPrefix(prefix string, pkgName string) string {
	return filepath.Join(prefix, "node_modules", pkgName)
}
