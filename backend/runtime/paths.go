// Package runtime 负责数据目录定位与内置运行时解压。
// 数据目录是 WorkBaby 自己的（库/日志/内置运行时）；PI 的 agent 目录不在这里：
// 它必须就是用户那份 ~/.pi/agent，否则用户装好的技能、插件、模型全都看不见。
package runtime

import (
	"os"
	"path/filepath"
	"time"

	"WorkBaby/backend/pkg"
)

// Paths 是全部落盘位置的唯一解析结果。
type Paths struct {
	DataDir    string
	DBPath     string
	LogDir     string
	TmpDir     string
	RuntimeDir string
	// NodeDir / PythonDir 是内置运行时；NpmGlobalDir 是「缺 PI 时」的安装前缀。
	NodeDir      string
	PythonDir    string
	NpmGlobalDir string

	// AgentDir 是 PI 的 agent 目录：默认 ~/.pi/agent，可用 PI_CODING_AGENT_DIR 覆盖。
	// 它是唯一的配置真相源，WorkBaby 读写的就是用户 pi 命令读写的那一份。
	AgentDir  string
	SkillsDir string
	// DisabledSkillsDir 不是技能目录：PI 只扫 skills/，停用的技能住在这里——
	// 对模型不存在，对界面仍要可见可恢复。
	DisabledSkillsDir string
	SessionsDir       string
	ExtensionsDir     string

	ConfigFile    string
	BackgroundDir string
	SitePackages  string
}

// Resolve 定位数据根并展开全部子路径。WORKBABY_HOME 优先，便于便携版与测试隔离。
func Resolve() (Paths, error) {
	root := os.Getenv("WORKBABY_HOME")
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, pkg.Wrap("1001", "定位用户数据目录失败", err)
		}
		root = filepath.Join(base, "WorkBaby")
	}
	return ResolveAt(root)
}

// ResolveAt 在指定根目录下展开全部子路径；测试用它隔离到临时目录。
func ResolveAt(root string) (Paths, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, pkg.Wrap("1001", "解析数据目录失败", err)
	}
	agent := AgentDir()
	p := Paths{
		DataDir:      root,
		DBPath:       filepath.Join(root, "workbaby.db"),
		LogDir:       filepath.Join(root, "logs"),
		TmpDir:       filepath.Join(root, "tmp"),
		RuntimeDir:   filepath.Join(root, "runtime"),
		PythonDir:    filepath.Join(root, "runtime", "python"),
		NodeDir:      filepath.Join(root, "runtime", "node"),
		NpmGlobalDir: filepath.Join(root, "runtime", "npm-global"),
		AgentDir:     agent,
		SkillsDir:    filepath.Join(agent, "skills"),
		// 停用的技能移出 PI 的扫描范围才算真停用；同层目录，不嵌套进 skills/。
		DisabledSkillsDir: filepath.Join(agent, "skills.disabled"),
		SessionsDir:       filepath.Join(agent, "sessions"),
		ExtensionsDir:     filepath.Join(agent, "extensions"),
		ConfigFile:        filepath.Join(root, "config.yaml"),
		BackgroundDir:     filepath.Join(root, "background"),
		SitePackages:      filepath.Join(root, "site-packages"),
	}
	for _, dir := range []string{
		p.DataDir, p.LogDir, p.TmpDir, p.RuntimeDir,
		p.BackgroundDir, p.SitePackages, p.NodeDir, p.NpmGlobalDir,
	} {
		if err := pkg.EnsureDir(dir); err != nil {
			return Paths{}, err
		}
	}
	sweepTmp(p.TmpDir)
	return p, nil
}

// AgentDir 解析 PI 的 agent 目录：显式环境变量优先，其次 ~/.pi/agent。
// 这里只解析不创建——用户没装过 PI 时不该凭空出现一堆空目录。
func AgentDir() string {
	if v := os.Getenv("PI_CODING_AGENT_DIR"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return filepath.Clean(v)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".pi", "agent")
	}
	return filepath.Join(home, ".pi", "agent")
}

// sweepTmp 清理临时目录里超过 7 天的溢出文件（工具截断的完整输出落在这里）。
// 启动期没有任何 run 在跑，此时清理没有竞态；失败只告警，不该挡启动。
func sweepTmp(dir string) {
	cutoff := time.Now().AddDate(0, 0, -7)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			pkg.Warnf("runtime: 清理临时文件失败 %s: %v", e.Name(), err)
		}
	}
}
