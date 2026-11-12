package repo

import (
	"sort"
	"strings"

	"gorm.io/gorm"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// SessionRepo 是会话索引的仓储。
// 会话正文不在这里——JSONL 文件是真相，本表只是「可查询的目录」。
type SessionRepo struct {
	// db 数据库连接
	db *gorm.DB
}

// NewSessionRepo 造一个会话索引仓储。
func NewSessionRepo(db *DB) *SessionRepo { return &SessionRepo{db: db.db} }

// Upsert 写入或更新一条会话索引。
func (r *SessionRepo) Upsert(row domain.SessionIndexDO) error {
	err := r.db.Save(&row).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "会话索引写入失败", err)
	}
	return nil
}

// List 列出会话索引，按最近修改在前。可按工作区过滤。
func (r *SessionRepo) List(workspace string, limit int) ([]domain.SessionIndexDO, error) {
	if limit <= 0 {
		limit = maxIndexRows
	}
	query := r.db.Model(&domain.SessionIndexDO{})
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		query = query.Where("workspace_path = ?", trimmed)
	}
	rows := make([]domain.SessionIndexDO, 0, 32)
	err := query.Order("updated_at_ms DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "会话索引查询失败", err)
	}
	return rows, nil
}

// Find 按会话 ID 取一行。
func (r *SessionRepo) Find(sessionID string) (domain.SessionIndexDO, bool, error) {
	row := domain.SessionIndexDO{}
	err := r.db.Where("session_id = ?", sessionID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return domain.SessionIndexDO{}, false, nil
	}
	if err != nil {
		return domain.SessionIndexDO{}, false, pkg.WrapErr(pkg.CodeStorage, "会话索引查询失败", err)
	}
	return row, true, nil
}

// Delete 删掉一行索引。
func (r *SessionRepo) Delete(sessionID string) error {
	err := r.db.Where("session_id = ?", sessionID).Delete(&domain.SessionIndexDO{}).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "会话索引删除失败", err)
	}
	return nil
}

// Rename 只改标题：列清单时改个名不该动其它字段。
func (r *SessionRepo) Rename(sessionID, title string) error {
	err := r.db.Model(&domain.SessionIndexDO{}).
		Where("session_id = ?", sessionID).
		Update("session_title", title).Error
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "会话改名失败", err)
	}
	return nil
}

// Count 统计会话数；workspace 为空时统计全部。
func (r *SessionRepo) Count(workspace string) (int64, error) {
	query := r.db.Model(&domain.SessionIndexDO{})
	if trimmed := strings.TrimSpace(workspace); trimmed != "" {
		query = query.Where("workspace_path = ?", trimmed)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, pkg.WrapErr(pkg.CodeStorage, "会话统计失败", err)
	}
	return total, nil
}

// 聚合查询的行数上限。
// 这些查询要把一段时间内的记录读进内存做分桶或分组，
// 上限防止一张被灌爆的表把内存吃光；真实用量远低于这个数。
const (
	maxUsageRows = 20000
	maxToolRows  = 20000
	maxIndexRows = 5000
)

// UsageRepo 是用量与工具调用记录的仓储。
type UsageRepo struct {
	// db 数据库连接
	db *gorm.DB
}

// NewUsageRepo 造一个用量仓储。
func NewUsageRepo(db *DB) *UsageRepo { return &UsageRepo{db: db.db} }

// Record 记一条用量。
func (r *UsageRepo) Record(row domain.UsageRecordDO) error {
	if err := r.db.Create(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "用量记录写入失败", err)
	}
	return nil
}

// RecordToolCall 记一条工具调用。
func (r *UsageRepo) RecordToolCall(row domain.ToolCallRecordDO) error {
	if err := r.db.Create(&row).Error; err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "工具调用记录写入失败", err)
	}
	return nil
}

// UsageSummary 是一段区间内的用量汇总。
type UsageSummary struct {
	// InputTokens 输入 token 合计
	InputTokens int64 `json:"input_tokens"`
	// OutputTokens 输出 token 合计
	OutputTokens int64 `json:"output_tokens"`
	// CacheReadTokens 缓存命中 token 合计
	CacheReadTokens int64 `json:"cache_read_tokens"`
	// TotalTokens 合计 token
	TotalTokens int64 `json:"total_tokens"`
	// RecordCount 记录条数
	RecordCount int64 `json:"record_count"`
	// ToolCallCount 工具调用次数
	ToolCallCount int64 `json:"tool_call_count"`
	// ToolErrorCount 工具失败次数
	ToolErrorCount int64 `json:"tool_error_count"`
}

// Summary 汇总一段区间内的用量。sinceMs 为 0 表示不限起点。
func (r *UsageRepo) Summary(sinceMs int64) (UsageSummary, error) {
	out := UsageSummary{}
	query := r.db.Model(&domain.UsageRecordDO{})
	if sinceMs > 0 {
		query = query.Where("recorded_at_ms >= ?", sinceMs)
	}
	err := query.Select(
		"COALESCE(SUM(input_tokens), 0) AS input_tokens, " +
			"COALESCE(SUM(output_tokens), 0) AS output_tokens, " +
			"COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens, " +
			"COALESCE(SUM(total_tokens), 0) AS total_tokens, " +
			"COUNT(*) AS record_count").Scan(&out).Error
	if err != nil {
		return UsageSummary{}, pkg.WrapErr(pkg.CodeStorage, "用量汇总失败", err)
	}
	toolQuery := r.db.Model(&domain.ToolCallRecordDO{})
	if sinceMs > 0 {
		toolQuery = toolQuery.Where("recorded_at_ms >= ?", sinceMs)
	}
	err = toolQuery.Select(
		"COUNT(*) AS tool_call_count, " +
			"COALESCE(SUM(CASE WHEN tool_is_error THEN 1 ELSE 0 END), 0) AS tool_error_count").Scan(&out).Error
	if err != nil {
		return UsageSummary{}, pkg.WrapErr(pkg.CodeStorage, "工具统计失败", err)
	}
	return out, nil
}

