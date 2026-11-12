package pkg

import (
	"os"
	"path/filepath"
	"strings"
)

// Layout 是本应用的目录布局：装配时解析一次，之后全项目只读。
// 只分两块地盘——壳自己的数据目录，和用户已有的 pi 家目录。壳不复制 pi 的家，
// 那个目录是用户与命令行共享的唯一真相。
type Layout struct {
	// DataDir 壳的数据根：%LOCALAPPDATA%\WorkBaby（开发态带 -dev 后缀）
	DataDir string
	// PiAgentDir 用户 pi 的家：%USERPROFILE%\.pi\agent，与管理页看到的完全同一份
	PiAgentDir string
	// LogDir 壳的日志目录：data\logs
	LogDir string
	// CacheDir 壳自己的缓存根：data\cache
	CacheDir string
	// StorePath 壳的存储文件：data\workbaby.json
	StorePath string
	// AssetsDir 随包分发的资产（内置技能包）
	AssetsDir string
	// Dev 开发态标记：数据目录加后缀，避免与安装态互相污染
	Dev bool
}

// ResolveLayout 解析布局。exeDir 是 exe 所在目录（开发态为仓库根）。
// 资产的定位不在这里做：安装态的资产在 exe 同级，开发态在仓库根，由调用方判定。
func ResolveLayout(exeDir string, assetsDir string, dev bool) Layout {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = exeDir
	}
	name := "WorkBaby"
	if dev {
		name = "WorkBaby-dev"
	}
	data := filepath.Join(local, name)
	return Layout{
		DataDir:    data,
		PiAgentDir: ResolvePiAgentDir(),
		LogDir:     filepath.Join(data, "logs"),
		CacheDir:   filepath.Join(data, "cache"),
		StorePath:  filepath.Join(data, "workbaby.json"),
		AssetsDir:  assetsDir,
		Dev:        dev,
	}
}

// ResolvePiAgentDir 用户 pi 的家目录。
// 先认 PI_CODING_AGENT_DIR——用户可能自己重定向过；否则取 ~/.pi/agent，
// 与 pi 的 getAgentDir() 同一口径。
func ResolvePiAgentDir() string {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".pi", "agent")
	}
	return filepath.Join(home, ".pi", "agent")
}

// SessionsDir 会话 JSONL 根。
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

// PiPresent pi 的家是否已存在（用户装过 pi 的最直接证据）。
func (l Layout) PiPresent() bool { return IsDir(l.PiAgentDir) }
