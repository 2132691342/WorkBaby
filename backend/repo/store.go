// Package repo 是持久化层：SQLite 连接、迁移与设置 KV。
// 它只依赖 pkg / domain，不感知业务编排。
package repo

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"WorkBaby/backend/pkg"
)

// Store 是壳自己的设置库。会话、插件、技能都不在这里——
// 它们的真相在磁盘文件与引擎里，这里只存「壳的开关台账」。
type Store struct {
	db *gorm.DB
}

// SettingDO 是设置 KV 的持久化行。
type SettingDO struct {
	// SettingKey 键
	SettingKey string `gorm:"column:setting_key;primaryKey;size:200"`
	// SettingValue 值（一律字符串，复杂结构自己 JSON）
	SettingValue string `gorm:"column:setting_value;type:text"`
	// UpdatedAt 最后写入时刻（毫秒）
	UpdatedAt int64 `gorm:"column:updated_at"`
}

// TableName 固定表名：迁移与手写 SQL 都认这一个名字。
func (SettingDO) TableName() string { return "setting" }

// SkillToggleDO 是技能开关台账。开与关的真相是技能目录在哪个区（skills /
// skills_disabled），这张表只做记账：健康面板与测试用它核对搬移是否发生过。
type SkillToggleDO struct {
	// SkillDirName 技能目录名
	SkillDirName string `gorm:"column:skill_dir_name;primaryKey;size:200"`
	// SkillEnabled 是否启用
	SkillEnabled bool `gorm:"column:skill_enabled"`
	// UpdatedAt 最后写入时刻（毫秒）
	UpdatedAt int64 `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (SkillToggleDO) TableName() string { return "skill_toggle" }

// Open 打开（必要时创建）设置库并跑迁移。
func Open(dbPath string) (*Store, error) {
	if err := pkg.EnsureDir(filepath.Dir(dbPath)); err != nil {
		return nil, pkg.WrapErr(pkg.CodeDatabase, "数据目录不可写", err)
	}
	// WAL：界面与后台任务会同时读写设置，回滚日志模式会互相阻塞。
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// 这条库里没有业务查询日志值得打；SQL 噪音会淹掉真正要紧的行。
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeDatabase, "设置库打开失败", err)
	}
	if err := db.AutoMigrate(&SettingDO{}, &SkillToggleDO{}); err != nil {
		return nil, pkg.WrapErr(pkg.CodeDatabase, "设置库迁移失败", err)
	}
	return &Store{db: db}, nil
}

// Close 关库。
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Get 读一个设置项；不存在返回空串与 false。
func (s *Store) Get(key string) (string, bool) {
	var row SettingDO
	if err := s.db.First(&row, "setting_key = ?", key).Error; err != nil {
		return "", false
	}
	return row.SettingValue, true
}

// All 读出全部设置项。
func (s *Store) All() map[string]string {
	var rows []SettingDO
	out := make(map[string]string, len(rows))
	if err := s.db.Find(&rows).Error; err != nil {
		return out
	}
	for _, row := range rows {
		out[row.SettingKey] = row.SettingValue
	}
	return out
}

// Save 批量写入设置项（整表 upsert，单事务）。
func (s *Store) Save(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	now := pkg.NowMs()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			row := SettingDO{SettingKey: k, SettingValue: v, UpdatedAt: now}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return pkg.WrapErr(pkg.CodeDatabase, "设置保存失败", err)
	}
	return nil
}

// SkillEnabled 读一个技能的开关台账；没有记录时 ok 为 false。
func (s *Store) SkillEnabled(skillDirName string) (enabled bool, ok bool) {
	var row SkillToggleDO
	if err := s.db.First(&row, "skill_dir_name = ?", skillDirName).Error; err != nil {
		return false, false
	}
	return row.SkillEnabled, true
}

// SetSkillEnabled 写一个技能的开关台账。
func (s *Store) SetSkillEnabled(skillDirName string, enabled bool) error {
	row := SkillToggleDO{SkillDirName: skillDirName, SkillEnabled: enabled, UpdatedAt: pkg.NowMs()}
	if err := s.db.Save(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeDatabase, "技能开关记账失败", err)
	}
	return nil
}
