package domain

// ApprovalRecordDO 审批/补充输入请求的持久化记录（暂停态可恢复，durable pause）。
//
// 审批不再是纯内存事件：run 因 ask/input_required 暂停时落 pending 行；
// 进程重启后未决请求保留（重新武装决策窗口）、仍可见、可决策，决策即触发续跑；
// 续跑后护栏链按已决记录快速放行/拒绝（consumed = 已消费，防重复免审）。
type ApprovalRecordDO struct {
	ID        string `gorm:"primaryKey;size:64"             json:"id"`
	RunID     string `gorm:"size:64;index:idx_apr_run"      json:"run_id"`
	SessionID string `gorm:"size:64;index:idx_apr_session"  json:"session_id"`
	Kind      string `gorm:"size:16;index"                  json:"kind"`    // approval | input
	Command   string `gorm:"type:text"                      json:"command"` // 待审批命令 / 补充输入问题
	Risk      string `gorm:"size:32"                        json:"risk"`
	Status    string `gorm:"size:16;index:idx_apr_status"   json:"status"` // pending/approved/denied/timeout/cancelled/answered/skipped/consumed
	Answer    string `gorm:"type:text"                      json:"answer"` // input 类用户回复
	ExpiresAt int64  `gorm:"index"                          json:"expires_at"`
	CreatedAt int64  `gorm:"autoCreateTime:milli"           json:"created_at"`
	DecidedAt int64  `gorm:"default:0"                      json:"decided_at"`
}

// TableName 固定表名。
func (ApprovalRecordDO) TableName() string { return "approval_records" }

// 审批记录状态枚举。
const (
	ApprovalStatusPending   = "pending"
	ApprovalStatusApproved  = "approved"
	ApprovalStatusDenied    = "denied"
	ApprovalStatusTimeout   = "timeout"
	ApprovalStatusCancelled = "cancelled"
	ApprovalStatusAnswered  = "answered"
	ApprovalStatusSkipped   = "skipped"
	// ApprovalStatusConsumed 已决策记录被护栏链消费（快速放行/拒绝各一次，防重复免审）。
	ApprovalStatusConsumed = "consumed"
)

// 审批记录类别。
const (
	ApprovalKindApproval = "approval"
	ApprovalKindInput    = "input"
)

// AnswerInputREQ 补充输入回复入参。
type AnswerInputREQ struct {
	Answer string `json:"answer"`
}
