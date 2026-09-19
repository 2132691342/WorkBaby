package skill

import (
	"strings"
	"sync"

	"WorkBaby/internal/domain"
)

// LoadedSkill 运行时装载的 Skill（含解析后的工具白名单与触发词）。
type LoadedSkill struct {
	Skill    *domain.SkillDO
	Body     string   // Markdown 正文（prompt）
	Tools    []string // 工具白名单；空 = 不限制
	Triggers []string // 触发关键词
}

// Registry Skill 注册中心。
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*LoadedSkill
}

// NewRegistry 构造空注册中心。
func NewRegistry() *Registry { return &Registry{skills: make(map[string]*LoadedSkill)} }

// Build 启动期全量装载；单个 Skill 解析失败降级跳过，不阻断启动。
func (r *Registry) Build(list []domain.SkillDO) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills = make(map[string]*LoadedSkill, len(list))
	for i := range list {
		sk := &list[i]
		if !sk.Enabled {
			continue
		}
		r.skills[sk.Name] = &LoadedSkill{
			Skill:    sk,
			Body:     sk.Body,
			Tools:    allowedTools(sk.AllowedTools),
			Triggers: splitTriggers(sk.WhenToUse),
		}
	}
	return nil
}

// Reload 增删改后重建（复用 Build 语义）。
func (r *Registry) Reload(list []domain.SkillDO) error { return r.Build(list) }

// Get 按名取 Skill。
func (r *Registry) Get(name string) (*LoadedSkill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sk, ok := r.skills[name]
	return sk, ok
}

// List 返回全部已装载 Skill（按名称排序）。
func (r *Registry) List() []*LoadedSkill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*LoadedSkill, 0, len(r.skills))
	for _, sk := range r.skills {
		out = append(out, sk)
	}
	return out
}

// Match 关键词确定性匹配：命中触发词最长者优先（更具体），等长按 skill 名字典序。
//
// 旧实现遍历 map 命中即返回——Go map 遍历顺序随机，多个 Skill 同时命中时
// 每次结果可能不同。长触发词语义更具体（"合并 pdf" 优于 "pdf"），取最长命中
// 让路由结果确定且更符合意图。
func (r *Registry) Match(content string) string {
	if content == "" {
		return ""
	}
	lower := strings.ToLower(content)
	r.mu.RLock()
	defer r.mu.RUnlock()
	bestName := ""
	bestLen := 0
	for _, sk := range r.skills {
		for _, trig := range sk.Triggers {
			t := strings.ToLower(trig)
			if t == "" || !strings.Contains(lower, t) {
				continue
			}
			if len(t) > bestLen || (len(t) == bestLen && (bestName == "" || sk.Skill.Name < bestName)) {
				bestName = sk.Skill.Name
				bestLen = len(t)
			}
		}
	}
	return bestName
}

// splitTriggers 按行拆分触发词，过滤空行与前后空白。
func splitTriggers(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}
