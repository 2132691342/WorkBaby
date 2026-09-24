package core

import "WorkBaby/internal/llm"

// Checkpoint 续跑位点：每轮工具回填后写入，同一 run 只保留最新一轮。
type Checkpoint struct {
	RunID          string
	SessionID      string
	Turn           int
	AssistantMsgID string            // 续跑落回同一条 assistant 消息，不新开
	Messages       []*llm.Message    // 完整消息序列（含 system）
	Content        string            // 已累积正文
	Thinking       string            // 已累积推理
	Usage          llm.TokenUsage    // 跨段累计用量
	Steps          map[string]string // 幂等步骤记忆：tool:{name}|{args} → 结果
	CreatedAt      int64
}

// CheckpointStore 检查点读写；由 app 层注入持久化实现。
// Append 语义是「可覆盖的最新一轮」：同一 run 只保留最后位点，否则磁盘随轮次线性放大。
type CheckpointStore interface {
	Append(cp *Checkpoint) error
	LoadLast(runID string) (*Checkpoint, error)
	Cleanup(sessionID string, keep int) error
}
