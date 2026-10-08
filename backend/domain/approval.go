// 审批聚合根：审批卡的生命周期与放行范围。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrApprovalNotFound = pkg.New(4101, "审批记录不存在", "")
	ErrApprovalSettled  = pkg.New(4102, "这条审批已经处理过了", "")
	ErrApprovalTimeout  = pkg.New(4103, "等待确认超时，已按拒绝处理", "")
)

// ApprovalDO 审批记录。落表是为了跨重启后决策卡仍在。
type ApprovalDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	SessionID  string `gorm:"size:64;index" json:"session_id"`
	ToolCallID string `gorm:"size:64" json:"tool_call_id"`
	Tool       string `gorm:"size:32" json:"tool"`
	Label      string `gorm:"size:128" json:"label"`
	ArgsJSON   string `gorm:"type:text" json:"args_json"`
	Risk       string `gorm:"size:16" json:"risk"`
	Reason     string `gorm:"size:512" json:"reason"`
	Status     string `gorm:"size:16;index" json:"status"`
	CreatedAt  int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	DecidedAt  int64  `json:"decided_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (ApprovalDO) TableName() string { return "approvals" }

// ApprovalVO 审批出参。
type ApprovalVO struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"session_id"`
	ToolCallID string         `json:"tool_call_id"`
	Tool       string         `json:"tool"`
	Label      string         `json:"label"`
	Args       map[string]any `json:"args"`
	Risk       string         `json:"risk"`
	Reason     string         `json:"reason"`
	Status     string         `json:"status"`
	CreatedAt  int64          `json:"created_at"`
	DecidedAt  int64          `json:"decided_at"`
}

// DecideApprovalREQ 审批决策入参；scope=session 表示本次会话内同类工具免审。
type DecideApprovalREQ struct {
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}
