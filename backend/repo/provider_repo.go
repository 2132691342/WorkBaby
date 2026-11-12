package repo

import (
	"sort"
	"strings"

	"gorm.io/gorm"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 查询行数上限。服务商与模型的条数是用户自己填的，
// 没有上限的 Find 会把一张被灌爆的表整个读进内存。
const (
	maxProviderRows = 512
	maxModelRows    = 4096
)

// ProviderRepo 是服务商与模型的仓储。
// 凭据与服务商定义同表存放：它们同生同死，拆两张表只会让删除漏一半。
type ProviderRepo struct {
	// db 数据库连接
	db *gorm.DB
}

// NewProviderRepo 造一个服务商仓储。
func NewProviderRepo(db *DB) *ProviderRepo { return &ProviderRepo{db: db.db} }

// Upsert 保存一个服务商；有同名则更新。
func (r *ProviderRepo) Upsert(row domain.ProviderDO) error {
	if err := r.db.Save(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "服务商写入失败", err)
	}
	return nil
}

// Delete 删掉一个服务商及其全部模型。
func (r *ProviderRepo) Delete(providerName string) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_name = ?", providerName).Delete(&domain.ProviderModelDO{}).Error; err != nil {
			return err
		}
		return tx.Where("provider_name = ?", providerName).Delete(&domain.ProviderDO{}).Error
	})
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "服务商删除失败", err)
	}
	return nil
}

// Providers 列全部服务商，按排序位再按名字。
func (r *ProviderRepo) Providers() ([]domain.ProviderDO, error) {
	rows := make([]domain.ProviderDO, 0, 16)
	err := r.db.Order("provider_sort ASC, provider_name ASC").Limit(maxProviderRows).Find(&rows).Error
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "服务商查询失败", err)
	}
	return rows, nil
}

// Provider 取一个服务商。
func (r *ProviderRepo) Provider(providerName string) (domain.ProviderDO, bool, error) {
	row := domain.ProviderDO{}
	err := r.db.Where("provider_name = ?", providerName).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.ProviderDO{}, false, nil
	}
	if err != nil {
		return domain.ProviderDO{}, false, pkg.WrapErr(pkg.CodeStorage, "服务商查询失败", err)
	}
	return row, true, nil
}

// UpsertModel 保存一个模型；有同名则更新。
func (r *ProviderRepo) UpsertModel(row domain.ProviderModelDO) error {
	if err := r.db.Save(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "模型写入失败", err)
	}
	return nil
}

// DeleteModel 删掉一个模型。
// 返回是否真的删掉了：界面拿它区分「删成功」与「本来就没有这条」。
func (r *ProviderRepo) DeleteModel(providerName, modelId string) (bool, error) {
	result := r.db.Where("provider_name = ? AND model_id = ?", providerName, modelId).
		Delete(&domain.ProviderModelDO{})
	if result.Error != nil {
		return false, pkg.WrapErr(pkg.CodeStorage, "模型删除失败", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// Models 列某个服务商下的全部模型。
func (r *ProviderRepo) Models(providerName string) ([]domain.ProviderModelDO, error) {
	rows := make([]domain.ProviderModelDO, 0, 32)
	err := r.db.Where("provider_name = ?", providerName).
		Order("model_sort ASC, model_id ASC").Find(&rows).Error
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "模型查询失败", err)
	}
	return rows, nil
}

// AllModels 列全部模型。
func (r *ProviderRepo) AllModels() ([]domain.ProviderModelDO, error) {
	rows := make([]domain.ProviderModelDO, 0, 64)
	err := r.db.Order("provider_name ASC, model_sort ASC, model_id ASC").Limit(maxModelRows).Find(&rows).Error
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "模型查询失败", err)
	}
	return rows, nil
}

// FindModel 取一个模型。
func (r *ProviderRepo) FindModel(providerName, modelId string) (domain.ProviderModelDO, bool, error) {
	row := domain.ProviderModelDO{}
	err := r.db.Where("provider_name = ? AND model_id = ?", providerName, modelId).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.ProviderModelDO{}, false, nil
	}
	if err != nil {
		return domain.ProviderModelDO{}, false, pkg.WrapErr(pkg.CodeStorage, "模型查询失败", err)
	}
	return row, true, nil
}

// PlanRepo 是计划模式的持久状态仓储。
type PlanRepo struct {
	// db 数据库连接
	db *gorm.DB
}

