// Package db 的迁移入口：GORM AutoMigrate 业务表 + 原生 SQL 建 FTS5 虚拟表。
package db

import (
	"fmt"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// Migrate 启动期调用；AutoMigrate 失败即 fail-fast（半残比不可用更危险）。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&domain.AiProviderDO{},
		&domain.ChatSessionDO{},
		&domain.MessageDO{},
		&domain.MessageBlockDO{},
		&domain.SessionTodoDO{},
		&domain.TokenUsageDO{},
		&domain.SystemSettingDO{},
		&domain.SkillDO{},
		&domain.AgentProfileDO{},
		&domain.UserCommandDO{},
		&domain.UserHookDO{},
		&domain.McpServerDO{},
		&domain.KnowledgeDocDO{},
		&domain.KnowledgeChunkDO{},
		&domain.AgentCheckpointDO{},
		&domain.RunRecordDO{},
		&domain.ApprovalRecordDO{},
		&domain.ApprovalGrantDO{},
		&domain.ChatTaskDO{},
		&domain.PetConfigDO{},
		&domain.PetSpriteDO{},
		&domain.FolderDO{},
		&domain.FileDO{},
		&domain.FileChangeDO{},
		&domain.ArtifactDO{},
		&domain.WorkspaceTrustDO{},
	); err != nil {
		return pkg.Wrap(2012, "automigrate failed", err)
	}
	return nil
}

// fts5Tables FTS5 虚拟表定义（唯一真相源；AutoMigrate 不覆盖，启动期单独执行）。tokenizer
// 必须用 trigram：unicode61 不按字切分 CJK，整句中文退化成单 token，中文检索全面失效。
// 代价是 2 字中文查询在 MATCH 上必然零命中——由调用方的带打分子串兜底承担。
var fts5Tables = []struct {
	name string
	ddl  string
}{
	// memory_fts 是 MEMORY.md 的派生索引：条目本身存在文件里，索引按条目全量重建。
	{"memory_fts", "CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(section, content, tokenize='trigram')"},
	{"knowledge_chunks_fts", "CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_chunks_fts USING fts5(id UNINDEXED, doc_id UNINDEXED, chunk_idx UNINDEXED, title, content, tokenize='trigram')"},
}

// CreateFTS5 建记忆/知识库的 FTS5 虚拟表（幂等）。
func CreateFTS5(db *gorm.DB) error {
	for _, t := range fts5Tables {
		if err := db.Exec(t.ddl).Error; err != nil {
			return pkg.Wrap(2013, "create fts5 failed", fmt.Errorf("%s: %w", t.name, err))
		}
	}
	return nil
}

// EnsureFTS5Table 单表惰性建表（repo 层首次写索引前的兜底，DDL 仍取自唯一真相源）。
func EnsureFTS5Table(db *gorm.DB, name string) error {
	for _, t := range fts5Tables {
		if t.name == name {
			return db.Exec(t.ddl).Error
		}
	}
	return pkg.New(2013, "unknown fts5 table", name)
}
