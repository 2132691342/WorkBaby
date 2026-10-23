// Package skill 解析 SKILL.md 并维护注册中心。文件即真相源，不落库。
package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"gopkg.in/yaml.v3"
)

// nameRe 限定技能名，保证 /skill:名字 的词法边界清晰。
var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidName 报告一个名字能否用作技能目录名与 /skill: 词法名：
// 只允许小写字母、数字与连字符，最长 64 字符——路径分隔符与「..」天然被拒。
func ValidName(name string) bool {
	return name != "" && len(name) <= 64 && nameRe.MatchString(name)
}

// Registry 是技能注册中心；写只在加载期与设置页发生，读是高频路径。
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*domain.Skill
	order  []string
}

// New 构造空注册中心。
func New() *Registry {
	return &Registry{skills: map[string]*domain.Skill{}}
}

// Add 加入一个技能；同名按来源优先级覆盖（workspace > global > builtin）。
func (r *Registry) Add(s domain.Skill) { r.add(s) }

// add 加入一个技能；同名按来源优先级覆盖，先加入者先排序。
func (r *Registry) add(s domain.Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := s.Name + "@" + s.Source
	if _, ok := r.skills[key]; !ok {
		r.order = append(r.order, key)
	}
	r.skills[key] = &s
}

// List 按来源与名称排序列出技能。
func (r *Registry) List() []domain.Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Skill, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, *r.skills[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return sourceRank(out[i].Source) < sourceRank(out[j].Source)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Get 按 id（name@source）或名字取技能；同名多来源时返回优先级最高的那个。
// 两种形态都要认：前端拿到的就是 id，只认名字会让「删除」这类按 id 调用的操作永远落空。
func (r *Registry) Get(idOrName string) (*domain.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *domain.Skill
	for _, k := range r.order {
		s := r.skills[k]
		if k != idOrName && s.Name != idOrName {
			continue
		}
		if best == nil || sourceRank(s.Source) > sourceRank(best.Source) {
			best = s
		}
	}
	if best == nil {
		return nil, false
	}
	cp := *best
	return &cp, true
}

// SetEnabled 同步某个技能的启用标志：移目录后重扫前由服务层调用，保证界面即时反映。
func (r *Registry) SetEnabled(idOrName string, enabled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ok := false
	for _, k := range r.order {
		if k == idOrName || r.skills[k].Name == idOrName {
			r.skills[k].Enabled = enabled
			ok = true
		}
	}
	return ok
}

// Remove 从注册表里移除一个技能。磁盘文件由调用方负责。
func (r *Registry) Remove(idOrName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.order[:0]
	removed := false
	for _, k := range r.order {
		if k == idOrName || r.skills[k].Name == idOrName {
			delete(r.skills, k)
			removed = true
			continue
		}
		kept = append(kept, k)
	}
	r.order = kept
	return removed
}

// DropSource 摘掉某个来源的全部技能；切换工作目录时用，避免新旧目录的技能混在一起。
func (r *Registry) DropSource(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.order[:0]
	for _, k := range r.order {
		if r.skills[k].Source == source {
			delete(r.skills, k)
			continue
		}
		kept = append(kept, k)
	}
	r.order = kept
}

// Content 读技能正文：按 Location 从磁盘读；停用的技能也要能打开来看。
func (r *Registry) Content(name string) (string, error) {
	s, ok := r.Get(name)
	if !ok {
		return "", domain.ErrSkillNotFound
	}
	raw, err := os.ReadFile(s.Location)
	if err != nil {
		return "", pkg.Wrap("8104", "读取技能文件失败", err)
	}
	return stripFrontmatter(string(raw)), nil
}

// LoadDir 扫描一个目录下的 SKILL.md；遇到 SKILL.md 的目录不再向下递归。
// 扫出来的技能都是启用态——清单注入由 PI 原生完成，只有 skills/ 会被模型看见（specs/03）。
func LoadDir(dir, source string) []domain.Skill {
	return loadDir(dir, source, true)
}

// LoadDisabledDir 扫描停用了的技能目录：它们不进 PI 的视野，只给界面列与恢复用。
func LoadDisabledDir(dir, source string) []domain.Skill {
	return loadDir(dir, source, false)
}

func loadDir(dir, source string, enabled bool) []domain.Skill {
	out := []domain.Skill{}
	if dir == "" || !pkg.DirExists(dir) {
		return out
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		s, perr := Parse(path, source)
		if perr != nil {
			pkg.Warnf("skill: 跳过无法解析的技能 %s: %v", path, perr)
			return nil
		}
		s.Enabled = enabled
		out = append(out, s)
		return filepath.SkipDir
	})
	return out
}

// Parse 解析磁盘上的一个 SKILL.md：YAML frontmatter 给元信息，正文是方法论。
func Parse(path, source string) (domain.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, pkg.Wrap("8102", "读取技能文件失败", err)
	}
	return ParseContent(string(raw), path, source)
}

// ParseContent 解析技能正文：YAML frontmatter 给元信息，其余是方法论。
func ParseContent(text, location, source string) (domain.Skill, error) {
	// frontmatter 里可能混有列表、布尔、数字（when_to_use、version…），
	// 收进 map[string]string 会让整份 YAML 解析失败、技能被静默跳过。
	meta := map[string]any{}
	body := text

	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		rest := strings.TrimSpace(text)
		rest = strings.TrimPrefix(rest, "---")
		if idx := strings.Index(rest, "\n---"); idx >= 0 {
			head := rest[:idx]
			body = strings.TrimSpace(rest[idx+4:])
			if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
				return domain.Skill{}, pkg.Wrap("8102", "技能开头说明格式不正确", err)
			}
		}
	}

	name := strings.TrimSpace(scalar(meta["name"]))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filepath.Dir(location)), ".md")
	}
	if !ValidName(name) {
		return domain.Skill{}, pkg.Wrap("8101", "技能名不合法", domain.ErrSkillName)
	}
	desc := strings.TrimSpace(scalar(meta["description"]))
	if desc == "" {
		desc = firstLine(body)
	}
	return domain.Skill{
		ID:          name + "@" + source,
		Name:        name,
		Description: desc,
		WhenToUse:   stringsOf(meta["when_to_use"]),
		Location:    location,
		Source:      source,
		Enabled:     true,
	}, nil
}

// stripFrontmatter 去掉正文开头的 frontmatter，只留方法论部分。
func stripFrontmatter(text string) string {
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		return text
	}
	rest := strings.TrimSpace(text)
	rest = strings.TrimPrefix(rest, "---")
	if idx := strings.Index(rest, "\n---"); idx >= 0 {
		return strings.TrimSpace(rest[idx+4:])
	}
	return text
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.TrimSpace(s)
}

// stringsOf 把 frontmatter 里可能是标量或列表的值收成字符串列表（when_to_use 两种写法都合法）。
func stringsOf(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s := strings.TrimSpace(scalar(it)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func sourceRank(s string) int {
	switch s {
	case domain.SkillSourceWorkspace:
		return 3
	case domain.SkillSourceGlobal:
		return 2
	default:
		return 1
	}
}
