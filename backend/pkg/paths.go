package pkg

import (
	"os"
	"path/filepath"
)

// Layout 是本应用的目录布局：装配时解析一次，之后全项目只读。
// 所有真相都在自己的数据目录里——引擎在本进程内实现，
// 配置、会话、技能都归本应用自己管，不再依赖别人的目录。
type Layout struct {
	// DataDir 数据根：%LOCALAPPDATA%\WorkBaby（开发态带 -dev 后缀）
	DataDir string
	// LogDir 日志目录：data\logs
	LogDir string
	// CacheDir 缓存根：data\cache
	CacheDir string
	// SessionsDir 会话目录：data\sessions
	SessionsDir string
	// SkillsDir 启用态技能区：data\skills
	SkillsDir string
	// SkillsOffDir 停用态技能区：data\skills_disabled
	SkillsOffDir string
	// TemplatesDir 提示词模板区：data\templates
	TemplatesDir string
	// StorePath 设置文件：data\workbaby.json
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
		DataDir:      data,
		LogDir:       filepath.Join(data, "logs"),
		CacheDir:     filepath.Join(data, "cache"),
		SessionsDir:  filepath.Join(data, "sessions"),
		SkillsDir:    filepath.Join(data, "skills"),
		SkillsOffDir: filepath.Join(data, "skills_disabled"),
		TemplatesDir: filepath.Join(data, "templates"),
		StorePath:    filepath.Join(data, "workbaby.json"),
		AssetsDir:    assetsDir,
		Dev:          dev,
	}
}

// McpFile MCP 服务清单。
func (l Layout) McpFile() string { return filepath.Join(l.DataDir, "mcp.json") }

// DatabasePath SQLite 数据库文件。
// 索引、统计与模型目录落在这里；会话正文仍在 JSONL 文件里。
func (l Layout) DatabasePath() string { return filepath.Join(l.DataDir, "workbaby.db") }

// ChildEnv 是给子进程的环境：只追加必须项，不遮蔽 HOME、不改 PATH 顺序——
// 工具跑的是用户自己的环境，它该看见真实的 shell 与工具链。
func (l Layout) ChildEnv(extra map[string]string) []string {
	env := os.Environ()
	env = append(env, "NO_COLOR=1", "GIT_TERMINAL_PROMPT=0")
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}
