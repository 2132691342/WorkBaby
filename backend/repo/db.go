// Package repo 是持久化层：JSON 设置台账 + SQLite 派生索引。
// 它只依赖 pkg 与 domain，不感知业务编排。
package repo

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// DB 是 SQLite 连接。纯 Go 驱动（glebarez/sqlite），不引 cgo——
// 静态构建与交叉编译是这套交付的前提，mattn/go-sqlite3 的性能换不来这个。
type DB struct {
	// db GORM 连接
	db *gorm.DB
}

// OpenDB 打开（或创建）SQLite 数据库并完成迁移。
// 外键与 WAL 都要开：前者保证行不会指向不存在的东西，
// 后者让「读的同时后台在写索引」不互相阻塞。
func OpenDB(path string) (*DB, error) {
	if err := pkg.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "数据库目录创建失败", err)
	}
	gormDB, err := gorm.Open(sqlite.Open(path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{
		// GORM 默认会把每条 SQL 打到标准错误；发布版没有控制台，
		// 这些输出会污染用户的终端。慢查询另有日志兜底。
		Logger: logger.Discard,
	})
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "数据库打开失败", err)
	}
	db := &DB{db: gormDB}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

// GORM 暴露底层连接；只给同层的仓储实现用。
func (d *DB) GORM() *gorm.DB { return d.db }

// Close 关闭连接。
func (d *DB) Close() error {
	sqlDB, err := d.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// migrate 建表。AutoMigrate 只增不改：删列这类破坏性变更要走手写迁移。
func (d *DB) migrate() error {
	models := []any{
		&domain.SessionIndexDO{},
		&domain.UsageRecordDO{},
		&domain.ToolCallRecordDO{},
		&domain.ProviderDO{},
		&domain.ProviderModelDO{},
		&domain.SearchDocDO{},
		&domain.PlanStateDO{},
	}
	if err := d.db.AutoMigrate(models...); err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "数据库迁移失败", err)
	}
	return nil
}

// SessionIndexDO 是会话索引的持久化行映射。
type SessionIndexDO = domain.SessionIndexDO

// UsageRecordDO 是用量记录的持久化行映射。
type UsageRecordDO = domain.UsageRecordDO

// ToolCallRecordDO 是工具调用记录的持久化行映射。
type ToolCallRecordDO = domain.ToolCallRecordDO

// ProviderDO 是服务商的持久化行映射。
type ProviderDO = domain.ProviderDO

// ProviderModelDO 是模型的持久化行映射。
type ProviderModelDO = domain.ProviderModelDO

// SearchDocDO 是检索文档的持久化行映射。
type SearchDocDO = domain.SearchDocDO

// PlanStateDO 是计划状态的持久化行映射。
type PlanStateDO = domain.PlanStateDO
