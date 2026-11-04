package pkg

import (
	"os"
	"path/filepath"
)

// Layout 是沙箱的目录布局：装配时解析一次，之后全项目只读。
type Layout struct {
	// DataDir 数据根：%LOCALAPPDATA%\WorkBaby（开发态带 -dev 后缀）
	DataDir string
	// RuntimeDir 内置运行时解压根：data\runtime
	RuntimeDir string
	// NodeDir 内置 Node.js：runtime\node
	NodeDir string
	// GitDir 内置 PortableGit：runtime\PortableGit
	GitDir string
	// PythonDir 内置 Python：runtime\python
	PythonDir string
	// PiAppDir pi 本体（内置 npm --prefix 安装产物）：data\pi-app
	PiAppDir string
	// PiAgentDir pi 的家（PI_CODING_AGENT_DIR）：data\pi
	PiAgentDir string
	// CacheDir npm 缓存与全局前缀的根：data\cache
	CacheDir string
	// NpmCacheDir npm 缓存：cache\npm
	NpmCacheDir string
	// NpmGlobalDir npm 全局前缀：cache\npm-global
	NpmGlobalDir string
	// LogDir 按天滚动的日志目录：data\logs
	LogDir string
	// DbPath 设置库：data\workbaby.db
	DbPath string
	// AssetsDir 随包分发的资产（运行时 zip / 内置技能 / 帮助文档）
	AssetsDir string
	// Dev 开发态标记：数据目录加后缀，避免与安装态互相污染
	Dev bool
}

// ResolveLayout 解析布局。exeDir 是 exe 所在目录（开发态为仓库根）。
func ResolveLayout(exeDir string, dev bool) Layout {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = exeDir
	}
	name := "WorkBaby"
	if dev {
		name = "WorkBaby-dev"
	}
	data := filepath.Join(local, name)
	// 开发态的资产在仓库根；打包态的资产在 exe 同级 assets。
	assets := filepath.Join(exeDir, "assets")
	if dev {
		if probe := filepath.Join(exeDir, "assets"); IsDir(filepath.Join(probe, "runtime")) {
			assets = probe
		}
	}
	return Layout{
		DataDir:      data,
		RuntimeDir:   filepath.Join(data, "runtime"),
		NodeDir:      filepath.Join(data, "runtime", "node"),
		GitDir:       filepath.Join(data, "runtime", "PortableGit"),
		PythonDir:    filepath.Join(data, "runtime", "python"),
		PiAppDir:     filepath.Join(data, "pi-app"),
		PiAgentDir:   filepath.Join(data, "pi"),
		CacheDir:     filepath.Join(data, "cache"),
		NpmCacheDir:  filepath.Join(data, "cache", "npm"),
		NpmGlobalDir: filepath.Join(data, "cache", "npm-global"),
		LogDir:       filepath.Join(data, "logs"),
		DbPath:       filepath.Join(data, "workbaby.db"),
		AssetsDir:    assets,
		Dev:          dev,
	}
}

// SessionsDir 会话 JSONL 根：pi 的家下面。
func (l Layout) SessionsDir() string { return filepath.Join(l.PiAgentDir, "sessions") }

// SkillsDir 启用态技能区。
func (l Layout) SkillsDir() string { return filepath.Join(l.PiAgentDir, "skills") }

// SkillsOffDir 停用态技能区：目录搬进来 pi 自然不再发现。
func (l Layout) SkillsOffDir() string { return filepath.Join(l.PiAgentDir, "skills_disabled") }

// PluginRoot 插件安装根：pi 自己的 npm 约定。
func (l Layout) PluginRoot() string { return filepath.Join(l.PiAgentDir, "npm") }

// SettingsFile pi 的 settings.json。
func (l Layout) SettingsFile() string { return filepath.Join(l.PiAgentDir, "settings.json") }

// ModelsFile pi 的 models.json。
func (l Layout) ModelsFile() string { return filepath.Join(l.PiAgentDir, "models.json") }

// AuthFile pi 的 auth.json。
func (l Layout) AuthFile() string { return filepath.Join(l.PiAgentDir, "auth.json") }

// McpFile 用户级 mcp.json。
func (l Layout) McpFile() string { return filepath.Join(l.PiAgentDir, "mcp.json") }

// PiPkgDir pi 包的安装目录。
func (l Layout) PiPkgDir() string {
	return filepath.Join(l.PiAppDir, "node_modules", "@earendil-works", "pi-coding-agent")
}

// PiEntry pi 的 CLI 入口。
func (l Layout) PiEntry() string {
	return filepath.Join(l.PiPkgDir(), "dist", "bundle", "cli.js")
}

// NodeExe 内置 node.exe（不判断存在性，调用方用 Sandbox 的探测版）。
func (l Layout) NodeExe() string { return filepath.Join(l.NodeDir, "node.exe") }

// NpmCli 内置 npm-cli.js。
func (l Layout) NpmCli() string {
	return filepath.Join(l.NodeDir, "node_modules", "npm", "bin", "npm-cli.js")
}

// GitExe 内置 git.exe。
func (l Layout) GitExe() string { return filepath.Join(l.GitDir, "cmd", "git.exe") }

// BashExe 内置 bash.exe：agent 主力 shell。
func (l Layout) BashExe() string { return filepath.Join(l.GitDir, "bin", "bash.exe") }

// PythonExe 内置 python.exe。
func (l Layout) PythonExe() string { return filepath.Join(l.PythonDir, "python.exe") }

// NodeReady 内置 node 是否就绪（不抛的探测版，给状态行用）。
func (l Layout) NodeReady() bool { return Exists(l.NodeExe()) }

// GitReady 内置 git 是否就绪。
func (l Layout) GitReady() bool { return Exists(l.GitExe()) }

// PythonReady 内置 python 是否就绪。
func (l Layout) PythonReady() bool { return Exists(l.PythonExe()) }

// PiInstalled pi 本体是否已装。
func (l Layout) PiInstalled() bool { return Exists(l.PiEntry()) }
