// Package db 负责打开 SQLite、执行迁移与创建 FTS5 虚表。
package db

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/glebarez/sqlite"
)

// 连接期 PRAGMA：WAL 提升读写并发，busy_timeout 规避瞬时写锁失败。
const pragmas = `PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;`

// FTS5 必须用 trigram：默认 unicode61 不按字切分 CJK，整句中文会退化成单个 token。
// 拆成多条语句逐条执行：一次 Exec 多条在建表场景下不可靠。
var ftsStatements = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_chunks_fts
USING fts5(content, doc_id UNINDEXED, chunk_id UNINDEXED, tokenize='trigram')`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_chunks_ai AFTER INSERT ON knowledge_chunks BEGIN
  INSERT INTO knowledge_chunks_fts(content, doc_id, chunk_id) VALUES (new.content, new.doc_id, new.id);
END`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_chunks_ad AFTER DELETE ON knowledge_chunks BEGIN
  DELETE FROM knowledge_chunks_fts WHERE chunk_id = old.id;
END`,
	`CREATE TRIGGER IF NOT EXISTS knowledge_chunks_au AFTER UPDATE ON knowledge_chunks BEGIN
  UPDATE knowledge_chunks_fts SET content = new.content WHERE chunk_id = new.id;
END`,
}

// Open 打开（或创建）数据库并完成迁移。连接数固定为 1：单连接 + WAL 让写行为完全可预测。
func Open(path string) (*gorm.DB, error) {
	if err := pkg.EnsureDir(dirOf(path)); err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, pkg.Wrap(2101, "打开数据库失败", err)
	}
	pool, err := gdb.DB()
	if err != nil {
		return nil, pkg.Wrap(2101, "获取数据库连接失败", err)
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)

	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		// 「数据库被占用」几乎总是另一个 WorkBaby 进程还活着。这个错误必须说人话，
		// 否则用户只会看到空窗口。
		if isLocked(err) {
			return nil, pkg.Wrap(2101, "数据库被占用，另一个 WorkBaby 还在运行，请先在任务管理器里结束它", err)
		}
		return nil, pkg.Wrap(2101, "打开数据库失败", err)
	}
	if _, err := pool.Exec(pragmas); err != nil {
		return nil, pkg.Wrap(2102, "设置数据库参数失败", err)
	}
	if err := Migrate(gdb); err != nil {
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

// Migrate 执行表迁移与 FTS 建表。只增不删，零值兜底。
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(
		&domain.SessionDO{},
		&domain.EntryDO{},
		&domain.ApprovalDO{},
		&domain.ProviderDO{},
		&domain.ModelConfigDO{},
		&domain.KnowledgeDocDO{},
		&domain.KnowledgeChunkDO{},
		&domain.SettingDO{},
		&domain.TokenUsageDO{},
	); err != nil {
		return pkg.Wrap(2103, "数据库迁移失败", err)
	}
	for _, stmt := range ftsStatements {
		if err := gdb.Exec(stmt).Error; err != nil {
			return pkg.Wrap(2104, "创建全文索引失败", err)
		}
	}
	return nil
}

// dirOf 取数据库文件所在目录，供建目录用。
func dirOf(path string) string { return filepath.Dir(path) }
