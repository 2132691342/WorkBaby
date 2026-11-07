package service

import (
	"os"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/service/platform"
)

// OpenDataDir 在资源管理器里打开数据目录。
func OpenDataDir(layout pkg.Layout) error {
	if err := pkg.EnsureDir(layout.DataDir); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "数据目录不可写", err)
	}
	return platform.RevealPath(layout.DataDir)
}

// Reveal 在资源管理器里定位一个路径。
func Reveal(path string) error {
	if !pkg.Exists(path) {
		return pkg.Err(pkg.CodeFileNotFound, "路径不存在", path)
	}
	return platform.RevealPath(path)
}

// OpenPath 用系统默认程序打开一个路径。
func OpenPath(path string) error {
	if !pkg.Exists(path) {
		return pkg.Err(pkg.CodeFileNotFound, "路径不存在", path)
	}
	return platform.OpenPath(path)
}

// configFileSpec 是一个可查看的引擎配置文件。
type configFileSpec struct {
	name  string
	label string
	path  string
}

// configFiles 列出可查看的配置文件（白名单，不接受外部路径）。
// auth.json 不在列：密钥只在「密钥恒为掩码」的服务商页流转，不走原文查看。
func configFiles(layout pkg.Layout) []domain.ConfigFileVO {
	specs := []configFileSpec{
		{name: "settings", label: "settings.json", path: layout.SettingsFile()},
		{name: "models", label: "models.json", path: layout.ModelsFile()},
		{name: "mcp", label: "mcp.json", path: layout.McpFile()},
	}
	out := make([]domain.ConfigFileVO, 0, len(specs))
	for _, spec := range specs {
		row := domain.ConfigFileVO{
			ConfigName:  spec.name,
			ConfigLabel: spec.label,
			FilePath:    spec.path,
			FileExists:  pkg.Exists(spec.path),
		}
		if info, err := os.Stat(spec.path); err == nil {
			row.SizeBytes = info.Size()
		}
		out = append(out, row)
	}
	return out
}

// ConfigText 读配置文件原文；只认白名单里的名字。
func ConfigText(layout pkg.Layout, name string) (domain.ConfigTextRESP, error) {
	for _, row := range configFiles(layout) {
		if row.ConfigName != name {
			continue
		}
		out := domain.ConfigTextRESP{FilePath: row.FilePath}
		raw, err := os.ReadFile(row.FilePath)
		if err != nil {
			// 还没生成过不是错误：界面显示「尚未生成」比弹一个红框有用。
			return out, nil
		}
		out.Found = true
		const limit = 512 * 1024
		if len(raw) > limit {
			out.FileText = string(raw[:limit])
			out.Truncated = true
			return out, nil
		}
		out.FileText = string(raw)
		return out, nil
	}
	return domain.ConfigTextRESP{}, pkg.Err(pkg.CodeBadRequest, "未知的配置文件名", name)
}

// RuntimeStatus 汇总三个内置运行时与 pi 本体的就绪状态。
func RuntimeStatus(rt *engine.Runtime) domain.RuntimeStatusVO {
	layout := rt.Layout
	return domain.RuntimeStatusVO{
		NodeReady:     layout.NodeReady(),
		NodeVersion:   rt.NodeVersion(),
		GitReady:      layout.GitReady(),
		GitVersion:    rt.GitVersion(),
		PythonReady:   layout.PythonReady(),
		PythonVersion: rt.PythonVersion(),
		PiReady:       layout.PiInstalled(),
		PiVersion:     rt.PiVersion(),
	}
}

// EngineLayout 输出引擎落盘布局。
func EngineLayout(layout pkg.Layout) domain.EngineLayoutVO {
	return domain.EngineLayoutVO{
		EngineKind:   domain.EnginePi,
		RootDir:      layout.PiAppDir,
		AgentDir:     layout.PiAgentDir,
		SessionsDir:  layout.SessionsDir(),
		SkillsDir:    layout.SkillsDir(),
		PluginsDir:   layout.PluginRoot(),
		SettingsFile: layout.SettingsFile(),
		DirExists:    pkg.IsDir(layout.PiAgentDir),
	}
}
