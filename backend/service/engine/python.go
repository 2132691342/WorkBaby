package engine

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// ensurePipEntrypoints 补齐 Python 的 Scripts 入口垫片：随包 Python 只带 pip 模块，
// 没有垫片时技能脚本里的裸 `pip install` 会掉进系统 Python——沙箱就漏了。
// 补法是让 pip 自装生成 pip.exe（批处理垫子在 Git Bash 里不可执行）；垫片已存在就跳过。
func (r *Runtime) ensurePipEntrypoints() error {
	scripts := filepath.Join(r.Layout.PythonDir, "Scripts")
	if !pkg.IsDir(scripts) {
		if err := pkg.EnsureDir(scripts); err != nil {
			return pkg.WrapErr(pkg.CodeProvision, "Python Scripts 目录创建失败", err)
		}
	}
	if pkg.Exists(filepath.Join(scripts, "pip.exe")) {
		return nil
	}
	exe, err := r.PythonExe()
	if err != nil {
		return err
	}
	res, err := pkg.RunProc(exe, []string{"-m", "pip", "install", "--force-reinstall", "--no-deps", "pip"}, pkg.ProcOptions{
		Env:       r.Layout.SandboxEnv(nil),
		TimeoutMs: 5 * 60_000,
	})
	if err != nil {
		return pkg.WrapErr(pkg.CodeProvision, "pip 入口补齐失败", err)
	}
	if res.Code != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(res.Stdout)
		}
		return pkg.Err(pkg.CodeProvision, "pip 入口补齐失败", pkg.Truncate(detail, 1000))
	}
	if !pkg.Exists(filepath.Join(scripts, "pip.exe")) {
		return pkg.Err(pkg.CodeProvision, "pip 入口补齐后仍找不到 pip.exe", scripts)
	}
	return nil
}
