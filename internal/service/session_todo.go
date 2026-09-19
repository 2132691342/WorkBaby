package service

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/repo"
)

// SessionTodoStore 会话计划持久化存储（实现 internal/tool/todo.Store 与 capability.TodoStore）。
//
// 计划随会话落库：跨进程重启、归档、切换会话后仍能回放进度，不再依赖进程内存。
// 覆盖语义由 repo.ReplaceAll（事务删旧 + 插新）承担。
type SessionTodoStore struct {
	repo *repo.SessionTodoRepo
}

// NewSessionTodoStore 构造；repo 为 nil 时读写退化为空操作（不阻断 run）。
func NewSessionTodoStore(r *repo.SessionTodoRepo) *SessionTodoStore {
	return &SessionTodoStore{repo: r}
}

// Load 返回该会话当前计划。
func (s *SessionTodoStore) Load(ctx context.Context, sessionID string) ([]domain.TodoItem, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	rows, err := s.repo.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return toTodoItems(rows), nil
}

// Save 覆盖该会话计划。
func (s *SessionTodoStore) Save(ctx context.Context, sessionID string, items []domain.TodoItem) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.ReplaceAll(ctx, sessionID, items)
}

// State 返回会话计划快照（done/total 已统计）。
func (s *SessionTodoStore) State(ctx context.Context, sessionID string) (domain.TodoStateRESP, error) {
	items, err := s.Load(ctx, sessionID)
	if err != nil {
		return domain.TodoStateRESP{SessionID: sessionID}, err
	}
	return stateOf(sessionID, items), nil
}

// Toggle 用户手动勾选/取消某条待办（会话计划面板交互）。
// 与模型侧的 todo 工具写同一份状态，下一轮 system 注入会带上最新进度。
func (s *SessionTodoStore) Toggle(ctx context.Context, sessionID, itemID string) (domain.TodoStateRESP, error) {
	if s == nil || s.repo == nil {
		return domain.TodoStateRESP{SessionID: sessionID}, nil
	}
	items, err := s.Load(ctx, sessionID)
	if err != nil {
		return domain.TodoStateRESP{SessionID: sessionID}, err
	}
	for i := range items {
		if items[i].ID != itemID {
			continue
		}
		next := !items[i].Done
		if _, err := s.repo.UpdateDone(ctx, sessionID, itemID, next); err != nil {
			return domain.TodoStateRESP{SessionID: sessionID}, err
		}
		items[i].Done = next
		return stateOf(sessionID, items), nil
	}
	return domain.TodoStateRESP{SessionID: sessionID}, domain.ErrTodoItemNotFound
}

// toTodoItems DO → 领域项（ItemID 承载模型给出的标识，主键仅用于存储）。
func toTodoItems(rows []domain.SessionTodoDO) []domain.TodoItem {
	out := make([]domain.TodoItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.TodoItem{ID: r.ItemID, Title: r.Title, Done: r.Done})
	}
	return out
}

// stateOf 由 items 组装快照。
func stateOf(sessionID string, items []domain.TodoItem) domain.TodoStateRESP {
	done := 0
	for _, it := range items {
		if it.Done {
			done++
		}
	}
	return domain.TodoStateRESP{SessionID: sessionID, Items: items, DoneCount: done, Total: len(items)}
}
