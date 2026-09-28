// Package runtime 负责数据目录定位与内置 Python 运行时解压。
package runtime

import (
	"os"
	"path/filepath"

	"WorkBaby/internal/pkg"
)

// Paths 是全部落盘位置的唯一解析结果。
type Paths struct {
	DataDir    string
	DBPath     string
	LogDir     string
	TmpDir     string
	RuntimeDir string
	PythonDir  string
	SkillsDir  string
	ConfigFile string
	ModelFile  string
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
		DataDir:    root,
		DBPath:     filepath.Join(root, "workbaby.db"),
		LogDir:     filepath.Join(root, "logs"),
		TmpDir:     filepath.Join(root, "tmp"),
		RuntimeDir: filepath.Join(root, "runtime"),
		PythonDir:  filepath.Join(root, "runtime", "python"),
		SkillsDir:  filepath.Join(root, "skills"),
		ConfigFile: filepath.Join(root, "config.yaml"),
		ModelFile:  filepath.Join(root, "model.json"),
	}
	for _, dir := range []string{p.DataDir, p.LogDir, p.TmpDir, p.RuntimeDir, p.SkillsDir} {
		if err := pkg.EnsureDir(dir); err != nil {
			return Paths{}, err
		}
	}
	return p, nil
}
