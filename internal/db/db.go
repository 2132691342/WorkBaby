// Package db 封装 SQLite 初始化：glebarez/sqlite (pure-Go) + GORM + WAL。
package db

import (
	"os"
	"path/filepath"
	"time"

	"WorkBaby/internal/pkg"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开 SQLite 数据库；设置 WAL + busy_timeout；启用 FTS5（v1）。
// 单一 DB 真相源；FTS5 虚拟表由 migrate.go 创建。
func Open(path string, busyMs int) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, pkg.Wrap(2010, "mkdir db dir failed", err)
		}
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(" + itoa(busyMs) + ")&_pragma=foreign_keys(ON)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		DisableForeignKeyConstraintWhenMigrating: false,
		PrepareStmt:                              true,
	})
	if err != nil {
		return nil, pkg.Wrap(2011, "open db failed", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, pkg.Wrap(2011, "raw db failed", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite 单写者；多写须走单写者 channel；本版本允许多读
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// Close 收尾关闭。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
