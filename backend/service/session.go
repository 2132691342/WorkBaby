// 会话编排：列表与详情读 PI 的会话文件，改名 / 删除 / 换模型都落到同一条会话上。
// 这里不持有会话表——会话目录即真相，界面与 pi 命令看到的是同一批会话。
package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// SessionService 把 PI 的会话文件投影成界面可用的会话。
type SessionService struct {
	env *Env
}

// NewSessionService 构造会话服务。
func NewSessionService(env *Env) *SessionService { return &SessionService{env: env} }

// List 列出会话；workspace 非空时只列那个工作目录下的会话。
func (s *SessionService) List(workspace string) ([]domain.SessionVO, error) {
	metas, err := pi.ListSessions(s.env.PiEnv.SessionsDir())
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionVO, 0, len(metas))
	for _, m := range metas {
		if workspace != "" && !samePath(m.Cwd, workspace) {
			continue
		}
		out = append(out, s.toVO(m))
	}
	return out, nil
}

// toVO 把会话文件元信息转成出参；运行态从对话服务现取。
func (s *SessionService) toVO(m pi.SessionMeta) domain.SessionVO {
	vo := domain.SessionVO{
		ID: m.ID, File: m.File, Title: m.Title(), Workspace: m.Cwd,
		Provider: m.Provider, Model: m.Model, ThinkingLevel: m.ThinkingLevel,
		MessageCount: m.MessageCount, TotalTokens: m.TotalTokens, Cost: m.Cost,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if s.env.Chat != nil && s.env.Chat.IsRunning(m.ID) {
		vo.Running = true
	}
	return vo
}

// meta 按会话编号找文件。
func (s *SessionService) meta(id string) (pi.SessionMeta, error) {
	if id == "" {
		return pi.SessionMeta{}, domain.ErrSessionNotFound
	}
	metas, err := pi.ListSessions(s.env.PiEnv.SessionsDir())
	if err != nil {
		return pi.SessionMeta{}, err
	}
	for _, m := range metas {
		if m.ID == id {
			return m, nil
		}
	}
	return pi.SessionMeta{}, domain.ErrSessionNotFound
}

// Get 取一条会话。
func (s *SessionService) Get(id string) (*domain.SessionVO, error) {
	m, err := s.meta(id)
	if err != nil {
		return nil, err
	}
	vo := s.toVO(m)
	return &vo, nil
}

// Detail 取会话与从 JSONL 投影的消息。
func (s *SessionService) Detail(id string) (*domain.SessionDetailVO, error) {
	m, err := s.meta(id)
	if err != nil {
		return nil, err
	}
	data, err := pi.LoadSession(m.File)
	if err != nil {
		return nil, err
	}
	msgs := []domain.MessageVO{}
	for i := range data.Entries {
		if msg, ok := projectEntry(&data.Entries[i]); ok {
			msgs = append(msgs, msg)
		}
	}
	vo := s.toVO(data.Meta)
	return &domain.SessionDetailVO{Session: vo, Messages: msgs}, nil
}

// Rename 改会话名：走 PI 的 set_session_name，它写进会话文件，pi 的 /resume 也认。
// 改名要切到那条会话上，所以同工作区有对话在跑时拒绝。
func (s *SessionService) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return pkg.New("1105", "会话名不能为空", "")
	}
	m, err := s.meta(id)
	if err != nil {
		return err
	}
	client, err := s.idleClient(m.Cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.SwitchSession(ctx, m.File); err != nil {
		return err
	}
	if err := client.SetSessionName(ctx, title); err != nil {
		return err
	}
	pi.InvalidateSessionCache(m.File)
	return nil
}

// Delete 删除会话：直接删那个 JSONL 文件（PI 的会话就是这么删的）。
func (s *SessionService) Delete(id string) error {
	m, err := s.meta(id)
	if err != nil {
		return err
	}
	if s.env.Chat != nil && s.env.Chat.IsRunning(id) {
		return domain.ErrSessionRunning
	}
	// 进程可能正停在它上面：先切走，再删文件，免得 PI 继续往已删文件追加。
	if client := s.env.PiSupervisor().Get(m.Cwd); client != nil && client.Alive() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = client.NewSession(ctx, "")
		cancel()
	}
	if err := os.Remove(m.File); err != nil && !os.IsNotExist(err) {
		return pkg.Wrap("1108", "删除会话文件失败", err)
	}
	pi.InvalidateSessionCache(m.File)
	if s.env.Chat != nil {
		s.env.Chat.Forget(id)
	}
	return nil
}