// NewPlanRepo 造一个计划状态仓储。
func NewPlanRepo(db *DB) *PlanRepo { return &PlanRepo{db: db.db} }

// Set 写某会话的计划模式状态。
func (r *PlanRepo) Set(sessionID string, enabled bool) error {
	row := domain.PlanStateDO{
		SessionID:       sessionID,
		PlanEnabled:     enabled,
		PlanUpdatedAtMs: pkg.NowMs(),
	}
	if err := r.db.Save(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "计划状态写入失败", err)
	}
	return nil
}

// Get 读某会话的计划模式状态；没有记录时给 false。
func (r *PlanRepo) Get(sessionID string) (bool, error) {
	row := domain.PlanStateDO{}
	err := r.db.Where("session_id = ?", sessionID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, pkg.WrapErr(pkg.CodeStorage, "计划状态查询失败", err)
	}
	return row.PlanEnabled, nil
}

// Delete 删掉某会话的计划状态。
func (r *PlanRepo) Delete(sessionID string) error {
	err := r.db.Where("session_id = ?", sessionID).Delete(&domain.PlanStateDO{}).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "计划状态删除失败", err)
	}
	return nil
}

// SearchRepo 是检索文档的仓储。
type SearchRepo struct {
	// db 数据库连接
	db *gorm.DB
}

// NewSearchRepo 造一个检索仓储。
func NewSearchRepo(db *DB) *SearchRepo { return &SearchRepo{db: db.db} }

// ReplaceWorkspace 用一批新文档替换某工作区的全部文档。
// 整体替换而不是逐条 diff：索引是派生数据，重建一次比维护增量简单得多，
// 而重建的代价（一遍文件遍历）远小于增量逻辑出错的风险。
func (r *SearchRepo) ReplaceWorkspace(workspace string, docs []domain.SearchDocDO) error {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workspace_path = ?", workspace).Delete(&domain.SearchDocDO{}).Error; err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		return tx.CreateInBatches(docs, 500).Error
	})
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "检索索引写入失败", err)
	}
	return nil
}

// Search 按关键字检索文档。
// keyword 为空时列出全部（受限 limit 条）——界面打开索引面板就要先看到点东西。
func (r *SearchRepo) Search(workspace, keyword, kind string, limit int) ([]domain.SearchDocDO, error) {
	query := r.db.Model(&domain.SearchDocDO{})
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		query = query.Where("workspace_path = ?", trimmed)
	}
	if trimmed := strings.TrimSpace(kind); trimmed != "" && trimmed != "all" {
		query = query.Where("doc_kind = ?", trimmed)
	}
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("doc_path LIKE ? OR doc_symbol LIKE ? OR doc_body LIKE ?", like, like, like)
	}
	if limit <= 0 {
		limit = 50
	}
	rows := make([]domain.SearchDocDO, 0, limit)
	err := query.Order("doc_hit_count DESC, doc_last_hit_ms DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "检索查询失败", err)
	}
	return rows, nil
}

// BumpHit 给一批文档的命中计数加一，并记下最近命中时刻。
// 频次与时近共同构成 frecency 排序：常用且近用的文件排在前面。
func (r *SearchRepo) BumpHit(docIDs []string) error {
	if len(docIDs) == 0 {
		return nil
	}
	now := pkg.NowMs()
	err := r.db.Model(&domain.SearchDocDO{}).
		Where("doc_id IN ?", docIDs).
		Updates(map[string]any{"doc_hit_count": gorm.Expr("doc_hit_count + 1"), "doc_last_hit_ms": now}).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "命中计数更新失败", err)
	}
	return nil
}

// Count 统计某工作区的索引文档数。
func (r *SearchRepo) Count(workspace string) (int64, error) {
	query := r.db.Model(&domain.SearchDocDO{})
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		query = query.Where("workspace_path = ?", trimmed)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, pkg.WrapErr(pkg.CodeStorage, "索引统计失败", err)
	}
	return total, nil
}

// Clear 清掉某工作区的索引。
func (r *SearchRepo) Clear(workspace string) error {
	err := r.db.Where("workspace_path = ?", workspace).Delete(&domain.SearchDocDO{}).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "索引清空失败", err)
	}
	return nil
}

// SortedProviderNames 列服务商名并排序：注册表输出必须是确定的。
func SortedProviderNames(rows []domain.ProviderDO) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ProviderName)
	}
	sort.Strings(out)
	return out
}
