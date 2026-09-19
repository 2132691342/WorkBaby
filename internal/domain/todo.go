package domain

import "WorkBaby/internal/pkg"

// TodoItem 会话内计划项（todo 工具的唯一有状态数据）。
type TodoItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// TodoStateRESP 计划状态快照（供前端进度卡 / 后续 API 复用）。
type TodoStateRESP struct {
	SessionID string     `json:"session_id"`
	Items     []TodoItem `json:"items"`
	DoneCount int        `json:"done_count"`
	Total     int        `json:"total"`
}

// SessionTodoDO 会话计划项持久化实体：计划随会话落库，跨进程重启与归档后仍可回放。
// 覆盖语义由 repo 的 ReplaceAll（事务删旧 + 插新）承担。
type SessionTodoDO struct {
	ID        string `gorm:"primaryKey;size:64"`
	SessionID string `gorm:"size:64;uniqueIndex:idx_todo_session_item"`
	ItemID    string `gorm:"size:64;uniqueIndex:idx_todo_session_item"`
	Title     string `gorm:"type:text"`
	Done      bool   `gorm:"default:false"`
	Ordering  int    `gorm:"default:0"`
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (SessionTodoDO) TableName() string { return "session_todos" }

// ErrTodoItemNotFound 勾选不存在的计划项（前端面板可能持有过期快照）。
var ErrTodoItemNotFound = pkg.New(5009, "todo item not found", "")
