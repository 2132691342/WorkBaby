// Package repo 是 GORM 持久层：只做读写与必要的原子更新，不含业务规则。
package repo

import (
	"errors"

	"WorkBaby/backend/pkg"

	"gorm.io/gorm"
)

// Repo 持有唯一数据库连接；所有仓储方法挂在它上面，避免各处自行打开连接。
type Repo struct {
	db *gorm.DB
}

// wrapDB 给底层 DB 错误补上 2xxx 段错误码：repo 出口不带码，到前端就只剩
// 兜底的 9999，前端失去按段位分流的能力。op 用「动词 + 对象」，日志能直接定位。
func wrapDB(op string, err error) error {
	if err == nil {
		return nil
	}
	return pkg.Wrap(2105, op+"失败", err)
}

// notFound 统一「未命中」判据，避免各仓储文件重复判断 gorm 错误。
func notFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// New 构造仓储。
func New(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB 暴露底层连接，仅供 FTS 虚表查询等必须走 raw SQL 的场景。
func (r *Repo) DB() *gorm.DB { return r.db }

// WithTx 在一个事务里跑 fn：多步写要么一起生效要么一起回滚。
// 连接固定为一条，fn 里不要做长 IO（网络请求、解压）——那会把写锁拖成整库停顿。
func (r *Repo) WithTx(fn func(tx *Repo) error) error {
	if r == nil || r.db == nil {
		return pkg.New(2105, "数据仓储未就绪", "")
	}
	return wrapDB("事务", r.db.Transaction(func(tx *gorm.DB) error { return fn(&Repo{db: tx}) }))
}

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
