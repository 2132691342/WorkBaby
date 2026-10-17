// 会话编排：创建、列表、详情、改名、换模型与工作目录、分叉与删除。
// 会话元数据在 SQLite，消息真身在 PI 的 JSONL 里——这里只读投影，不写 JSONL。
package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// defaultSessionTitle 是新建会话的占位标题；首条用户消息到达后会被自动替换。
const defaultSessionTitle = "新对话"

// sessionPageSize 一页会话数。
const sessionPageSize = 200

// SessionService 负责会话元数据与 JSONL 投影。
type SessionService struct {
	env *Env
}

// NewSessionService 构造会话服务。
func NewSessionService(env *Env) *SessionService { return &SessionService{env: env} }

// Create 新建会话；缺省字段用全局设置兜底。
// 会话是懒创建的：PI 那边要等第一次真正发消息时才建，避免留下一堆空文件。
func (s *SessionService) Create(req domain.CreateSessionREQ) (*domain.SessionVO, error) {
	workspace := req.Workspace
	if workspace == "" {
		workspace = s.env.Cfg.Workspace
	}
	perm := req.Permission
	if perm == "" {
		perm = domain.PermissionAskWrite
		if v, _ := s.env.Repo.GetSetting(domain.SettingPermission); v != "" {
			perm = v
		}
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID, _ = s.env.Repo.GetSetting(domain.SettingDefaultProvider)
	}
	model := req.Model
	if model == "" {
		model, _ = s.env.Repo.GetSetting(domain.SettingDefaultModel)
	}
	// 默认模型没落库时从默认服务的列表取第一个：否则模型名为空，窗口永远算不出来。
	if model == "" && providerID != "" {
		if d, err := s.env.Repo.GetProvider(providerID); err == nil {
			if ms := parseModels(d.Models); len(ms) > 0 {
				model = ms[0]
			}
		}
	}
	title := req.Title
	if title == "" {
		title = defaultSessionTitle
	}
	sess := &domain.SessionDO{
		ID:         pkg.NewID(domain.PrefixSession),
		Title:      title,
		Workspace:  workspace,
		ProviderID: providerID,
		Model:      model,
		Permission: PermissionOf(perm),
		Hidden:     req.Hidden,
	}
	if err := s.env.Repo.CreateSession(sess); err != nil {
		return nil, err
	}
	vo := sess.ToVO()
	return &vo, nil
}

// List 按 offset 分页列出会话。
func (s *SessionService) List(offset int) ([]domain.SessionVO, error) {
	list, err := s.env.Repo.ListSessions(sessionPageSize, offset)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionVO, 0, len(list))
	for i := range list {
		out = append(out, list[i].ToVO())
	}
	return out, nil
}

// Get 取会话元数据。
func (s *SessionService) Get(id string) (*domain.SessionVO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	vo := sess.ToVO()
	return &vo, nil
}

// Detail 取会话详情与从 JSONL 投影的消息。
func (s *SessionService) Detail(id string) (*domain.SessionDetailVO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	msgs, err := s.Messages(sess)
	if err != nil {
		return nil, err
	}
	return &domain.SessionDetailVO{Session: sess.ToVO(), Messages: msgs}, nil
}

// Messages 读 PI 的 JSONL 并投影成消息视图。
// 不做上下文重建：只显示文件里有什么，不决定模型看到什么。
func (s *SessionService) Messages(sess *domain.SessionDO) ([]domain.MessageVO, error) {
	out := []domain.MessageVO{}
	if sess.PiSessionFile == "" || !pkg.FileExists(sess.PiSessionFile) {
		// 懒创建：还没发过消息的会话没有 JSONL，这是正常状态不是错误。
		return out, nil
	}
	raw, err := os.ReadFile(sess.PiSessionFile)
	if err != nil {
		return nil, pkg.Wrap("9715", "读不到会话记录", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry jsonlEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// 单行坏掉不该让整个会话打不开。
			continue
		}
		if msg, ok := projectEntry(&entry); ok {
			out = append(out, msg)
		}
	}
	return out, nil
}

// Rename 重命名；同时同步到 PI（它的 /resume 也读这个名字）。
func (s *SessionService) Rename(id, title string) error {
	if title == "" {
		return pkg.New("1105", "会话名不能为空", "")
	}
	if err := s.env.Repo.UpdateSessionColumns(id, map[string]any{"title": title}); err != nil {
		return err
	}
	sess, err := s.env.Repo.GetSession(id)
	if err != nil || sess.PiSessionID == "" {
		return nil
	}
	if client := s.env.PiSupervisor().Get(sess.Workspace); client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.SetSessionName(ctx, title); err != nil {
			pkg.Warnf("session: 同步会话名到 PI 失败: %v", err)
		}
	}
	return nil
}