// SetModel 切换会话模型：写 PI 的会话条目，下次请求就用它。
func (s *SessionService) SetModel(id, provider, model string) error {
	if provider == "" || model == "" {
		return pkg.New("1109", "先选好服务商与模型", "")
	}
	m, err := s.meta(id)
	if err != nil {
		return err
	}
	client, err := s.idleClient(m.Cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.SwitchSession(ctx, m.File); err != nil {
		return err
	}
	if err := client.SetModel(ctx, provider, model); err != nil {
		return err
	}
	pi.InvalidateSessionCache(m.File)
	return nil
}

// SetThinking 切换思考档位。
func (s *SessionService) SetThinking(id, level string) error {
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return pkg.New("1107", "思考档位不正确", level)
	}
	m, err := s.meta(id)
	if err != nil {
		return err
	}
	client, err := s.idleClient(m.Cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.SwitchSession(ctx, m.File); err != nil {
		return err
	}
	if err := client.SetThinkingLevel(ctx, level); err != nil {
		return err
	}
	pi.InvalidateSessionCache(m.File)
	return nil
}

// idleClient 取工作区进程并确认没有对话在跑：切会话会动进程的当前指针。
func (s *SessionService) idleClient(workspace string) (*pi.Client, error) {
	if s.env.Chat != nil && s.env.Chat.RunningIn(workspace) {
		return nil, domain.ErrSessionRunning
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.env.PiSupervisor().Ensure(ctx, workspace)
}

// FileOf 返回会话文件路径（导出、排查用）。
func (s *SessionService) FileOf(id string) (string, error) {
	m, err := s.meta(id)
	if err != nil {
		return "", err
	}
	return m.File, nil
}

// projectEntry 把一行会话条目投影成消息视图；不产消息时返回 false。
func projectEntry(e *pi.SessionEntry) (domain.MessageVO, bool) {
	switch e.Type {
	case "message":
		return projectMessage(e)
	case "compaction":
		return domain.MessageVO{
			ID: e.ID, Role: domain.RoleNotice, Type: "notice",
			Notice: "已压缩上下文", Content: pkg.StripANSI(e.Summary), CreatedAt: pi.IsoMillis(e.Timestamp),
		}, true
	case "branch_summary":
		return domain.MessageVO{
			ID: e.ID, Role: domain.RoleNotice, Type: "notice",
			Notice: "分支摘要", Content: pkg.StripANSI(e.Summary), CreatedAt: pi.IsoMillis(e.Timestamp),
		}, true
	case "model_change":
		return domain.MessageVO{
			ID: e.ID, Role: domain.RoleNotice, Type: "notice",
			Notice: "换了模型", Content: e.Provider + " / " + e.ModelID,
			CreatedAt: pi.IsoMillis(e.Timestamp),
		}, true
	case "thinking_level_change":
		return domain.MessageVO{
			ID: e.ID, Role: domain.RoleNotice, Type: "notice",
			Notice: "调整了思考档", Content: e.ThinkingLevel,
			CreatedAt: pi.IsoMillis(e.Timestamp),
		}, true
	default:
		// 会话头、用量、custom、label、session_info、context_edit 都不产消息。
		return domain.MessageVO{}, false
	}
}

// projectMessage 把一条 message 条目投影成消息视图。
func projectMessage(e *pi.SessionEntry) (domain.MessageVO, bool) {
	var m pi.SessionMessage
	if err := json.Unmarshal(e.Message, &m); err != nil {
		return domain.MessageVO{}, false
	}
	// 系统消息是提示词与工具声明，不是对话内容。
	if m.Role == "system" {
		return domain.MessageVO{}, false
	}
	vo := domain.MessageVO{
		ID: e.ID, Role: normalizeRole(m.Role), Type: "text",
		CreatedAt: pi.IsoMillis(e.Timestamp),
	}
	if m.Timestamp > 0 {
		vo.CreatedAt = m.Timestamp
	}
	vo.StopReason = m.StopReason
	vo.ToolCallID = m.ToolCallID
	vo.ToolName = m.ToolName
	vo.IsError = m.IsError
	if m.Usage != nil {
		vo.Usage = &domain.UsageVO{
			Input: m.Usage.Input, Output: m.Usage.Output,
			Cached: m.Usage.CacheRead, CacheWrite: m.Usage.CacheWrite,
			Total: m.Usage.TotalTokens, Cost: m.Usage.Cost.Total,
			Context: promptTokensOf(m.Usage),
		}
	}
	blocks := decodeBlocks(m.Content)
	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text := pkg.StripANSI(b.Text)
			texts = append(texts, text)
			vo.Blocks = append(vo.Blocks, domain.MessageBlock{Kind: domain.BlockText, Text: text})
		case "thinking":
			think := pkg.StripANSI(b.Thinking)
			vo.Thinking += think
			vo.Blocks = append(vo.Blocks, domain.MessageBlock{Kind: domain.BlockThinking, Text: think})
		case "toolCall":
			call := domain.ToolCall{ID: b.ID, Name: b.Name, Label: labelOf(b.Name), Args: b.Arguments}
			vo.ToolCalls = append(vo.ToolCalls, call)
			vo.Blocks = append(vo.Blocks, domain.MessageBlock{
				Kind: domain.BlockTool, ToolCallID: call.ID, ToolName: call.Name,
				Label: call.Label, Args: call.Args,
			})
		case "image":
			vo.Images = append(vo.Images, domain.MessageImage{MIME: b.MimeType, Base64: b.Data})
		}
	}
	vo.Content = strings.Join(texts, "\n")
	if m.Role == "toolResult" {
		vo.Type = "tool"
	}
	return vo, true
}

// promptTokensOf 是一次调用的提示词总量：上游把命中与未命中分两个字段报。
func promptTokensOf(u *pi.Usage) int {
	return u.Input + u.CacheRead + u.CacheWrite
}

// jsonlBlock 是内容块。
type jsonlBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	Thinking  string         `json:"thinking"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Data      string         `json:"data"`
	MimeType  string         `json:"mimeType"`
}

// decodeBlocks 解内容块；它可能是字符串（user 消息）或数组。
func decodeBlocks(raw []byte) []jsonlBlock {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return []jsonlBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	var blocks []jsonlBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

// normalizeRole 把 PI 的角色名换成前端的取值。
func normalizeRole(role string) string {
	if role == "toolResult" {
		return domain.RoleTool
	}
	return role
}

// samePath 比较两个路径是否指向同一处（Windows 大小写不敏感）。
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
