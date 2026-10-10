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
	DataDir          string
	DBPath           string
	LogDir           string
	TmpDir           string
	RuntimeDir       string
	PythonDir        string
	PowerShellDir    string
	SkillsDir        string
	BuiltinSkillsDir string
	ConfigFile       string
	BackgroundDir    string
}

// Resolve 定位数据根并展开全部子路径。WORKBABY_HOME 优先，便于便携版与测试隔离。
func Resolve() (Paths, error) {
	root := os.Getenv("WORKBABY_HOME")
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, pkg.Wrap(1001, "定位用户数据目录失败", err)
		}
		root = filepath.Join(base, "WorkBaby")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, pkg.Wrap(1001, "解析数据目录失败", err)
	}
	p := Paths{
		DataDir:       root,
		DBPath:        filepath.Join(root, "workbaby.db"),
		LogDir:        filepath.Join(root, "logs"),
		TmpDir:        filepath.Join(root, "tmp"),
		RuntimeDir:    filepath.Join(root, "runtime"),
		PythonDir:     filepath.Join(root, "runtime", "python"),
		PowerShellDir: filepath.Join(root, "runtime", "powershell"),
		SkillsDir:     filepath.Join(root, "skills"),
		ConfigFile:    filepath.Join(root, "config.yaml"),
		// 内置技能要落到真实磁盘路径：系统提示里让模型 read 的 SKILL.md 必须存在。
		// 单独一个目录而不是 skills/builtin，是为了不被「全局技能」扫描重复登记。
		BuiltinSkillsDir: filepath.Join(root, "builtin-skills"),
		BackgroundDir:    filepath.Join(root, "background"),
	}
	for _, dir := range []string{p.DataDir, p.LogDir, p.TmpDir, p.RuntimeDir, p.SkillsDir, p.BuiltinSkillsDir, p.BackgroundDir} {
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
