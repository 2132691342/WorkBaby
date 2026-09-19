package domain

// AgentCheckpointDO Agent 运行检查点：跨进程重启可续跑。
// 复杂字段 JSON 文本化，转换在 service 层完成；每个 run 只保留最新一轮（避免磁盘放大）。
type AgentCheckpointDO struct {
	ID                 string `gorm:"primaryKey;size:64"                    json:"id"`
	SessionID          string `gorm:"size:64;index:idx_cp_session"          json:"session_id"`
	RunID              string `gorm:"size:64;index:idx_cp_session;uniqueIndex:uk_cp_run_turn,priority:1" json:"run_id"`
	Turn               int    `gorm:"uniqueIndex:uk_cp_run_turn,priority:2" json:"turn"`
	AssistantMessageID string `gorm:"size:64;index"                        json:"assistant_message_id"`
	MessagesJSON       string `gorm:"type:text"                             json:"messages_json"`
	StateJSON          string `gorm:"type:text"                             json:"state_json"` // 旧内核的停滞计数载体；现行写空对象
	UsageJSON          string `gorm:"type:text"                             json:"usage_json"` // llm.TokenUsage
	StepsJSON          string `gorm:"type:text"                             json:"steps_json"` // map[string]string：幂等恢复的已完成调用（旧行空串）
	Content            string `gorm:"type:text"                             json:"content"`
	Thinking           string `gorm:"type:text"                             json:"thinking"`
	CreatedAt          int64  `gorm:"autoCreateTime:milli"                  json:"created_at"`
}

// TableName 固定表名。
func (AgentCheckpointDO) TableName() string { return "agent_checkpoints" }
