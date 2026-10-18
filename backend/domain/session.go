// 会话聚合根：一条 PI 会话文件的投影形态与迁移入参。
// 消息真身与元数据都在 PI 的 JSONL 里，这里只是它的一张视图。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrSessionNotFound = pkg.New("1101", "会话不存在", "")
	ErrEmptyContent    = pkg.New("1104", "说点什么再发送吧", "")
	ErrSessionRunning  = pkg.New("5005", "同一个文件夹里还有对话在跑，等它停下再操作", "")
)

// SessionVO 是会话列表项；字段全部来自会话文件与进程状态。
type SessionVO struct {
	// ID 是 PI 的会话编号（文件头 id），也是前端路由用的键。
	ID string `json:"id"`
	// File 是会话 JSONL 的绝对路径。
	File      string `json:"file"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	// ThinkingLevel 取 off/minimal/low/medium/high/xhigh/max；空 = 跟随 PI 默认。
	ThinkingLevel string  `json:"thinking_level"`
	MessageCount  int     `json:"message_count"`
	TotalTokens   int     `json:"total_tokens"`
	Cost          float64 `json:"cost"`
	// Running 表示这条会话现在有 run 在跑。
	Running   bool  `json:"running"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// SessionDetailVO 会话详情：元信息 + 从 JSONL 投影的消息。
type SessionDetailVO struct {
	Session  SessionVO   `json:"session"`
	Messages []MessageVO `json:"messages"`
}

// CreateSessionREQ 新建会话入参：会话在工作区里创建，其余用全局设置兜底。
type CreateSessionREQ struct {
	Workspace string `json:"workspace"`
}

// RenameSessionREQ 重命名入参。
type RenameSessionREQ struct {
	Title string `json:"title"`
}

// SetModelREQ 切换模型入参（服务商名即 models.json 的键）。
type SetModelREQ struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// SetThinkingREQ 切换思考档入参。
type SetThinkingREQ struct {
	Level string `json:"level"`
}

// SendMessageREQ 发送入参：session_id 为空表示这条对话还没落地，由 PI 现场新建。
type SendMessageREQ struct {
	SessionID   string          `json:"session_id"`
	Workspace   string          `json:"workspace"`
	Content     string          `json:"content"`
	Attachments []AttachmentREQ `json:"attachments"`
}

// AttachmentREQ 一条引用：工作区内的相对路径，或粘贴进来的图片。
type AttachmentREQ struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	ImageBase64 string `json:"image_base64"`
}

// SendMessageRESP 发送出参：session_id 可能是刚建出来的那条。
type SendMessageRESP struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
}

// SteerREQ 插话 / 排队 / 停止的入参。
type SteerREQ struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// AnswerREQ 回答引擎抛回来的问题（confirm / select / input）。
type AnswerREQ struct {
	// ID 是 PI 那次 extension_ui_request 的编号。
	ID        string `json:"id"`
	Value     string `json:"value"`
	Confirmed bool   `json:"confirmed"`
	Cancelled bool   `json:"cancelled"`
}
