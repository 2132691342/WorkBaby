// Package runtime 提供运行时的横切设施：数据目录解析、路径常量。
// 不依赖 Wails runtime；可纯单测。
package runtime

import (
	"os"
	"path/filepath"

	"WorkBaby/internal/pkg"
)

// Paths 持有 WorkBaby 本机数据目录；所有持久化的根路径都从它派生。
type Paths struct {
	Home string // %APPDATA%/WorkBaby/
	DB   string // Home/db/workbaby.db
	Log  string // Home/logs/
	Cfg  string // Home/config.yaml
}

// Resolve 根据用户配置/环境变量解析数据目录；优先环境变量 WORKBABY_HOME，便携模式下走 .portable 标记。
// 当前仅 Windows，按 %APPDATA% 解析。
func Resolve() (*Paths, error) {
	home := os.Getenv("WORKBABY_HOME")
	if home == "" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData, _ = os.UserHomeDir()
		}
		if appData == "" {
			return nil, pkg.New(2001, "cannot resolve user data dir", "")
		}
		home = filepath.Join(appData, "WorkBaby")
	}
	if err := pkg.EnsureDir(home); err != nil {
		return nil, err
	}
	dbDir := filepath.Join(home, "db")
	if err := pkg.EnsureDir(dbDir); err != nil {
		return nil, err
	}
	logDir := filepath.Join(home, "logs")
	if err := pkg.EnsureDir(logDir); err != nil {
		return nil, err
	}
	return &Paths{
		Home: home,
		DB:   filepath.Join(dbDir, "workbaby.db"),
		Log:  logDir,
		Cfg:  filepath.Join(home, "config.yaml"),
	}, nil
}
