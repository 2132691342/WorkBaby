package service

// 检查点续跑语义：每 run 只留最新一轮快照，且按会话保留最近 N 个 run。

import (
	"context"
	"testing"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newCheckpointTestStore(t *testing.T) (*sqlCheckpointStore, *repo.AgentCheckpointRepo, *gorm.DB) {
	t.Helper()
	gdb := newChatOpsTestDB(t)
	r := repo.NewAgentCheckpointRepo(gdb)
	return &sqlCheckpointStore{repo: r}, r, gdb
}

func checkpointFixture(runID string, turn int) *core.Checkpoint {
	return &core.Checkpoint{
		RunID:          runID,
		SessionID:      "SESSION_CP",
		Turn:           turn,
		AssistantMsgID: "MSG_CP",
		Messages: []*llm.Message{
			llm.SystemMessage("你是 WorkBaby"),
			llm.UserMessage("第 " + string(rune('0'+turn)) + " 轮输入"),
		},
		Content:  "已累积正文 " + string(rune('0'+turn)),
		Thinking: "推理 " + string(rune('0'+turn)),
		Usage:    llm.TokenUsage{InputTokens: 100 * (turn + 1), OutputTokens: 10, TotalTokens: 100*(turn+1) + 10},
		Steps:    map[string]string{"tool:echo|{}": "echo:ok"},
	}
}

// TestCheckpointKeepsLatestSnapshotOnly 连写 4 轮：表里只剩最后一轮，且字段无损还原。
func TestCheckpointKeepsLatestSnapshotOnly(t *testing.T) {
	store, _, gdb := newCheckpointTestStore(t)

	for turn := 0; turn < 4; turn++ {
		require.NoError(t, store.Append(checkpointFixture("RUN_CP1", turn)))

	}

	var rows int64
	require.NoError(t, gdb.Model(&domain.AgentCheckpointDO{}).Where("run_id = ?", "RUN_CP1").Count(&rows).Error)
	assert.Equal(t, int64(1), rows, "每轮都留一份会让长 run 放大成 N× 全文")

	got, err := store.LoadLast("RUN_CP1")
	require.NoError(t, err)
	assert.Equal(t, 3, got.Turn)
	assert.Equal(t, "已累积正文 3", got.Content)
	assert.Equal(t, "推理 3", got.Thinking)
	assert.Equal(t, 400, got.Usage.InputTokens)
	assert.Equal(t, "MSG_CP", got.AssistantMsgID)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, llm.RoleSystem, got.Messages[0].Role, "system 段不能丢，否则 Resume 后模型失去人设与工具约定")
	assert.Equal(t, "echo:ok", got.Steps["tool:echo|{}"], "幂等记录丢了会重放副作用")
}

// TestCheckpointCleanupKeepsRecentRuns 清理：保留最近 2 个 run，更早的整 run 删除；删会话则全清。
func TestCheckpointCleanupKeepsRecentRuns(t *testing.T) {
	store, r, _ := newCheckpointTestStore(t)
	ctx := context.Background()

	for _, runID := range []string{"RUN_A", "RUN_B", "RUN_C"} {
		require.NoError(t, store.Append(checkpointFixture(runID, 0)))
		time.Sleep(2 * time.Millisecond) // created_at 毫秒精度：拉开时间保证清理顺序确定
	}
	total, err := r.CountBySession(ctx, "SESSION_CP")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)

	require.NoError(t, store.Cleanup("SESSION_CP", 2))
	kept, err := r.CountBySession(ctx, "SESSION_CP")
	require.NoError(t, err)
	assert.Equal(t, int64(2), kept)
	_, err = store.LoadLast("RUN_A")
	assert.Error(t, err, "最早的 run 应被清理")
	_, err = store.LoadLast("RUN_C")
	assert.NoError(t, err, "最近的 run 必须保留")

	require.NoError(t, r.DeleteBySession(ctx, "SESSION_CP"))
	left, err := r.CountBySession(ctx, "SESSION_CP")
	require.NoError(t, err)
	assert.Zero(t, left)
}
