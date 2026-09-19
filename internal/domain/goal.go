package domain

// SessionGoal 会话目标：一句话可校验的目标 +
// 每轮结束自动校验是否达成，未达成则携带下一步动作自动续跑。
type SessionGoal struct {
	Text        string `json:"text"`                   // 目标描述（越具体、越可校验越好）
	Status      string `json:"status"`                 // active / paused / done
	Round       int    `json:"round"`                  // 已自动推进的轮数（含当前轮）
	MaxRounds   int    `json:"max_rounds"`             // 自动续跑轮数上限（防失控；0 = 默认 20）
	NextStep    string `json:"next_step,omitempty"`    // 最近一次校验给出的下一步动作
	DoneBecause string `json:"done_because,omitempty"` // 判定达成时的依据摘要
	UpdatedAt   int64  `json:"updated_at"`
}

// 目标状态取值。
const (
	GoalStatusActive = "active"
	GoalStatusPaused = "paused"
	GoalStatusDone   = "done"
)

// GoalDefaultMaxRounds 自动续跑轮数默认上限。
const GoalDefaultMaxRounds = 20

// GoalREQ 目标操作入参：set/replace 携带 Text；pause/resume/clear 忽略 Text。
type GoalREQ struct {
	Action string `json:"action"`
	Text   string `json:"text,omitempty"`
}

// GoalRESP 目标操作出参；Goal 为 null 表示会话当前无目标。
type GoalRESP struct {
	SessionID string       `json:"session_id"`
	Goal      *SessionGoal `json:"goal"`
}