// Delete 删除会话：SQLite 行 + PI 的 JSONL 文件都要删，只删一边会留孤儿。
func (s *SessionService) Delete(id string) error {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return err
	}
	// 会话在跑时先停：不等它收尾就删，PI 会往已删除的文件继续追加。
	if client := s.env.PiSupervisor().Get(sess.Workspace); client != nil && client.Alive() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = client.Abort(ctx)
		cancel()
	}
	if sess.PiSessionFile != "" && pkg.FileExists(sess.PiSessionFile) {
		if err := os.Remove(sess.PiSessionFile); err != nil {
			pkg.Warnf("session: 删除会话文件失败 %s: %v", sess.PiSessionFile, err)
		}
	}
	return s.env.Repo.DeleteSession(id)
}

// SetModel 切换模型；同步到 PI。
func (s *SessionService) SetModel(id, providerID, model string) error {
	if err := s.env.Repo.UpdateSessionColumns(id, map[string]any{
		"provider_id": providerID, "model": model,
	}); err != nil {
		return err
	}
	if sess, err := s.env.Repo.GetSession(id); err == nil {
		if client := s.env.PiSupervisor().Get(sess.Workspace); client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := client.SetModel(ctx, providerID, model); err != nil {
				pkg.Warnf("session: 同步模型到 PI 失败: %v", err)
			}
		}
	}
	return nil
}

// SetThinkingLevel 切换思考档位。
func (s *SessionService) SetThinkingLevel(id, level string) error {
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return pkg.New("1107", "思考档位不正确", level)
	}
	if err := s.env.Repo.UpdateSessionColumns(id, map[string]any{"thinking_level": level}); err != nil {
		return err
	}
	if sess, err := s.env.Repo.GetSession(id); err == nil {
		if client := s.env.PiSupervisor().Get(sess.Workspace); client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := client.SetThinkingLevel(ctx, level); err != nil {
				pkg.Warnf("session: 同步思考档到 PI 失败: %v", err)
			}
		}
	}
	return nil
}

// SetPermission 切换权限档，并写进 policy 文件供审批扩展读取。
func (s *SessionService) SetPermission(id, perm string) error {
	switch perm {
	case domain.PermissionAuto, domain.PermissionAskWrite, domain.PermissionReadonly:
	default:
		return pkg.New("1106", "权限档位不正确", perm)
	}
	if err := s.env.Repo.UpdateSessionColumns(id, map[string]any{"permission": perm}); err != nil {
		return err
	}
	return s.WritePolicy(id, perm)
}

// WritePolicy 把一个会话的审批档写进 policy 文件。
// 审批扩展在 session_start 读它，所以切档位必须连同文件一起改。
func (s *SessionService) WritePolicy(sessionID, perm string) error {
	policy := map[string]any{
		"permission": perm,
		"timeoutMs":  1800000,
	}
	if allowed := s.env.Approvals.AllowedTools(sessionID); len(allowed) > 0 {
		policy["allowTools"] = allowed
	}
	return writeJSONFile(s.env.PiEnv.PolicyPath(), policy)
}

// SetWorkspace 切换会话工作目录。
// cwd 是 PI 进程级属性，所以切换后当前进程的会话上下文失效：下一条消息按新工作区重建。
func (s *SessionService) SetWorkspace(id, workspace string) error {
	if workspace == "" {
		return pkg.New("1002", "工作目录不能为空", "")
	}
	if !pkg.DirExists(workspace) {
		return pkg.New("1003", "这个目录不存在", workspace)
	}
	if err := s.env.Repo.UpdateSessionColumns(id, map[string]any{"workspace": workspace}); err != nil {
		return err
	}
	// 旧工作区的进程不能继续承载这个会话：它的 cwd 不是新目录。
	if sess, err := s.env.Repo.GetSession(id); err == nil && sess.PiSessionFile != "" {
		if client := s.env.PiSupervisor().Get(sess.Workspace); client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = client.Abort(ctx)
			cancel()
		}
	}
	s.env.Emitter.Emit(id, domain.EventChatSwitched, domain.SwitchedData{
		SessionID: id, Reason: "workspace_restart",
	})
	return nil
}

// Fork 从某条历史分叉成一条新会话。
// PI 的 leaf 是内部状态，协议上没有「改 leaf」这回事——所以语义是分叉而不是回退。
func (s *SessionService) Fork(id, entryID string) (*domain.SessionVO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	client, err := s.ensureClient(sess)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 先切回原会话（进程可能停在别的会话上），再从那条条目分叉。
	if sess.PiSessionFile != "" {
		if err := client.SwitchSession(ctx, sess.PiSessionFile); err != nil {
			pkg.Warnf("session: 切回原会话失败（继续尝试分叉）: %v", err)
		}
	}
	res, err := client.Fork(ctx, entryID)
	if err != nil {
		return nil, err
	}
	st, err := client.GetState(ctx)
	if err != nil {
		return nil, err
	}
	sessionID := st.SessionID
	if res != nil && res.SessionID != "" {
		sessionID = res.SessionID
	}
	file := st.SessionFile
	if res != nil && res.SessionFile != "" {
		file = res.SessionFile
	}
	// 新会话在 WorkBaby 侧也要有一行，否则界面上找不到它。
	vo, err := s.Create(domain.CreateSessionREQ{
		Title:      sess.Title + "（分支）",
		Workspace:  sess.Workspace,
		ProviderID: sess.ProviderID,
		Model:      sess.Model,
		Permission: sess.Permission,
		Hidden:     sess.Hidden,
	})
	if err != nil {
		return nil, err
	}
	if err := s.env.Repo.UpdateSessionColumns(vo.ID, map[string]any{
		"pi_session_id": sessionID, "pi_session_file": file,
	}); err != nil {
		return nil, err
	}
	vo.PiSessionID = sessionID
	vo.PiSessionFile = file
	return vo, nil
}

