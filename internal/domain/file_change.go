package domain

import "WorkBaby/internal/pkg"

// ChangeAction 文件变更动作（文件变更与回滚点）。
type ChangeAction string

const (
	ChangeCreate ChangeAction = "create"
	ChangeModify ChangeAction = "modify"
	ChangeDelete ChangeAction = "delete"
)

// FileChangeDO 一次文件写操作的变更记录（file_changes 表）。
//
// 写前落快照 → 落 diff → 前端 FileChangesPanel 可预览/对比/一键回滚。
// v1 只覆盖 file_write（exec 的副作用不追踪：无法可靠归因）。
type FileChangeDO struct {
	ID           string       `gorm:"primaryKey;size:64"           json:"id"`
	SessionID    string       `gorm:"size:64;index:idx_fc_session" json:"session_id"`
	RunID        string       `gorm:"size:64;index"                json:"run_id"`
	ToolName     string       `gorm:"size:64"                      json:"tool_name"`
	Path         string       `gorm:"size:512"                     json:"-"` // 磁盘绝对路径，不回传前端
	RelPath      string       `gorm:"size:512"                     json:"rel_path"`
	Action       ChangeAction `gorm:"size:16"                      json:"action"`
	SnapshotPath string       `gorm:"size:512"                     json:"-"` // 变更前内容快照；create 时为空
	BytesBefore  int64        `json:"bytes_before"`
	BytesAfter   int64        `json:"bytes_after"`
	AddedLines   int          `json:"added_lines"`
	RemovedLines int          `json:"removed_lines"`
	Diff         string       `gorm:"type:text"                    json:"-"` // unified diff（详情接口单独返回）
	RolledBack   bool         `gorm:"default:false"                json:"rolled_back"`
	CreatedAt    int64        `gorm:"autoCreateTime:milli;index:idx_fc_session" json:"created_at"`
}

// TableName 固定表名。
func (FileChangeDO) TableName() string { return "file_changes" }

// FileChangeRESP 出参（前端变更面板列表项）。
type FileChangeRESP struct {
	ID           string       `json:"id"`
	SessionID    string       `json:"session_id"`
	RunID        string       `json:"run_id"`
	ToolName     string       `json:"tool_name"`
	RelPath      string       `json:"rel_path"`
	Action       ChangeAction `json:"action"`
	BytesBefore  int64        `json:"bytes_before"`
	BytesAfter   int64        `json:"bytes_after"`
	AddedLines   int          `json:"added_lines"`
	RemovedLines int          `json:"removed_lines"`
	RolledBack   bool         `json:"rolled_back"`
	CreatedAt    int64        `json:"created_at"`
}

// FileChangeDetailRESP 单条变更详情：diff 正文 + 变更前内容（回滚前对照用）。
type FileChangeDetailRESP struct {
	FileChangeRESP
	Diff       string `json:"diff"`
	BeforeText string `json:"before_text"`
	AfterText  string `json:"after_text"`
	Truncated  bool   `json:"truncated"`
}

// FileChangeListRESP 变更列表出参。
type FileChangeListRESP struct {
	Items []FileChangeRESP `json:"items"`
	Total int              `json:"total"`
}

// 错误变量；段位 4009-4010。
var (
	ErrFileChangeNotFound = pkg.New(4009, "文件变更记录不存在", "")
	ErrFileChangeRollback = pkg.New(4010, "文件回滚失败", "")
)
