// Package repo 是 GORM 持久层：只做读写与必要的原子更新，不含业务规则。
package repo

import (
	"errors"

	"gorm.io/gorm"
)

// Repo 持有唯一数据库连接；所有仓储方法挂在它上面，避免各处自行打开连接。
type Repo struct {
	db *gorm.DB
}

// notFound 统一「未命中」判据，避免各仓储文件重复判断 gorm 错误。
func notFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB 暴露底层连接，仅供 FTS 虚表查询等必须走 raw SQL 的场景。
func (r *Repo) DB() *gorm.DB { return r.db }

// Close 关闭底层连接池。SQLite 文件被占用时数据目录无法删除、
// 备份也无法复制，退出时必须显式关掉。
func (r *Repo) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	pool, err := r.db.DB()
	if err != nil {
		return err
	}
	return pool.Close()
}