// UsageBucket 是一个时间桶的用量。
type UsageBucket struct {
	// BucketAt 桶起点（毫秒）
	BucketAt int64 `json:"bucket_at"`
	// TotalTokens 合计 token
	TotalTokens int64 `json:"total_tokens"`
	// InputTokens 输入 token
	InputTokens int64 `json:"input_tokens"`
	// OutputTokens 输出 token
	OutputTokens int64 `json:"output_tokens"`
	// RecordCount 记录条数
	RecordCount int64 `json:"record_count"`
}

// Buckets 按天或按小时分桶统计用量。
// SQLite 没有原生时间函数可用（纯 Go 驱动的 strftime 受限于编译标签），
// 所以在 Go 侧分桶：数据量是本机会话级，扫一遍完全够用。
func (r *UsageRepo) Buckets(sinceMs int64, bucketMs int64) ([]UsageBucket, error) {
	if bucketMs <= 0 {
		bucketMs = 24 * 60 * 60 * 1000
	}
	query := r.db.Model(&domain.UsageRecordDO{})
	if sinceMs > 0 {
		query = query.Where("recorded_at_ms >= ?", sinceMs)
	}
	rows := make([]domain.UsageRecordDO, 0, 256)
	if err := query.Limit(maxUsageRows).Find(&rows).Error; err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "用量查询失败", err)
	}
	buckets := map[int64]*UsageBucket{}
	for _, row := range rows {
		key := row.RecordedAtMs / bucketMs * bucketMs
		item, ok := buckets[key]
		if !ok {
			item = &UsageBucket{BucketAt: key}
			buckets[key] = item
		}
		item.TotalTokens += row.TotalTokens
		item.InputTokens += int64(row.InputTokens)
		item.OutputTokens += int64(row.OutputTokens)
		item.RecordCount++
	}
	out := make([]UsageBucket, 0, len(buckets))
	for _, item := range buckets {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BucketAt < out[j].BucketAt })
	return out, nil
}

// ModelUsage 是按模型分组的用量。
type ModelUsage struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
	// TotalTokens 合计 token
	TotalTokens int64 `json:"total_tokens"`
	// RecordCount 记录条数
	RecordCount int64 `json:"record_count"`
}

// ByModel 按模型分组统计用量。
func (r *UsageRepo) ByModel(sinceMs int64) ([]ModelUsage, error) {
	query := r.db.Model(&domain.UsageRecordDO{})
	if sinceMs > 0 {
		query = query.Where("recorded_at_ms >= ?", sinceMs)
	}
	rows := make([]domain.UsageRecordDO, 0, 256)
	if err := query.Limit(maxUsageRows).Find(&rows).Error; err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "用量查询失败", err)
	}
	groups := map[string]*ModelUsage{}
	for _, row := range rows {
		key := row.ProviderName + "\x00" + row.ModelId
		item, ok := groups[key]
		if !ok {
			item = &ModelUsage{ProviderName: row.ProviderName, ModelId: row.ModelId}
			groups[key] = item
		}
		item.TotalTokens += row.TotalTokens
		item.RecordCount++
	}
	out := make([]ModelUsage, 0, len(groups))
	for _, item := range groups {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalTokens > out[j].TotalTokens })
	return out, nil
}

// ToolUsage 是按工具分组的调用统计。
type ToolUsage struct {
	// ToolName 工具名
	ToolName string `json:"tool_name"`
	// CallCount 调用次数
	CallCount int64 `json:"call_count"`
	// ErrorCount 失败次数
	ErrorCount int64 `json:"error_count"`
	// AvgDurationMs 平均耗时（毫秒）
	AvgDurationMs int64 `json:"avg_duration_ms"`
}

// ByTool 按工具分组统计调用次数与耗时。
func (r *UsageRepo) ByTool(sinceMs int64) ([]ToolUsage, error) {
	query := r.db.Model(&domain.ToolCallRecordDO{})
	if sinceMs > 0 {
		query = query.Where("recorded_at_ms >= ?", sinceMs)
	}
	rows := make([]domain.ToolCallRecordDO, 0, 512)
	if err := query.Limit(maxToolRows).Find(&rows).Error; err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "工具统计查询失败", err)
	}
	groups := map[string]*ToolUsage{}
	durations := map[string]int64{}
	for _, row := range rows {
		item, ok := groups[row.ToolName]
		if !ok {
			item = &ToolUsage{ToolName: row.ToolName}
			groups[row.ToolName] = item
		}
		item.CallCount++
		durations[row.ToolName] += row.DurationMs
		if row.ToolIsError {
			item.ErrorCount++
		}
	}
	out := make([]ToolUsage, 0, len(groups))
	for name, item := range groups {
		if item.CallCount > 0 {
			item.AvgDurationMs = durations[name] / item.CallCount
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CallCount > out[j].CallCount })
	return out, nil
}
