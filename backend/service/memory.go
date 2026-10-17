// 记忆服务：读写长期记忆，并把它渲染成系统提示的一段。
package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 提示注入的两个上限：只封条数会遇上几条超长的，只封字符会遇上几百条短的
// 把提示切成碎片，两个都封才稳。
const (
	maxPromptMemories = 30
	maxPromptChars    = 2000
)

// MemoryService 记忆的读写与提示注入。
type MemoryService struct {
	env *Env
}

// NewMemoryService 构造记忆服务。
func NewMemoryService(env *Env) *MemoryService { return &MemoryService{env: env} }

// List 全部记忆。
func (s *MemoryService) List() (*domain.ListMemoriesRESP, error) {
	list, err := s.env.Repo.ListMemories()
	if err != nil {
		return nil, err
	}
	items := make([]domain.MemoryVO, 0, len(list))
	for i := range list {
		items = append(items, memoryVO(&list[i]))
	}
	return &domain.ListMemoriesRESP{Items: items}, nil
}

// Add 写一条记忆。内容完全相同的只刷新时刻：模型记不住自己说过什么，
// 「同一件事记十遍」很快就把提示吃满了。
func (s *MemoryService) Add(content string, source string) (*domain.MemoryVO, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, domain.ErrMemoryEmpty
	}
	if len([]rune(content)) > domain.MaxMemoryChars {
		return nil, domain.ErrMemoryTooLong
	}
	if n, err := s.env.Repo.CountMemories(); err == nil && n >= domain.MaxMemories {
		return nil, domain.ErrMemoryFull
	}
	if old, err := s.env.Repo.FindMemoryByContent(content); err == nil && old != nil {
		ts := nowMillis()
		if err := s.env.Repo.UpdateMemory(old.ID, map[string]any{"updated_at": ts}); err != nil {
			return nil, err
		}
		old.UpdatedAt = ts
		vo := memoryVO(old)
		return &vo, nil
	}
	m := &domain.MemoryDO{
		ID:      pkg.NewID(domain.PrefixMemory),
		Content: content,
		Source:  source,
		Enabled: true,
	}
	if err := s.env.Repo.AddMemory(m); err != nil {
		return nil, err
	}
	pkg.Infof("memory: 记下一条 source=%s len=%d", source, len([]rune(content)))
	vo := memoryVO(m)
	return &vo, nil
}

// Remember 供工具层调用：助手在对话里记下的一律标为 auto，
// 界面上用户自己写的才标 manual——两类要在界面上分得开。
func (s *MemoryService) Remember(content string) (*domain.MemoryVO, error) {
	return s.Add(content, domain.MemorySourceAuto)
}

// Update 改内容或开关；两者都空时报错而不是静默成功。
func (s *MemoryService) Update(id string, req domain.UpdateMemoryREQ) (*domain.MemoryVO, error) {
	fields := map[string]any{}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}
	if c := strings.TrimSpace(req.Content); c != "" {
		if len([]rune(c)) > domain.MaxMemoryChars {
			return nil, domain.ErrMemoryTooLong
		}
		fields["content"] = c
	}
	if len(fields) == 0 {
		return nil, pkg.New("1107", "没有要改的内容", "")
	}
	if err := s.env.Repo.UpdateMemory(id, fields); err != nil {
		return nil, err
	}
	m, err := s.env.Repo.GetMemory(id)
	if err != nil {
		return nil, err
	}
	vo := memoryVO(m)
	return &vo, nil
}

// Delete 删除一条记忆。
func (s *MemoryService) Delete(id string) error {
	return s.env.Repo.DeleteMemory(id)
}

// RenderForPrompt 把启用的记忆拼成提示段；没有可注入的就返回空串，调用方据此整段省略。
func (s *MemoryService) RenderForPrompt() string {
	list, err := s.env.Repo.ListMemories()
	if err != nil {
		// 记读取失败但不拦对话：记忆是增强，读不出来不该让整个助手开不了口。
		pkg.Warnf("memory: 读取失败，本轮不带记忆: %v", err)
		return ""
	}
	var b strings.Builder
	used, n := 0, 0
	for i := range list {
		m := &list[i]
		if !m.Enabled {
			continue
		}
		size := len([]rune(m.Content))
		if n >= maxPromptMemories || used+size > maxPromptChars {
			break
		}
		b.WriteString("- " + m.Content + "\n")
		used += size
		n++
	}
	if b.Len() == 0 {
		return ""
	}
	return "## 记住的这些事\n下面是用户说过或你记下的长期事实，跨对话有效；" +
		"与当前对话里刚说的内容冲突时，以当前对话为准。\n" + b.String()
}

// memoryVO 把 DO 转成视图对象。
func memoryVO(m *domain.MemoryDO) domain.MemoryVO {
	return domain.MemoryVO{
		ID:        m.ID,
		Content:   m.Content,
		Source:    m.Source,
		Enabled:   m.Enabled,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
