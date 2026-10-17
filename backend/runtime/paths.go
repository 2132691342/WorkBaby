// Package runtime 负责数据目录定位与内置 Python 运行时解压。
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
	PythonDir  string
	// NodeDir 与 NpmGlobalDir 是跑 PI 的那套：Node 必须内置优先，
	// npm 全局前缀必须落在应用目录（系统前缀要管理员权限，写失败时用户完全看不懂）。
	NodeDir      string
	NpmGlobalDir string
	PiAgentDir   string
	SkillsDir    string
	// DisabledSkillsDir 不是技能目录：PI 只扫 skills/，停用的技能住在这里——
	// 对模型不存在，对界面仍要可见可恢复。
	DisabledSkillsDir string
	ConfigFile        string
	BackgroundDir     string
	// SitePackages 是给内置 Python 装第三方包的地方。落在数据根而不是
	// runtime/python 里：后者升级时会整个清场，装过的包跟着一起没。
	SitePackages string
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
	p := Paths{
		DataDir:      root,
		DBPath:       filepath.Join(root, "workbaby.db"),
		LogDir:       filepath.Join(root, "logs"),
		TmpDir:       filepath.Join(root, "tmp"),
		RuntimeDir:   filepath.Join(root, "runtime"),
		PythonDir:    filepath.Join(root, "runtime", "python"),
		NodeDir:      filepath.Join(root, "runtime", "node"),
		NpmGlobalDir: filepath.Join(root, "runtime", "npm-global"),
		// PiAgentDir 是 PI 的 agent 目录：WorkBaby 显式指定它，把 PI 的状态收进
		// 应用目录，卸载即清；同时保证它的 skills/ 就是我们的技能目录。
		PiAgentDir: filepath.Join(root, "pi-agent"),
		SkillsDir:  filepath.Join(root, "pi-agent", "skills"),
		// 停用的技能移出 PI 的扫描范围才算真停用；同层目录，不嵌套进 skills/。
		DisabledSkillsDir: filepath.Join(root, "pi-agent", "skills.disabled"),
		ConfigFile:        filepath.Join(root, "config.yaml"),
		BackgroundDir:     filepath.Join(root, "background"),
		SitePackages:      filepath.Join(root, "site-packages"),
	}
	for _, dir := range []string{p.DataDir, p.LogDir, p.TmpDir, p.RuntimeDir, p.SkillsDir, p.DisabledSkillsDir, p.BackgroundDir, p.SitePackages, p.NodeDir, p.NpmGlobalDir, p.PiAgentDir} {
		if err := pkg.EnsureDir(dir); err != nil {
			return Paths{}, err
		}
	}
	sweepTmp(p.TmpDir)
	return p, nil
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
