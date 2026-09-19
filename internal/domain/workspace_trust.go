package domain

import "WorkBaby/internal/pkg"

// TrustState 工作目录信任态（工作目录信任三态）。
//
// 挂在工具闸门前：工具目标目录解析出 deny → 直接拒绝（Refused 回执，模型可自愈）；
// ask → 走人工审批，批准后落盘为 allow，同目录后续不再打扰。
type TrustState string

const (
	// TrustStateAllow 已信任：工具在该目录内直行。
	TrustStateAllow TrustState = "allow"
	// TrustStateAsk 未登记：执行前询问（默认态，fail-closed）。
	TrustStateAsk TrustState = "ask"
	// TrustStateDeny 明确拒绝：永不执行，也不再询问。
	TrustStateDeny TrustState = "deny"
)

// ParseTrustState 解析信任态字符串；未知/空值返回 ask（fail-closed）。
func ParseTrustState(s string) TrustState {
	switch TrustState(s) {
	case TrustStateAllow, TrustStateDeny:
		return TrustState(s)
	default:
		return TrustStateAsk
	}
}

// WorkspaceTrustDO 工作目录信任登记（workspace_trust 表）。
//
// Path 是主键（绝对路径规范化后的形态）；就近查找由 service 层沿祖先链上溯实现，
// 只登记用户显式决策过的目录——未登记的目录恒为 ask，不写库，避免污染。
type WorkspaceTrustDO struct {
	Path      string     `gorm:"primaryKey;size:512" json:"path"`
	State     TrustState `gorm:"size:16"             json:"state"`
	CreatedAt int64      `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64      `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (WorkspaceTrustDO) TableName() string { return "workspace_trust" }

// WorkspaceTrustREQ 信任决策入参。
type WorkspaceTrustREQ struct {
	Path  string `json:"path"`
	State string `json:"state"` // allow / ask / deny；其余值按 ask 处理
}

// WorkspaceTrustRESP 信任登记项（列表展示）。
type WorkspaceTrustRESP struct {
	Path      string     `json:"path"`
	State     TrustState `json:"state"`
	UpdatedAt int64      `json:"updated_at"`
}

// TrustResolveRESP 单目录信任解析结果。
//
// Source 是实际命中的登记目录（可能与入参不同：沿祖先链上溯命中）；
// Source 为空表示未命中任何登记，State 为默认的 ask。
type TrustResolveRESP struct {
	Path    string     `json:"path"`
	State   TrustState `json:"state"`
	Source  string     `json:"source"`
	Matched bool       `json:"matched"`
}

// 包级错误变量；段位 1000-1999（通用 / 文件 / 路径）。
var ErrTrustPathEmpty = pkg.New(1020, "trust path is empty", "")
