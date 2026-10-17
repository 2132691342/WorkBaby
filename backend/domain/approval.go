// 审批聚合根：审批卡的生命周期与放行范围。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrApprovalNotFound = pkg.New("4101", "审批记录不存在", "")
	ErrApprovalSettled  = pkg.New("4102", "这条审批已经处理过了", "")
	ErrApprovalTimeout  = pkg.New("4103", "等待确认超时，已按拒绝处理", "")
)

// ApprovalDO 审批记录。落表是为了跨重启后决策卡仍在。
type ApprovalDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	SessionID  string `gorm:"size:64;index" json:"session_id"`
	ToolCallID string `gorm:"size:64" json:"tool_call_id"`
	Tool       string `gorm:"size:32" json:"tool"`
	Label      string `gorm:"size:128" json:"label"`
	ArgsJSON   string `gorm:"type:text" json:"args_json"`
	Risk       string `gorm:"size:16" json:"risk"`         // Risk* 取值
	Reason     string `gorm:"size:512" json:"reason"`      // 为什么要问这一句
	Status     string `gorm:"size:16;index" json:"status"` // Approval* 取值
	CreatedAt  int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	DecidedAt  int64  `json:"decided_at"` // 0 = 还没决策
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
	Risk       string         `json:"risk"`   // Risk* 取值
	Reason     string         `json:"reason"` // 为什么要问这一句
	Status     string         `json:"status"` // Approval* 取值
	CreatedAt  int64          `json:"created_at"`
	DecidedAt  int64          `json:"decided_at"` // 0 = 还没决策
}

// DecideApprovalREQ 审批决策入参；action 取 allow / deny，scope=session 表示本会话内同类工具免审。
// action 放请求体而不是路径，好让路径变量落在末尾。
type DecideApprovalREQ struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"`
	Scope     string `json:"scope"`
	Reason    string `json:"reason"`
}

// ApprovalCallDTO 是审批门需要的调用视图：内核事件 → 审批门。
type ApprovalCallDTO struct {
	ID     string
	Args   map[string]any
	Risk   string
	Reason string
}

// ApprovalDecisionDTO 是用户在审批卡上的选择；scope=session 表示本会话同类免审。
type ApprovalDecisionDTO struct {
	Approved bool
	Scope    string
}
