package engine

import (
	"strings"

	"WorkBaby/backend/pkg"
)

// NpmRun 用内置 npm 跑一条命令；逐行经 onLine 上报进度给界面。
// 超时给到十分钟：装 pi 本体要下几十 MB，插件也有十几 MB。
func (r *Runtime) NpmRun(args []string, onLine func(line string)) (*pkg.ProcResult, error) {
	nodeExe, err := r.NodeExe()
	if err != nil {
		return nil, err
	}
	return pkg.RunProc(nodeExe, r.npmArgs(args...), pkg.ProcOptions{
		Dir:       r.Layout.DataDir,
		Env:       r.Layout.SandboxEnv(nil),
		TimeoutMs: 10 * 60_000,
		OnLine:    onLine,
	})
}

// NpmInstall 装一个来源到指定前缀；返回结果供调用方判断退出码。
func (r *Runtime) NpmInstall(prefix string, source string, onLine func(line string)) (*pkg.ProcResult, error) {
	return r.NpmRun([]string{
		"install", source,
		"--prefix", prefix,
		// 与 pi 自己的包安装保持一致：宿主已有的包不再拉一份副本，
		// 否则插件和引擎会各拿一个 React / zod 实例，扩展注册表对不上。
		"--legacy-peer-deps",
		"--no-audit",
		"--no-fund",
	}, onLine)
}

// Version 跑一条 `--version` 拿版本串；失败给空串（探测不该抛）。
func (r *Runtime) Version(exe string, args ...string) string {
	if exe == "" || !pkg.Exists(exe) {
		return ""
	}
	res, err := pkg.RunProc(exe, args, pkg.ProcOptions{TimeoutMs: 15_000, Env: r.Layout.SandboxEnv(nil)})
	if err != nil || res == nil {
		return ""
	}
	// python 把版本打到 stderr，node / git 打 stdout：两处都看一眼。
	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		out = strings.TrimSpace(res.Stderr)
	}
	return firstLine(out)
}

// firstLine 取第一行：python --version 有时会多带一行提示。
func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return text
}
