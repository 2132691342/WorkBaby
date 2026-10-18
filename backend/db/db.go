// Package db 负责打开 SQLite 与执行迁移。
// 会话、消息与用量都在 PI 的会话文件里，这里只剩 WorkBaby 自己的界面设置。
package db

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 连接期 PRAGMA：WAL 提升读写并发，busy_timeout 规避瞬时写锁失败。
const pragmas = `PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;`

// Open 打开（或创建）数据库并完成迁移。连接数固定为 1：单连接 + WAL 让写行为完全可预测。
func Open(path string) (*gorm.DB, error) {
	if err := pkg.EnsureDir(dirOf(path)); err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, pkg.Wrap("2101", "打开数据库失败", err)
	}
	pool, err := gdb.DB()
	if err != nil {
		return nil, pkg.Wrap("2101", "获取数据库连接失败", err)
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)

	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		// 「数据库被占用」几乎总是另一个 WorkBaby 进程还活着。这个错误必须说人话。
		if isLocked(err) {
			return nil, pkg.Wrap("2101", "数据库被占用，另一个 WorkBaby 还在运行，请先在任务管理器里结束它", err)
		}
		return nil, pkg.Wrap("2101", "打开数据库失败", err)
	}
	// 打开之后的每一步失败都要把连接还回去：启动失败路径上留一个句柄，
	// 单实例锁会误判「还有一个实例在跑」，下一次启动再也起不来。
	if _, err := pool.Exec(pragmas); err != nil {
		_ = pool.Close()
		return nil, pkg.Wrap("2102", "设置数据库参数失败", err)
	}
	if err := Migrate(gdb); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return gdb, nil
}

// isLocked 识别 SQLite 的「文件被别处锁住」。
func isLocked(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") ||
		strings.Contains(msg, "being used by another process") ||
		strings.Contains(msg, "busy")
}

// Migrate 执行表迁移。只增不删，零值兜底。
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(&domain.SettingDO{}); err != nil {
		return pkg.Wrap("2103", "数据库迁移失败", err)
	}
	return nil
}

// dirOf 取数据库文件所在目录，供建目录用。
func dirOf(path string) string { return filepath.Dir(path) }
