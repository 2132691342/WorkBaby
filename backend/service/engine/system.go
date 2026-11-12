package engine

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// NodeVersion 系统 node 的版本；没装给空串。
func (r *Runtime) NodeVersion() string { return r.systemVersion("node", "--version") }

// GitVersion 系统 git 的版本。
func (r *Runtime) GitVersion() string { return r.systemVersion("git", "--version") }

// PythonVersion 系统 python 的版本。
func (r *Runtime) PythonVersion() string { return r.systemVersion("python", "--version") }

// systemVersion 探测一个系统命令的版本；找不到给空串。
func (r *Runtime) systemVersion(name string, args ...string) string {
	exe := pkg.LookPath(name)
	// Windows 上 python 有时只装了 python3 的入口名。
	if exe == "" && name == "python" {
		exe = pkg.LookPath("python3")
	}
	if exe == "" {
		return ""
	}
	return r.Version(exe, args...)
}

// WhichPi pi 入口所在的目录；给界面「在资源管理器里定位」用。
func (r *Runtime) WhichPi() string {
	entry := r.Resolve()
	if entry.Script != "" {
		return filepath.Dir(entry.Script)
	}
	if entry.Command == "" {
		return ""
	}
	return filepath.Dir(entry.Command)
}

// ShellHint 给用户的一句安装指引。壳不代跑任何一条，只说清楚该做什么。
func ShellHint() string {
	return strings.Join([]string{
		"npm install -g --ignore-scripts @earendil-works/pi-coding-agent",
		"或按 pi 官网的一键安装脚本装好后重启本应用",
	}, "；")
}

// NodeHint 给「pi 在但 node 缺」这一种情况的指引。
// 重装 pi 修不好它：pi 的 CLI 是 JS，必须有一个 node 才跑得起来。
func NodeHint() string {
	return strings.Join([]string{
		"已找到 pi，但它的启动脚本需要 node 才跑得起来",
		"请从 nodejs.org 装一个 Node.js（LTS 即可），装完重启本应用",
	}, "；")
}
