// Package engine 管引擎入口：找到用户已经装好的 pi，读它的版本，按它的 CLI 干活。
// 它不下载、不安装、不接管 pi 的家——引擎是用户自己的，壳只是操作它的界面。
package engine

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// Runtime 是引擎入口门面：pi 在哪、什么版本、怎么调它的 CLI。
type Runtime struct {
	// Layout 目录布局
	Layout pkg.Layout
	// manualPath 用户在设置里指定的 pi 路径；空串表示没指定
	manualPath func() string
}

// New 造一个引擎入口门面。manualPath 由装配根注入，engine 不认识存储层。
func New(layout pkg.Layout, manualPath func() string) *Runtime {
	if manualPath == nil {
		manualPath = func() string { return "" }
	}
	return &Runtime{Layout: layout, manualPath: manualPath}
}

// Entry 是探测出来的 pi 入口。
type Entry struct {
	// Command 可直接执行的入口路径（.cmd / .exe），或者解释器（node）
	Command string
	// Script 需要解释器执行的 CLI 脚本；非空时 Command 是 node
	Script string
	// Source 发现方式：manual / path / npm-global / none
	Source string
}

// ManualPath 用户在设置里填的 pi 路径；没填给空串。
func (r *Runtime) ManualPath() string { return strings.TrimSpace(r.manualPath()) }

// Resolve 按优先级定位 pi 入口：设置里的手动路径 → PATH → npm 全局根。
// 三处都没有就给空 Entry，由界面渲染「未检测到 pi」的引导。
func (r *Runtime) Resolve() Entry {
	if entry := r.resolveManual(); entry.Command != "" {
		return entry
	}
	if entry := resolveOnPath(); entry.Command != "" {
		return entry
	}
	if root := npmGlobalRoot(); root != "" {
		if entry := entryFromDir(filepath.Join(root, "@earendil-works", "pi-coding-agent")); entry.Command != "" {
			entry.Source = "npm-global"
			return entry
		}
	}
	return Entry{Source: "none"}
}

// resolveManual 用手动路径。填目录时按里面的 pi 入口补全，填文件时直接用。
// 手动路径无效就返回空，让后面的探测接手——用户填错了不该让整个引擎不可用。
func (r *Runtime) resolveManual() Entry {
	raw := r.ManualPath()
	if raw == "" {
		return Entry{}
	}
	if pkg.IsDir(raw) {
		entry := entryFromDir(raw)
		entry.Source = "manual"
		return entry
	}
	if !pkg.Exists(raw) {
		return Entry{}
	}
	entry := entryOfFile(raw)
	entry.Source = "manual"
	return entry
}

// resolveOnPath 用 PATH 上的 pi。npm 全局装的 pi 是 .cmd 垫片，直接跑即可。
func resolveOnPath() Entry {
	if hit := pkg.LookPath("pi"); hit != "" {
		entry := entryOfFile(hit)
		entry.Source = "path"
		return entry
	}
	return Entry{}
}

// entryFromDir 从一个包目录里找入口：CLI 脚本配 PATH 上的 node，或包内自带的垫片。
func entryFromDir(dir string) Entry {
	script := filepath.Join(dir, "dist", "bundle", "cli.js")
	if pkg.Exists(script) {
		if nodeExe := pkg.LookPath("node"); nodeExe != "" {
			return Entry{Command: nodeExe, Script: script}
		}
	}
	for _, name := range []string{"pi.cmd", "pi.exe", "pi"} {
		if candidate := filepath.Join(dir, name); pkg.Exists(candidate) {
			return entryOfFile(candidate)
		}
	}
	return Entry{}
}

// entryOfFile 把用户给的路径判成「直接跑」还是「交给 node 跑」。
func entryOfFile(path string) Entry {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".mjs", ".cjs":
		// 命中 CLI 脚本：必须有 node 才跑得起来，找不到就留空脚本交给调用方报错。
		if nodeExe := pkg.LookPath("node"); nodeExe != "" {
			return Entry{Command: nodeExe, Script: path}
		}
		return Entry{}
	}
	return Entry{Command: path}
}

// Exe 界面上展示的 pi 入口路径。
// 脚本形态下真正执行的是 node，但界面该给人看的是 pi 自己的位置，
// 否则用户会以为「我的 pi 是一个 node.exe」。
func (r *Runtime) Exe() string {
	entry := r.Resolve()
	if entry.Script != "" {
		return entry.Script
	}
	return entry.Command
}

// Script 入口脚本路径；直接可执行的形态给空串。
func (r *Runtime) Script() string { return r.Resolve().Script }

// Installed pi 是否可用。
func (r *Runtime) Installed() bool { return r.Resolve().Command != "" }

// PiVersion 读 pi 的版本。
// 先读 package.json（毫秒级、无进程开销），读不到再问 CLI。
func (r *Runtime) PiVersion() string {
	entry := r.Resolve()
	if entry.Script != "" {
		// 脚本在 <pkg>/dist/bundle/cli.js：往上三级就是包根。
		if version := versionOfPackage(filepath.Dir(filepath.Dir(filepath.Dir(entry.Script)))); version != "" {
			return version
		}
	}
	if entry.Command == "" {
		return ""
	}
	return r.Version(entry.Command, append(entryArgs(entry), "--version")...)
}

// entryArgs 入口的固定参数：脚本形态要在命令行前面先给脚本路径。
func entryArgs(entry Entry) []string {
	if entry.Script == "" {
		return nil
	}
	return []string{entry.Script}
}

// args 组装一次 CLI 调用：脚本形态补上脚本路径，之后才是子命令。
func (e Entry) args(rest ...string) []string {
	return append(entryArgs(e), rest...)
}

// RunCLI 跑一条 pi 子命令；逐行经 onLine 上报进度。
// 插件装包走网络，超时给到十分钟；超时必杀整棵进程树。
func (r *Runtime) RunCLI(rest []string, onLine func(line string)) (*pkg.ProcResult, error) {
	entry := r.Resolve()
	if entry.Command == "" {
		return nil, pkg.Err(pkg.CodeEngineMissing, "没有检测到 pi，请在应用页指定 pi 的位置")
	}
	return pkg.RunProc(entry.Command, entry.args(rest...), pkg.ProcOptions{
		// 工作目录给数据目录：pi 的包命令不依赖 cwd，给一个一定存在的目录即可。
		Dir:       r.Layout.DataDir,
		Env:       r.Layout.ChildEnv(nil),
		TimeoutMs: 10 * 60_000,
		OnLine:    onLine,
	})
}

// Version 跑一条 `--version` 拿版本串；失败给空串（探测不该抛）。
func (r *Runtime) Version(exe string, args ...string) string {
	if exe == "" || !pkg.Exists(exe) {
		return ""
	}
	res, err := pkg.RunProc(exe, args, pkg.ProcOptions{TimeoutMs: 20_000, Env: r.Layout.ChildEnv(nil)})
	if err != nil || res == nil {
		return ""
	}
	// 有的工具把版本打到 stderr：两处都看一眼。
	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		out = strings.TrimSpace(res.Stderr)
	}
	return firstLine(out)
}

// firstLine 取第一行：版本命令有时会多带一行提示。
func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}

// versionOfPackage 读一个包目录下 package.json 的 version；读不到给空串。
func versionOfPackage(dir string) string {
	var meta struct {
		Version string `json:"version"`
	}
	if pkg.ReadJSON(filepath.Join(dir, "package.json"), &meta) != nil {
		return ""
	}
	return meta.Version
}
