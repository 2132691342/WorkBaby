package service

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// sqlCheckpointStore 把 core.Checkpoint 持久化到 agent_checkpoints 表。
// 每 run 只保留最新一轮快照（Resume 只认最后一轮，避免磁盘放大）；
// 序列化在 service 层收敛，repo 只做无业务语义读写。
type sqlCheckpointStore struct {
	repo *repo.AgentCheckpointRepo
}

// NewSQLCheckpointStore 构造 SQL 检查点存储（实现 core.CheckpointStore）。
func NewSQLCheckpointStore(r *repo.AgentCheckpointRepo) core.CheckpointStore {
	return &sqlCheckpointStore{repo: r}
}

// Append 保存一轮检查点（(run_id, turn) upsert），随后清掉同 run 的更早轮次。
func (s *sqlCheckpointStore) Append(cp *core.Checkpoint) error {
	ctx := context.Background()
	messages, err := json.Marshal(cp.Messages)
	if err != nil {
		return pkg.Wrap(5005, "checkpoint messages marshal failed", err)
	}
	usage, err := json.Marshal(cp.Usage)
	if err != nil {
		return pkg.Wrap(5005, "checkpoint usage marshal failed", err)
	}
	stepsJSON := ""
	if len(cp.Steps) > 0 {
		bs, serr := json.Marshal(cp.Steps)
		if serr != nil {
			return pkg.Wrap(5005, "checkpoint steps marshal failed", serr)
		}
		stepsJSON = string(bs)
	}
	row := &domain.AgentCheckpointDO{
		ID:                 pkg.NewID("CP"),
		SessionID:          cp.SessionID,
		RunID:              cp.RunID,
		Turn:               cp.Turn,
		AssistantMessageID: cp.AssistantMsgID,
		MessagesJSON:       string(messages),
		// StateJSON 是旧内核的停滞计数载体；新内核的守卫自带计数，
		// 写空对象保持列兼容，不做数据迁移。
		StateJSON: "{}",
		UsageJSON: string(usage),
		StepsJSON: stepsJSON,
		Content:   cp.Content,
		Thinking:  cp.Thinking,
	}
	if err := s.repo.Save(ctx, row); err != nil {
		return err
	}
	return s.repo.DeleteBeforeTurn(ctx, cp.RunID, cp.Turn)
}

// LoadLast 取最新一轮检查点。
func (s *sqlCheckpointStore) LoadLast(runID string) (*core.Checkpoint, error) {
	row, err := s.repo.LoadLast(context.Background(), runID)
	if err != nil {
		return nil, err
	}
	cp := &core.Checkpoint{
		RunID:          row.RunID,
		SessionID:      row.SessionID,
		Turn:           row.Turn,
		AssistantMsgID: row.AssistantMessageID,
		Content:        row.Content,
		Thinking:       row.Thinking,
		CreatedAt:      row.CreatedAt,
	}
	if err := json.Unmarshal([]byte(row.MessagesJSON), &cp.Messages); err != nil {
		return nil, pkg.Wrap(5005, "checkpoint messages parse failed", err)
	}
	if err := json.Unmarshal([]byte(row.UsageJSON), &cp.Usage); err != nil {
		return nil, pkg.Wrap(5005, "checkpoint usage parse failed", err)
	}
	if row.StepsJSON != "" {
		if err := json.Unmarshal([]byte(row.StepsJSON), &cp.Steps); err != nil {
			return nil, pkg.Wrap(5005, "checkpoint steps parse failed", err)
		}
	}
	return cp, nil
}

// Cleanup 保留最近 keep 个 run 的检查点。
func (s *sqlCheckpointStore) Cleanup(sessionID string, keep int) error {
	return s.repo.CleanupRuns(context.Background(), sessionID, keep)
}
