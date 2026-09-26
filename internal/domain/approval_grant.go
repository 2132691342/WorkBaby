package domain

// ApprovalGrantDO 持久化的「本会话允许」免审授权（needs_approval 类，irreversible 永不入库）。
//
// 语义：用户在某 run 内点了「本会话允许」→ 授权升级为跨重启生效；但授权与产生它的
// run 绑定记账——run 以 error/cancelled 收尾时回滚（未验证的工作不留下扩权），
// 设置页可随时撤销。
type ApprovalGrantDO struct {
	ID        string `gorm:"primaryKey;size:64"    json:"id"`
	Command   string `gorm:"type:text;uniqueIndex" json:"command"` // 免审命令（与审批请求的 command 同口径）
	Risk      string `gorm:"size:32"               json:"risk"`
	CreatedAt int64  `gorm:"autoCreateTime:milli"  json:"created_at"`
}

// TableName 固定表名。
func (ApprovalGrantDO) TableName() string { return "approval_grants" }

// ApprovalGrantRESP 免审授权出参（列表 / 撤销界面）。
type ApprovalGrantRESP struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Risk      string `json:"risk"`
	CreatedAt int64  `json:"created_at"`
}

// ApprovalPendingRESP 未决审批快照（§8 审批恢复：前端刷新后重拉注入）。
type ApprovalPendingRESP struct {
	ID        string `json:"id"` // approvalID，回填 /chat/approval/{id}/decide 用
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
	Reason    string `json:"reason"`
	Risk      string `json:"risk"`
	ExpiresAt int64  `json:"expires_at"`
	// CanRemember 是否允许「本会话允许」：只有 needs_approval 可以；
	// 不可逆操作每次都问，前端据此隐藏该选项而不是让用户选了却无效。
	CanRemember bool `json:"can_remember"`
}