// ensureClient 取会话工作区的 PI 进程。
func (s *SessionService) ensureClient(sess *domain.SessionDO) (*pi.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.env.PiSupervisor().Ensure(ctx, sess.Workspace)
}

// AutoTitle 用首条用户消息给会话起名。
func (s *SessionService) AutoTitle(sess *domain.SessionDO, content string) {
	if sess.MessageCount > 0 || (sess.Title != "" && sess.Title != defaultSessionTitle) {
		return
	}
	title := strings.Join(strings.Fields(content), " ")
	if len([]rune(title)) > 24 {
		title = string([]rune(title)[:24]) + "…"
	}
	if err := s.Rename(sess.ID, title); err != nil {
		pkg.Warnf("session: 自动命名失败: %v", err)
	}
}

// jsonlEntry 是 PI 会话文件的一行（只解我们关心的字段）。
type jsonlEntry struct {
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	ParentID      string          `json:"parentId"`
	Timestamp     string          `json:"timestamp"`
	Message       json.RawMessage `json:"message"`
	Summary       string          `json:"summary"`
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	ModelID       string          `json:"modelId"`
	ThinkingLevel string          `json:"thinkingLevel"`
}

// jsonlMessage 是 entry.message 的形状。
type jsonlMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Timestamp  int64           `json:"timestamp"`
	Usage      *pi.Usage       `json:"usage"`
	StopReason string          `json:"stopReason"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	IsError    bool            `json:"isError"`
}

// projectEntry 把一行 JSONL 投影成一条消息视图；不产消息时返回 false。
func projectEntry(e *jsonlEntry) (domain.MessageVO, bool) {
	switch e.Type {
	case "message":
		return projectMessage(e)
	case "compaction":
		return domain.MessageVO{
			ID: e.ID, Role: "notice", Type: "notice",
			Notice:  "已压缩上下文",
			Content: e.Summary, CreatedAt: isoMillis(e.Timestamp),
		}, true
	case "branch_summary":
		return domain.MessageVO{
			ID: e.ID, Role: "notice", Type: "notice",
			Notice: "分支摘要", Content: e.Summary, CreatedAt: isoMillis(e.Timestamp),
		}, true
	case "model_change":
		return domain.MessageVO{
			ID: e.ID, Role: "notice", Type: "notice",
			Notice: "换了模型", Content: e.Provider + " / " + e.ModelID,
			CreatedAt: isoMillis(e.Timestamp),
		}, true
	case "thinking_level_change":
		return domain.MessageVO{
			ID: e.ID, Role: "notice", Type: "notice",
			Notice: "调整了思考档", Content: e.ThinkingLevel,
			CreatedAt: isoMillis(e.Timestamp),
		}, true
	case "custom_message":
		return domain.MessageVO{
			ID: e.ID, Role: "assistant", Type: "text",
			Content: e.Summary, Hidden: true, CreatedAt: isoMillis(e.Timestamp),
		}, true
	default:
		// session 头、usage、custom、label、session_info、context_edit 都不产消息。
		return domain.MessageVO{}, false
	}
}

// projectMessage 把一条 message 条目投影成消息视图。
func projectMessage(e *jsonlEntry) (domain.MessageVO, bool) {
	var m jsonlMessage
	if err := json.Unmarshal(e.Message, &m); err != nil {
		return domain.MessageVO{}, false
	}
	// 系统消息是提示词与工具声明，不是对话内容。
	if m.Role == "system" {
		return domain.MessageVO{}, false
	}
	vo := domain.MessageVO{
		ID: e.ID, Role: normalizeRole(m.Role), Type: "text",
		CreatedAt: isoMillis(e.Timestamp),
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
			Cached: m.Usage.CacheRead, Total: m.Usage.TotalTokens,
			Cost: m.Usage.Cost.Total, Context: m.Usage.Input + m.Usage.CacheRead,
		}
	}
	blocks := decodeBlocks(m.Content)
	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "thinking":
			vo.Thinking += b.Thinking
		case "toolCall":
			vo.ToolCalls = append(vo.ToolCalls, domain.ToolCall{
				ID: b.ID, Name: b.Name, Label: labelOf(b.Name), Args: b.Arguments,
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
func decodeBlocks(raw json.RawMessage) []jsonlBlock {
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

// isoMillis 把 ISO 时间转成毫秒整数；解不出返回 0。
func isoMillis(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// writeJSONFile 原子写一份 JSON 配置。
func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "配置无法序列化", err)
	}
	return writeFileAtomic(path, append(raw, '\n'))
}
