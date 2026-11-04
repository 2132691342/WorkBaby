package engine

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// provisionItem 是一个随包分发的运行时压缩包。
type provisionItem struct {
	// ZipName 资产目录下的文件名
	ZipName string
	// Target 解压目标目录
	Target string
	// Label 界面上的名字
	Label string
}

// ProvisionRuntime 检查三个内置运行时是否齐备，缺哪个解哪个。
// 进度经 onProgress 上报；单个失败不影响其余，返回值是失败原因汇总。
func (r *Runtime) ProvisionRuntime(onProgress func(phase, message string)) []string {
	items := []provisionItem{
		{ZipName: "node-v24.21.0-win-x64.zip", Target: r.Layout.NodeDir, Label: "Node.js"},
		{ZipName: "PortableGit.zip", Target: r.Layout.GitDir, Label: "PortableGit"},
		{ZipName: "Python313.zip", Target: r.Layout.PythonDir, Label: "Python"},
	}
	assets := filepath.Join(r.Layout.AssetsDir, "runtime")
	var problems []string
	for _, item := range items {
		if pkg.Exists(item.Target) {
			continue
		}
		zipPath := filepath.Join(assets, item.ZipName)
		if !pkg.Exists(zipPath) {
			// 资产缺失不是致命错误：界面上那一行会显示「未就绪」，
			// 启动不该因为少一个可选运行时整体失败。
			problems = append(problems, "运行时压缩包缺失："+item.ZipName)
			pkg.Warn("运行时压缩包缺失：%s", zipPath)
			continue
		}
		onProgress("start", "正在准备内置 "+item.Label)
		if err := pkg.EnsureDir(r.Layout.RuntimeDir); err == nil {
			if err := pkg.UnpackZip(zipPath, item.Target); err != nil {
				problems = append(problems, "内置"+item.Label+"解压失败："+pkg.As(err).Message)
				pkg.Warn("运行时解压失败 %s：%v", item.Label, err)
				onProgress("fail", "内置 "+item.Label+" 解压失败")
				continue
			}
		}
		pkg.Info("运行时解压完成：%s", item.Label)
		onProgress("done", "内置 "+item.Label+" 就绪")
	}
	return problems
}

// NodeVersion 内置 node 版本；没就绪给空串。
func (r *Runtime) NodeVersion() string {
	if !r.Layout.NodeReady() {
		return ""
	}
	return r.Version(r.Layout.NodeExe(), "--version")
}

// GitVersion 内置 git 版本。
func (r *Runtime) GitVersion() string {
	if !r.Layout.GitReady() {
		return ""
	}
	return r.Version(r.Layout.GitExe(), "--version")
}

// PythonVersion 内置 python 版本。
func (r *Runtime) PythonVersion() string {
	if !r.Layout.PythonReady() {
		return ""
	}
	return r.Version(r.Layout.PythonExe(), "--version")
}

// PiVersion 读 pi 的 package.json 拿版本；没装给空串。
func (r *Runtime) PiVersion() string {
	var meta struct {
		Version string `json:"version"`
	}
	path := filepath.Join(r.Layout.PiPkgDir(), "package.json")
	if pkg.ReadJSON(path, &meta) == nil {
		return ""
	}
	return meta.Version
}

// PiInstall 用内置 npm 装 pi 本体到 data\pi-app。
func (r *Runtime) PiInstall(onLine func(line string)) (*pkg.ProcResult, error) {
	res, err := r.NpmInstall(r.Layout.PiAppDir, "@earendil-works/pi-coding-agent@latest", onLine)
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return res, pkg.Err(pkg.CodeProvision, "pi 安装失败", truncateDetail(res.Stderr, res.Stdout))
	}
	if !r.Layout.PiInstalled() {
		return res, pkg.Err(pkg.CodeProvision, "安装完成但找不到 pi 入口", r.Layout.PiEntry())
	}
	return res, nil
}

// PiUninstall 删掉 pi 包目录（外壳卸载入口）。
func (r *Runtime) PiUninstall() error {
	if err := os.RemoveAll(r.Layout.PiPkgDir()); err != nil {
		return pkg.WrapErr(pkg.CodeProvision, "pi 卸载失败", err)
	}
	return nil
}

// truncateDetail 把 stderr 优先、stdout 兜底的原文截到可读长度。
func truncateDetail(stderr, stdout string) string {
	text := stderr
	if text == "" {
		text = stdout
	}
	return pkg.Truncate(text, 2000)
}
