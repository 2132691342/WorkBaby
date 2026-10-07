package service

import (
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// defaultSessionTitle 是新建会话的占位标题；首条用户消息到达后会被自动替换。
const defaultSessionTitle = "新对话"

// SessionService 负责会话与其条目链的读写。
type SessionService struct {
	env *Env
}

// NewSessionService 构造会话服务。
func NewSessionService(env *Env) *SessionService { return &SessionService{env: env} }

// Create 新建会话；缺省字段用全局设置兜底。
func (s *SessionService) Create(req domain.CreateSessionREQ) (*domain.SessionVO, error) {
	workspace := req.Workspace
	if workspace == "" {
		workspace = s.env.Cfg.Workspace
	}
	perm := domain.PermissionAsk
	if v, _ := s.env.Repo.GetSetting(domain.SettingPermission); v != "" {
		perm = v
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID, _ = s.env.Repo.GetSetting(domain.SettingDefaultProvider)
	}
	model := req.Model
	if model == "" {
		model, _ = s.env.Repo.GetSetting(domain.SettingDefaultModel)
	}
	// 默认模型没落库时，从默认服务的模型列表里取第一个。
	// 少了这步，新会话的模型名就是空的，界面一直显示「默认模型」、
	// 上下文窗口也永远算不出来。
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
		Permission: perm,
	}
	if err := s.env.Repo.CreateSession(sess); err != nil {
		return nil, err
	}
	vo := sess.ToVO()
	return &vo, nil
}

// List 列出会话。
func (s *SessionService) List() ([]domain.SessionVO, error) {
	list, err := s.env.Repo.ListSessions(200)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionVO, 0, len(list))
	for i := range list {
		out = append(out, list[i].ToVO())
	}
	return out, nil
}

// Get 取会话。
func (s *SessionService) Get(id string) (*domain.SessionVO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	vo := sess.ToVO()
	return &vo, nil
}

// Detail 取会话详情与已还原的线性消息。
func (s *SessionService) Detail(id string) (*domain.SessionDetailVO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	msgs, err := s.Messages(id)
	if err != nil {
		return nil, err
	}
	return &domain.SessionDetailVO{Session: sess.ToVO(), Messages: msgs}, nil
}

// Messages 从 leaf 沿 parent 上溯还原消息；遇到压缩条目即停止。
func (s *SessionService) Messages(id string) ([]domain.MessageVO, error) {
	chain, err := s.Chain(id)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MessageVO, 0, len(chain))
	for i := range chain {
		out = append(out, toMessageVO(&chain[i]))
	}
	return out, nil
}

// Chain 还原条目链（从旧到新）。
func (s *SessionService) Chain(id string) ([]domain.EntryDO, error) {
	sess, err := s.env.Repo.GetSession(id)
	if err != nil {
		return nil, err
	}
	entries, err := s.env.Repo.ListEntries(id)
	if err != nil {
		return nil, err
	}
	return buildChain(entries, sess.LeafEntryID), nil
}

// History 还原内核需要的消息数组。
func (s *SessionService) History(id string) ([]llm.Message, error) {
	chain, err := s.Chain(id)
	if err != nil {
		return nil, err
	}
	return toLLMMessages(chain), nil
}

// Rename 重命名。
func (s *SessionService) Rename(id, title string) error {
	if title == "" {
		return pkg.New(1105, "会话名不能为空", "")
	}
	return s.env.Repo.UpdateSessionColumns(id, map[string]any{"title": title})
}

// Delete 删除会话。
func (s *SessionService) Delete(id string) error { return s.env.Repo.DeleteSession(id) }

// SetModel 切换模型。
func (s *SessionService) SetModel(id, providerID, model string) error {
	return s.env.Repo.UpdateSessionColumns(id, map[string]any{"provider_id": providerID, "model": model})
}

// SetPermission 切换权限档。
func (s *SessionService) SetPermission(id, perm string) error {
	switch perm {
	case domain.PermissionAsk, domain.PermissionAutoEdit, domain.PermissionYolo:
	default:
		return pkg.New(1106, "权限档位不正确", perm)
	}
	return s.env.Repo.UpdateSessionColumns(id, map[string]any{"permission": perm})
}

// SetWorkspace 切换会话工作目录：下一条消息起，读写与命令都在新目录里跑。
// 目录必须真实存在——对话框给的一定在，但接口直调不一定。
func (s *SessionService) SetWorkspace(id, workspace string) error {
	if workspace == "" {
		return pkg.New(1002, "工作目录不能为空", "")
	}
	if !pkg.DirExists(workspace) {
		return pkg.New(1003, "这个目录不存在", workspace)
	}
	return s.env.Repo.UpdateSessionColumns(id, map[string]any{"workspace": workspace})
}

// Branch 从某条历史回溯：只把 leaf 指针移过去，老分支仍留在树里。
func (s *SessionService) Branch(id, entryID string) error {
	entries, err := s.env.Repo.ListEntries(id)
	if err != nil {
		return err
	}
	found := false
	for _, e := range entries {
		if e.ID == entryID {
			found = true
			break
		}
	}
	if !found {
		return domain.ErrEntryNotFound
	}
	return s.env.Repo.UpdateSessionColumns(id, map[string]any{"leaf_entry_id": entryID})
}

// Append 追加一条条目并返回它。
func (s *SessionService) Append(e *domain.EntryDO) error {
	seq, err := s.env.Repo.MaxSeq(e.SessionID)
	if err != nil {
		return err
	}
	e.Seq = seq + 1
	return s.env.Repo.AppendEntry(e)
}

// buildChain 从 leaf 沿 parent_id 上溯到根再反转。
// 数据库永远存完整历史，压缩只发生在发给模型那一刻（见 spec 02），因此这里不必截断。
func buildChain(entries []domain.EntryDO, leafID string) []domain.EntryDO {
	byID := make(map[string]domain.EntryDO, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	out := []domain.EntryDO{}
	seen := map[string]bool{}
	cur := leafID
	for cur != "" {
		if seen[cur] {
			break
		}
		seen[cur] = true
		e, ok := byID[cur]
		if !ok {
			break
		}
		out = append(out, e)
		cur = e.ParentID
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
