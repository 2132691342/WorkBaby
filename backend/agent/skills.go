package agent

// 技能索引：引擎只拿「叫什么、干什么、正文在哪」三件事。
// 引擎不扫磁盘、不读正文——数据目录里的技能只有一个读者（service），
// 它扫完把清单交进来，引擎才不会长出第二套目录知识。

import (
	"path/filepath"
	"strings"
	"sync"
)

// Skill 是一条可以交给模型的技能索引项。
type Skill struct {
	// Name 显示名；上层保证非空（frontmatter 缺 name 时取目录名）
	Name string
	// Description 一句话说明；模型靠它判断这个任务要不要读正文
	Description string
	// Path SKILL.md 绝对路径
	Path string
}

// SkillSet 是当前已启用技能清单。
// 整段替换而不是逐条增删：开关就是搬目录，重扫一遍拿到的就是真相；
// 做增量就得跟着搬移维护，错一次就长期与现实不符。
type SkillSet struct {
	// mu 保护 items 与 paths：技能开关与提示拼装会同时碰到这两份
	mu sync.RWMutex
	// items 清单本体，保持上层给的顺序
	items []Skill
	// paths 规范化后的路径集合：读正文时用它判「这个文件是不是技能」
	paths map[string]struct{}
}

// NewSkillSet 造一个空清单。
func NewSkillSet() *SkillSet {
	return &SkillSet{paths: map[string]struct{}{}}
}

// Replace 整段换掉清单；名字或路径缺项的条目直接丢弃，提示里不该出现半条记录。
func (s *SkillSet) Replace(items []Skill) {
	next := make([]Skill, 0, len(items))
	paths := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		path := strings.TrimSpace(item.Path)
		if name == "" || path == "" {
			continue
		}
		item.Name = name
		item.Path = path
		next = append(next, item)
		paths[normalizeSkillPath(path)] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = next
	s.paths = paths
}

// All 当前清单的副本；空清单给 nil，提示拼装据此整段省略。
func (s *SkillSet) All() []Skill {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.items) == 0 {
		return nil
	}
	return append([]Skill{}, s.items...)
}

// IsFile 这个路径是不是当前某个已启用技能的 SKILL.md。
// 只认精确路径不认目录：放行一个目录等于把整个技能区交给工具。
func (s *SkillSet) IsFile(path string) bool {
	if s == nil {
		return false
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.paths[normalizeSkillPath(trimmed)]
	return ok
}

// SetSkills 换掉注册表上的技能清单；传空即清空。
// 一轮运行中途换也不要紧：系统提示每步重拼，下一轮就读到新清单。
func (r *Registry) SetSkills(items []Skill) {
	if r == nil {
		return
	}
	if r.SkillIndex == nil {
		r.SkillIndex = NewSkillSet()
	}
	r.SkillIndex.Replace(items)
}

// Skills 当前技能索引；从没装过给 nil，调用方按「没有技能」处理。
func (r *Registry) Skills() *SkillSet {
	if r == nil {
		return nil
	}
	return r.SkillIndex
}

// normalizeSkillPath 折成比较用的形状：绝对、清理、小写——Windows 路径不分大小写。
// 相对路径按进程工作目录拼接，与技能路径对不上就是「不是技能」，不需要额外拒绝。
func normalizeSkillPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return strings.ToLower(filepath.Clean(abs))
}
